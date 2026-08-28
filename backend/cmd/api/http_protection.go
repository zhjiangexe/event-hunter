package main

import (
	"context"
	"encoding/json"
	"math"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"event-hunter/backend/internal/platform/config"
)

type rateWindow struct {
	startedAt time.Time
	count     int
}

type fixedWindowLimiter struct {
	mu      sync.Mutex
	windows map[string]rateWindow
	limit   int
	window  time.Duration
	now     func() time.Time
}

func newFixedWindowLimiter(limit int, window time.Duration) *fixedWindowLimiter {
	return &fixedWindowLimiter{windows: make(map[string]rateWindow), limit: limit, window: window, now: time.Now}
}

func (limiter *fixedWindowLimiter) allow(key string) (bool, int, time.Duration) {
	limiter.mu.Lock()
	defer limiter.mu.Unlock()

	now := limiter.now()
	if len(limiter.windows) > 1024 {
		for address, candidate := range limiter.windows {
			if now.Sub(candidate.startedAt) >= limiter.window {
				delete(limiter.windows, address)
			}
		}
	}
	current, exists := limiter.windows[key]
	if !exists || now.Sub(current.startedAt) >= limiter.window {
		current = rateWindow{startedAt: now}
	}
	current.count++
	limiter.windows[key] = current
	remaining := max(0, limiter.limit-current.count)
	retryAfter := max(time.Second, limiter.window-now.Sub(current.startedAt))
	return current.count <= limiter.limit, remaining, retryAfter
}

func protectAPI(rawConfig config.Config, next http.Handler) http.Handler {
	cfg := rawConfig.WithDefaults()
	limiter := newFixedWindowLimiter(cfg.RateLimitRequests, cfg.RateLimitWindow)
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		ctx, cancel := context.WithTimeout(request.Context(), cfg.HTTPRequestTimeout)
		defer cancel()
		request = request.WithContext(ctx)

		if strings.HasPrefix(request.URL.Path, "/api/") {
			allowed, remaining, retryAfter := limiter.allow(remoteAddress(request))
			writer.Header().Set("RateLimit-Limit", strconv.Itoa(cfg.RateLimitRequests))
			writer.Header().Set("RateLimit-Remaining", strconv.Itoa(remaining))
			writer.Header().Set("RateLimit-Reset", strconv.Itoa(int(math.Ceil(retryAfter.Seconds()))))
			if !allowed {
				writer.Header().Set("Content-Type", "application/json")
				writer.Header().Set("Retry-After", strconv.Itoa(int(math.Ceil(retryAfter.Seconds()))))
				writer.WriteHeader(http.StatusTooManyRequests)
				_ = json.NewEncoder(writer).Encode(map[string]string{"code": "RATE_LIMIT_EXCEEDED"})
				return
			}
		}
		next.ServeHTTP(writer, request)
	})
}

func remoteAddress(request *http.Request) string {
	host, _, err := net.SplitHostPort(request.RemoteAddr)
	if err == nil && host != "" {
		return host
	}
	if request.RemoteAddr != "" {
		return request.RemoteAddr
	}
	return "unknown"
}
