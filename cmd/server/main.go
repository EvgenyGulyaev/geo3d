package main

import (
	"fmt"
	"log"
	"net/http"

	"github.com/evgeny/3d-maps/internal/api"
	"github.com/evgeny/3d-maps/internal/cache"
	"github.com/evgeny/3d-maps/internal/config"
)

func main() {
	// Загрузка конфигурации из .env и переменных окружения
	cfg := config.Load()

	// Размер кэша из конфига
	modelCache := cache.New(cfg.CacheSize)

	// Создаём обработчик и роутер с настройками из конфига
	handler := api.NewHandler(
		modelCache,
		cfg.OverpassAPIURL,
		cfg.ElevationAPIURL,
		cfg.NominatimAPIURL,
	)
	router := api.NewRouter(handler)

	addr := fmt.Sprintf(":%s", cfg.Port)
	log.Printf("🏙️  3D Maps Generator starting on http://localhost%s", addr)
	log.Printf("📍 POST /api/v1/generate - Generate 3D model")
	log.Printf("🔍 GET  /api/v1/geocode?q=Moscow - Geocode city")
	log.Printf("❤️  GET  /api/v1/health - Health check")

	if err := http.ListenAndServe(addr, router); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}
