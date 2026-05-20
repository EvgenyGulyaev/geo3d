package api

import (
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/evgeny/3d-maps/internal/config"
)

// withMiddleware оборачивает обработчик middleware-цепочкой.
func withMiddleware(next http.Handler, cfg *config.Config, limiter *ipRateLimiter) http.Handler {
	return corsMiddleware(
		loggingMiddleware(
			authMiddleware(cfg)(
				rateLimitMiddleware(limiter)(next),
			),
		),
	)
}

// loggingMiddleware логирует HTTP-запросы.
func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Обёртка для захвата статус-кода
		rw := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}
		next.ServeHTTP(rw, r)

		slog.Info("HTTP request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", rw.statusCode,
			"duration", time.Since(start),
		)
	})
}

// corsMiddleware добавляет CORS заголовки.
func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-API-Key")
		w.Header().Set("Access-Control-Max-Age", "86400")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// authMiddleware проверяет API-ключ, если он настроен.
func authMiddleware(cfg *config.Config) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Пропускаем служебные эндпоинты
			if r.URL.Path == "/health" || r.URL.Path == "/metrics" || r.URL.Path == "/api/v1/health" {
				next.ServeHTTP(w, r)
				return
			}

			if cfg.APIKey == "" {
				next.ServeHTTP(w, r)
				return
			}

			key := r.Header.Get("X-API-Key")
			if key == "" {
				authHeader := r.Header.Get("Authorization")
				if strings.HasPrefix(authHeader, "Bearer ") {
					key = strings.TrimPrefix(authHeader, "Bearer ")
				}
			}

			if key != cfg.APIKey {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				w.Write([]byte(`{"error":"unauthorized: invalid or missing API key"}`))
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// rateLimitMiddleware ограничивает частоту запросов по IP.
func rateLimitMiddleware(limiter *ipRateLimiter) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Пропускаем служебные эндпоинты
			if r.URL.Path == "/health" || r.URL.Path == "/metrics" || r.URL.Path == "/api/v1/health" {
				next.ServeHTTP(w, r)
				return
			}

			ip := getClientIP(r)
			if !limiter.allow(ip) {
				slog.Warn("Rate limit exceeded", "ip", ip, "path", r.URL.Path)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusTooManyRequests)
				w.Write([]byte(`{"error":"too many requests: rate limit exceeded"}`))
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// ipRateLimiter реализует ограничение частоты запросов на базе скользящего окна.
type ipRateLimiter struct {
	mu      sync.Mutex
	clients map[string][]time.Time
	rpm     int
}

// newIPRateLimiter создает новый ограничитель частоты запросов.
func newIPRateLimiter(rpm int) *ipRateLimiter {
	limiter := &ipRateLimiter{
		clients: make(map[string][]time.Time),
		rpm:     rpm,
	}
	limiter.startJanitor()
	return limiter
}

func (l *ipRateLimiter) allow(ip string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	oneMinuteAgo := now.Add(-1 * time.Minute)

	var active []time.Time
	for _, t := range l.clients[ip] {
		if t.After(oneMinuteAgo) {
			active = append(active, t)
		}
	}

	if len(active) >= l.rpm {
		l.clients[ip] = active
		return false
	}

	active = append(active, now)
	l.clients[ip] = active
	return true
}

func (l *ipRateLimiter) startJanitor() {
	ticker := time.NewTicker(5 * time.Minute)
	go func() {
		for range ticker.C {
			l.mu.Lock()
			now := time.Now()
			oneMinuteAgo := now.Add(-1 * time.Minute)
			for ip, reqs := range l.clients {
				var active []time.Time
				for _, t := range reqs {
					if t.After(oneMinuteAgo) {
						active = append(active, t)
					}
				}
				if len(active) == 0 {
					delete(l.clients, ip)
				} else {
					l.clients[ip] = active
				}
			}
			l.mu.Unlock()
		}
	}()
}

func getClientIP(r *http.Request) string {
	xff := r.Header.Get("X-Forwarded-For")
	if xff != "" {
		parts := strings.Split(xff, ",")
		return strings.TrimSpace(parts[0])
	}
	xri := r.Header.Get("X-Real-IP")
	if xri != "" {
		return strings.TrimSpace(xri)
	}
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return ip
}

type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}
