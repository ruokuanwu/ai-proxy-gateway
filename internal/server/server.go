package server

import (
	"net/http"
	"strings"
	"time"

	"ai-proxy-gateway/internal/auth"
	"ai-proxy-gateway/internal/config"
	"ai-proxy-gateway/internal/handler"

	"github.com/gin-gonic/gin"
)

func New(cfg *config.Config, authManager *auth.Manager, h *handler.Handler) *http.Server {
	r := gin.New()
	r.Use(gin.Recovery(), accessLog())
	r.GET("/healthz", h.Healthz)
	r.GET("/readyz", h.Readyz)

	v1 := r.Group("/v1", authManager.Middleware())
	v1.GET("/models", h.Models)
	v1.POST("/chat/completions", h.ChatCompletions)

	admin := r.Group("/admin", adminAuth(cfg.Auth.AdminKey))
	admin.GET("/providers", h.Providers)

	return &http.Server{
		Addr:         cfg.Server.Addr,
		Handler:      r,
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
	}
}

func adminAuth(key string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if key == "" {
			c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"error": gin.H{"message": "admin api disabled", "type": "not_found", "code": "not_found"}})
			return
		}
		token := strings.TrimPrefix(c.GetHeader("Authorization"), "Bearer ")
		if token == "" {
			token = c.GetHeader("X-Admin-Key")
		}
		if token != key {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": gin.H{"message": "invalid admin key", "type": "auth_error", "code": "invalid_admin_key"}})
			return
		}
		c.Next()
	}
}

func accessLog() gin.HandlerFunc {
	return gin.LoggerWithFormatter(func(p gin.LogFormatterParams) string {
		return time.Now().Format(time.RFC3339) + " " + p.Method + " " + p.Path + " " + p.StatusCodeColor() + http.StatusText(p.StatusCode) + p.ResetColor() + " " + p.Latency.String() + "\n"
	})
}
