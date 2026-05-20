package api

import "net/http"

// NewRouter создаёт HTTP-маршрутизатор.
func NewRouter(h *Handler) http.Handler {
	mux := http.NewServeMux()

	// API v1
	mux.HandleFunc("/api/v1/generate", h.HandleGenerate)
	mux.HandleFunc("/api/v1/geocode", h.HandleGeocode)
	mux.HandleFunc("/api/v1/health", h.HandleHealth)

	// Метрики
	mux.HandleFunc("/metrics", h.HandleMetrics)
	mux.HandleFunc("/api/v1/metrics", h.HandleMetrics)

	// Health check на корне
	mux.HandleFunc("/health", h.HandleHealth)

	// Создаем IP-ограничитель частоты запросов
	limiter := newIPRateLimiter(h.cfg.RateLimitRPM)

	// Обёртка с middleware
	return withMiddleware(mux, h.cfg, limiter)
}
