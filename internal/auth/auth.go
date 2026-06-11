package auth

import (
	"net/http"
	"strings"

	"ai-proxy-gateway/internal/config"

	"github.com/gin-gonic/gin"
)

const ContextKey = "app_key"

type AppKey struct {
	Name    string
	Key     string
	Enabled bool
	Models  map[string]struct{}
}

type Manager struct {
	keys map[string]AppKey
}

func NewManager(cfg config.AuthConfig) *Manager {
	keys := make(map[string]AppKey, len(cfg.AppKeys))
	for _, k := range cfg.AppKeys {
		models := make(map[string]struct{}, len(k.Models))
		for _, m := range k.Models {
			models[m] = struct{}{}
		}
		keys[k.Key] = AppKey{Name: k.Name, Key: k.Key, Enabled: k.Enabled, Models: models}
	}
	return &Manager{keys: keys}
}

func (m *Manager) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		key := bearerToken(c.GetHeader("Authorization"))
		if key == "" {
			key = c.GetHeader("X-App-Key")
		}
		if key == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": gin.H{"message": "missing app key", "type": "auth_error", "code": "missing_app_key"}})
			return
		}
		appKey, ok := m.keys[key]
		if !ok {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": gin.H{"message": "invalid app key", "type": "auth_error", "code": "invalid_app_key"}})
			return
		}
		if !appKey.Enabled {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": gin.H{"message": "disabled app key", "type": "auth_error", "code": "disabled_app_key"}})
			return
		}
		c.Set(ContextKey, appKey)
		c.Next()
	}
}

func Current(c *gin.Context) (AppKey, bool) {
	v, ok := c.Get(ContextKey)
	if !ok {
		return AppKey{}, false
	}
	appKey, ok := v.(AppKey)
	return appKey, ok
}

func (k AppKey) Allows(model string) bool {
	if len(k.Models) == 0 {
		return true
	}
	_, ok := k.Models[model]
	return ok
}

func bearerToken(header string) string {
	parts := strings.Fields(header)
	if len(parts) == 2 && strings.EqualFold(parts[0], "Bearer") {
		return parts[1]
	}
	return ""
}
