package config

import (
	"log"
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

// Config хранит конфигурацию приложения.
type Config struct {
	Port              string
	CacheSize         int
	OverpassAPIURL    string
	ElevationAPIURL   string
	NominatimAPIURL   string
	// SMTP settings
	SMTPHost          string
	SMTPPort          int
	SMTPUser          string
	SMTPPass          string
	SMTPFrom          string
}

// Load читает конфигурацию из .env файла и переменных окружения.
func Load() *Config {
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found or error loading it, using system environment variables")
	}

	cfg := &Config{
		Port:              getEnv("PORT", "8080"),
		CacheSize:         getEnvAsInt("CACHE_SIZE", 50),
		OverpassAPIURL:    getEnv("OVERPASS_API_URL", "https://overpass-api.de/api/interpreter"),
		ElevationAPIURL:   getEnv("ELEVATION_API_URL", "https://api.open-elevation.com/api/v1/lookup"),
		NominatimAPIURL:   getEnv("NOMINATIM_API_URL", "https://nominatim.openstreetmap.org"),
		SMTPHost:          getEnv("SMTP_HOST", ""),
		SMTPPort:          getEnvAsInt("SMTP_PORT", 587),
		SMTPUser:          getEnv("SMTP_USER", ""),
		SMTPPass:          getEnv("SMTP_PASS", ""),
		SMTPFrom:          getEnv("SMTP_FROM", ""),
	}

	return cfg
}

func getEnv(key, defaultVal string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return defaultVal
}

func getEnvAsInt(key string, defaultVal int) int {
	valueStr := getEnv(key, "")
	if value, err := strconv.Atoi(valueStr); err == nil {
		return value
	}
	return defaultVal
}
