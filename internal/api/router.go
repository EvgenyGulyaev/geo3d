package api

import "net/http"

// NewRouter создаёт HTTP-маршрутизатор.
func NewRouter(h *Handler) http.Handler {
	mux := http.NewServeMux()

	// API v1
	mux.HandleFunc("/api/v1/generate", h.HandleGenerate)
	mux.HandleFunc("/api/v1/geocode", h.HandleGeocode)
	mux.HandleFunc("/api/v1/health", h.HandleHealth)

	// Health check на корне
	mux.HandleFunc("/health", h.HandleHealth)

	// Обёртка с middleware
	return withMiddleware(mux)
}
