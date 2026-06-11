package provider

import (
	"context"
	"io"
	"net/http"

	"ai-proxy-gateway/internal/config"
)

type Node struct {
	Name   string
	Config config.ProviderConfig
}

type ProxyRequest struct {
	Method        string
	Path          string
	Header        http.Header
	Body          []byte
	Model         string
	UpstreamModel string
	Stream        bool
}

type ProxyResponse struct {
	StatusCode int
	Header     http.Header
	Body       []byte
}

type StreamResponse struct {
	StatusCode int
	Header     http.Header
	Body       io.ReadCloser
	Cancel     context.CancelFunc
}

type Adapter interface {
	Do(ctx context.Context, node Node, req *ProxyRequest) (*ProxyResponse, error)
	Stream(ctx context.Context, node Node, req *ProxyRequest) (*StreamResponse, error)
}
