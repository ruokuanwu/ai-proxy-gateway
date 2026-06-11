package main

import (
	"context"
	"errors"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"ai-proxy-gateway/internal/auth"
	"ai-proxy-gateway/internal/config"
	"ai-proxy-gateway/internal/handler"
	"ai-proxy-gateway/internal/lb"
	"ai-proxy-gateway/internal/provider/openai"
	"ai-proxy-gateway/internal/runtime"
	"ai-proxy-gateway/internal/server"
)

func main() {
	configPath := flag.String("config", "configs/config.example.json", "path to config file")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		slog.Error("load config failed", "error", err)
		os.Exit(1)
	}

	names := make([]string, 0, len(cfg.Provider))
	for name, p := range cfg.Provider {
		if p.Enabled == nil || *p.Enabled {
			names = append(names, name)
		}
	}
	store := runtime.NewStore(names, cfg.Routing.ErrorWindow)
	balancer := lb.New(cfg.Routing.Strategy, store)
	authManager := auth.NewManager(cfg.Auth)
	adapter := openai.New(cfg.Server.UpstreamTimeout)
	h := handler.New(cfg, balancer, store, adapter)
	srv := server.New(cfg, authManager, h)

	go func() {
		slog.Info("gateway started", "addr", cfg.Server.Addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("server failed", "error", err)
			os.Exit(1)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		slog.Error("server shutdown failed", "error", err)
		os.Exit(1)
	}
	slog.Info("gateway stopped")
}
