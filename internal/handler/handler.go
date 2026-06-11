package handler

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"sort"
	"time"

	"ai-proxy-gateway/internal/auth"
	"ai-proxy-gateway/internal/config"
	"ai-proxy-gateway/internal/lb"
	"ai-proxy-gateway/internal/provider"
	"ai-proxy-gateway/internal/runtime"

	"github.com/gin-gonic/gin"
)

type Handler struct {
	cfg      *config.Config
	lb       *lb.LoadBalancer
	store    *runtime.Store
	adapter  provider.Adapter
	models   map[string][]provider.Node
	retryMap map[int]struct{}
}

func New(cfg *config.Config, balancer *lb.LoadBalancer, store *runtime.Store, adapter provider.Adapter) *Handler {
	models := make(map[string][]provider.Node)
	for name, p := range cfg.Provider {
		if p.Enabled != nil && !*p.Enabled {
			continue
		}
		node := provider.Node{Name: name, Config: p}
		for model := range p.Models {
			models[model] = append(models[model], node)
		}
	}
	retryMap := make(map[int]struct{}, len(cfg.Routing.Retry.RetryOnStatus))
	for _, code := range cfg.Routing.Retry.RetryOnStatus {
		retryMap[code] = struct{}{}
	}
	return &Handler{cfg: cfg, lb: balancer, store: store, adapter: adapter, models: models, retryMap: retryMap}
}

func (h *Handler) Healthz(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func (h *Handler) Readyz(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ready"})
}

func (h *Handler) Models(c *gin.Context) {
	appKey, _ := auth.Current(c)
	ids := make([]string, 0, len(h.models))
	for model := range h.models {
		if appKey.Allows(model) {
			ids = append(ids, model)
		}
	}
	sort.Strings(ids)
	data := make([]gin.H, 0, len(ids))
	for _, id := range ids {
		data = append(data, gin.H{"id": id, "object": "model", "owned_by": "ai-proxy-gateway"})
	}
	c.JSON(http.StatusOK, gin.H{"object": "list", "data": data})
}

func (h *Handler) Providers(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"data": h.store.All()})
}

func (h *Handler) ChatCompletions(c *gin.Context) {
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		writeError(c, http.StatusBadRequest, "failed to read request body", "invalid_request_error", "bad_request")
		return
	}
	var payload struct {
		Model  string `json:"model"`
		Stream bool   `json:"stream"`
	}
	if err := json.Unmarshal(body, &payload); err != nil || payload.Model == "" {
		writeError(c, http.StatusBadRequest, "model is required", "invalid_request_error", "missing_model")
		return
	}
	appKey, _ := auth.Current(c)
	if !appKey.Allows(payload.Model) {
		writeError(c, http.StatusForbidden, "model is not allowed", "auth_error", "model_forbidden")
		return
	}
	candidates := h.models[payload.Model]
	if len(candidates) == 0 {
		writeError(c, http.StatusNotFound, "model is not available", "invalid_request_error", "model_not_found")
		return
	}
	req := &provider.ProxyRequest{Method: http.MethodPost, Path: c.Request.URL.Path, Header: c.Request.Header.Clone(), Body: body, Model: payload.Model, Stream: payload.Stream}
	if payload.Stream {
		h.handleStream(c, req, candidates)
		return
	}
	h.handleNonStream(c, req, candidates)
}

func (h *Handler) handleNonStream(c *gin.Context, req *provider.ProxyRequest, candidates []provider.Node) {
	tried := map[string]struct{}{}
	maxAttempts := min(h.cfg.Routing.Retry.MaxAttempts, len(candidates))
	var lastResp *provider.ProxyResponse
	var lastErr error
	for attempt := 0; attempt < maxAttempts; attempt++ {
		node, err := h.lb.Pick(c.Request.Context(), req.Model, candidates, tried)
		if err != nil {
			lastErr = err
			break
		}
		tried[node.Name] = struct{}{}
		req.UpstreamModel = node.Config.Models[req.Model].UpstreamModel
		ctx, cancel := context.WithTimeout(c.Request.Context(), h.cfg.Routing.Retry.PerAttemptTimeout)
		start := time.Now()
		resp, err := h.adapter.Do(ctx, node, req)
		cancel()
		latency := time.Since(start)
		if err != nil {
			h.store.Report(node.Name, false, latency)
			lastErr = err
			slog.Warn("upstream request failed", "provider", node.Name, "model", req.Model, "error", err.Error())
			continue
		}
		lastResp = resp
		if !h.isRetryable(resp.StatusCode) {
			h.store.Report(node.Name, resp.StatusCode < 500, latency)
			copyResponse(c, resp)
			return
		}
		h.store.Report(node.Name, false, latency)
	}
	if lastResp != nil {
		copyResponse(c, lastResp)
		return
	}
	if lastErr == nil {
		lastErr = errors.New("all providers failed")
	}
	writeError(c, http.StatusBadGateway, lastErr.Error(), "upstream_error", "provider_unavailable")
}

func (h *Handler) handleStream(c *gin.Context, req *provider.ProxyRequest, candidates []provider.Node) {
	tried := map[string]struct{}{}
	maxAttempts := min(h.cfg.Routing.Retry.MaxAttempts, len(candidates))
	var lastErr error
	for attempt := 0; attempt < maxAttempts; attempt++ {
		node, err := h.lb.Pick(c.Request.Context(), req.Model, candidates, tried)
		if err != nil {
			lastErr = err
			break
		}
		tried[node.Name] = struct{}{}
		req.UpstreamModel = node.Config.Models[req.Model].UpstreamModel
		start := time.Now()
		resp, err := h.adapter.Stream(c.Request.Context(), node, req)
		latency := time.Since(start)
		if err != nil {
			h.store.Report(node.Name, false, latency)
			lastErr = err
			continue
		}
		if h.isRetryable(resp.StatusCode) {
			h.store.Report(node.Name, false, latency)
			resp.Body.Close()
			resp.Cancel()
			continue
		}
		h.store.Report(node.Name, resp.StatusCode < 500, latency)
		streamResponse(c, resp)
		return
	}
	if lastErr == nil {
		lastErr = errors.New("all providers failed")
	}
	writeError(c, http.StatusBadGateway, lastErr.Error(), "upstream_error", "provider_unavailable")
}

func (h *Handler) isRetryable(status int) bool {
	_, ok := h.retryMap[status]
	return ok
}

func copyResponse(c *gin.Context, resp *provider.ProxyResponse) {
	for k, values := range resp.Header {
		if k == "Content-Length" {
			continue
		}
		for _, v := range values {
			c.Writer.Header().Add(k, v)
		}
	}
	c.Data(resp.StatusCode, resp.Header.Get("Content-Type"), resp.Body)
}

func streamResponse(c *gin.Context, resp *provider.StreamResponse) {
	defer resp.Body.Close()
	defer resp.Cancel()
	for k, values := range resp.Header {
		if k == "Content-Length" {
			continue
		}
		for _, v := range values {
			c.Writer.Header().Add(k, v)
		}
	}
	if c.Writer.Header().Get("Content-Type") == "" {
		c.Writer.Header().Set("Content-Type", "text/event-stream")
	}
	c.Status(resp.StatusCode)
	buf := make([]byte, 32*1024)
	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			if _, writeErr := c.Writer.Write(buf[:n]); writeErr != nil {
				return
			}
			c.Writer.Flush()
		}
		if err != nil {
			return
		}
	}
}

func writeError(c *gin.Context, status int, message, typ, code string) {
	c.JSON(status, gin.H{"error": gin.H{"message": message, "type": typ, "code": code}})
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
