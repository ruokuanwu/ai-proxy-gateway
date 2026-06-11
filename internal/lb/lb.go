package lb

import (
	"context"
	"errors"
	"math/rand"
	"sort"
	"sync"

	"ai-proxy-gateway/internal/provider"
	"ai-proxy-gateway/internal/runtime"
)

type LoadBalancer struct {
	strategy string
	store    *runtime.Store
	mu       sync.Mutex
	counters map[string]int
}

func New(strategy string, store *runtime.Store) *LoadBalancer {
	return &LoadBalancer{strategy: strategy, store: store, counters: make(map[string]int)}
}

func (lb *LoadBalancer) Pick(ctx context.Context, model string, candidates []provider.Node, tried map[string]struct{}) (provider.Node, error) {
	_ = ctx
	available := make([]provider.Node, 0, len(candidates))
	for _, c := range candidates {
		if _, ok := tried[c.Name]; ok {
			continue
		}
		if lb.store.Snapshot(c.Name).Status == runtime.StatusOpen {
			continue
		}
		available = append(available, c)
	}
	if len(available) == 0 {
		return provider.Node{}, errors.New("no available provider")
	}
	switch lb.strategy {
	case "least_errors":
		return lb.pickLeastErrors(model, available), nil
	case "round_robin":
		return lb.pickRoundRobin(model, available), nil
	case "random":
		return available[rand.Intn(len(available))], nil
	default:
		return available[rand.Intn(len(available))], nil
	}
}

func (lb *LoadBalancer) pickRoundRobin(model string, candidates []provider.Node) provider.Node {
	lb.mu.Lock()
	defer lb.mu.Unlock()
	idx := lb.counters[model] % len(candidates)
	lb.counters[model]++
	return candidates[idx]
}

func (lb *LoadBalancer) pickLeastErrors(model string, candidates []provider.Node) provider.Node {
	sort.SliceStable(candidates, func(i, j int) bool {
		a := lb.store.Snapshot(candidates[i].Name)
		b := lb.store.Snapshot(candidates[j].Name)
		if a.WindowErrors != b.WindowErrors {
			return a.WindowErrors < b.WindowErrors
		}
		if a.ConsecutiveErrors != b.ConsecutiveErrors {
			return a.ConsecutiveErrors < b.ConsecutiveErrors
		}
		return a.LatencyMS < b.LatencyMS
	})
	bestErrors := lb.store.Snapshot(candidates[0].Name).WindowErrors
	ties := candidates[:0]
	for _, c := range candidates {
		if lb.store.Snapshot(c.Name).WindowErrors == bestErrors {
			ties = append(ties, c)
		}
	}
	return lb.pickRoundRobin(model, ties)
}
