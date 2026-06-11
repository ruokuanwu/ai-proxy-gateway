package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"ai-proxy-gateway/internal/provider"
)

type Adapter struct {
	client *http.Client
}

func New(timeout time.Duration) *Adapter {
	return &Adapter{client: &http.Client{Timeout: timeout}}
}

func (a *Adapter) Do(ctx context.Context, node provider.Node, req *provider.ProxyRequest) (*provider.ProxyResponse, error) {
	body, err := rewriteModel(req.Body, req.UpstreamModel)
	if err != nil {
		return nil, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, req.Method, upstreamURL(node.Config.Options.BaseURL, req.Path), bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	copyForwardHeaders(httpReq.Header, req.Header)
	httpReq.Header.Set("Authorization", "Bearer "+node.Config.Options.APIKey)
	resp, err := a.client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	return &provider.ProxyResponse{StatusCode: resp.StatusCode, Header: resp.Header.Clone(), Body: respBody}, nil
}

func (a *Adapter) Stream(ctx context.Context, node provider.Node, req *provider.ProxyRequest) (*provider.StreamResponse, error) {
	body, err := rewriteModel(req.Body, req.UpstreamModel)
	if err != nil {
		return nil, err
	}
	streamCtx, cancel := context.WithCancel(ctx)
	httpReq, err := http.NewRequestWithContext(streamCtx, req.Method, upstreamURL(node.Config.Options.BaseURL, req.Path), bytes.NewReader(body))
	if err != nil {
		cancel()
		return nil, err
	}
	copyForwardHeaders(httpReq.Header, req.Header)
	httpReq.Header.Set("Authorization", "Bearer "+node.Config.Options.APIKey)
	client := *a.client
	client.Timeout = 0
	resp, err := client.Do(httpReq)
	if err != nil {
		cancel()
		return nil, err
	}
	return &provider.StreamResponse{StatusCode: resp.StatusCode, Header: resp.Header.Clone(), Body: resp.Body, Cancel: cancel}, nil
}

func rewriteModel(body []byte, model string) ([]byte, error) {
	if model == "" {
		return body, nil
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("invalid json body: %w", err)
	}
	payload["model"] = model
	return json.Marshal(payload)
}

func upstreamURL(baseURL, path string) string {
	baseURL = strings.TrimRight(baseURL, "/")
	path = "/" + strings.TrimLeft(path, "/")
	if u, err := url.Parse(baseURL); err == nil && u.Path != "" && strings.HasSuffix(u.Path, "/v1") && strings.HasPrefix(path, "/v1/") {
		path = strings.TrimPrefix(path, "/v1")
	}
	return baseURL + path
}

func copyForwardHeaders(dst, src http.Header) {
	for k, values := range src {
		if strings.EqualFold(k, "Authorization") || strings.EqualFold(k, "Host") || strings.EqualFold(k, "Content-Length") {
			continue
		}
		for _, v := range values {
			dst.Add(k, v)
		}
	}
	if dst.Get("Content-Type") == "" {
		dst.Set("Content-Type", "application/json")
	}
}
