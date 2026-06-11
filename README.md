# AI Proxy Gateway

轻量级 OpenAI-compatible AI 服务代理网关，基于 Go + Gin 实现。

## 功能

- AppKey 鉴权：`Authorization: Bearer <gateway_app_key>` 或 `X-App-Key`
- `/v1/models` 聚合模型列表
- `/v1/chat/completions` 非流式转发
- `/v1/chat/completions` SSE 流式透传
- 多 Provider 与同模型多 Provider 路由
- `round_robin`、`least_errors`、`random` 策略
- `429/5xx` 与网络错误失败重试
- Provider 基础错误统计与 `/admin/providers`
- 配置中的 `${ENV_NAME}` 环境变量展开

## 快速开始

准备配置：

```bash
cp configs/config.example.json config.json
export PROVIDER_A_API_KEY=sk-xxx
export PROVIDER_B_API_KEY=sk-yyy
```

启动：

```bash
go run ./cmd/gateway -config ./config.json
```

请求模型列表：

```bash
curl -H 'Authorization: Bearer gw_demo_key' http://127.0.0.1:8080/v1/models
```

请求 Chat Completions：

```bash
curl -N http://127.0.0.1:8080/v1/chat/completions \
  -H 'Authorization: Bearer gw_demo_key' \
  -H 'Content-Type: application/json' \
  -d '{"model":"gpt-5.5","messages":[{"role":"user","content":"hello"}]}'
```

查看 Provider 状态：

```bash
curl -H 'Authorization: Bearer admin_demo_key' http://127.0.0.1:8080/admin/providers
```

## 构建

```bash
CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o ai-proxy-gateway ./cmd/gateway
```

## 配置

参考 `configs/config.example.json`。Provider 兼容 `doc/example.json` 的核心字段，并扩展：

- `type`
- `enabled`
- `weight`
- `upstreamModel`
- `auth.appKeys`
- `routing.strategy`
- `routing.retry`
