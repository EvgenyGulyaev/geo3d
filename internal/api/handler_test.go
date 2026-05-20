package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/evgeny/3d-maps/internal/cache"
	"github.com/evgeny/3d-maps/internal/config"
)

func TestHandleHealth(t *testing.T) {
	cfg := &config.Config{Port: "8080"}
	c := cache.New(10, "")
	h := NewHandler(c, cfg)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rr := httptest.NewRecorder()

	h.HandleHealth(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rr.Code)
	}

	var resp map[string]string
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp["status"] != "ok" {
		t.Errorf("expected status 'ok', got '%s'", resp["status"])
	}
}

func TestHandleMetrics(t *testing.T) {
	cfg := &config.Config{Port: "8080"}
	c := cache.New(10, "")
	h := NewHandler(c, cfg)

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rr := httptest.NewRecorder()

	h.HandleMetrics(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rr.Code)
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	keys := []string{"uptime_seconds", "total_requests", "active_requests", "cache_hits", "cache_misses", "goroutines", "mem_alloc_mb"}
	for _, key := range keys {
		if _, ok := resp[key]; !ok {
			t.Errorf("expected metric key '%s' to be present", key)
		}
	}
}

func TestAuthMiddleware(t *testing.T) {
	cfg := &config.Config{
		Port:   "8080",
		APIKey: "secret-token",
	}
	c := cache.New(10, "")
	h := NewHandler(c, cfg)
	router := NewRouter(h)

	// Case 1: No API Key provided
	req1 := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	rr1 := httptest.NewRecorder()
	router.ServeHTTP(rr1, req1)
	// /api/v1/health skips auth middleware, should return 200
	if rr1.Code != http.StatusOK {
		t.Errorf("expected bypass endpoint to return 200, got %d", rr1.Code)
	}

	// Case 2: Endpoint requiring auth (geocode) without key
	req2 := httptest.NewRequest(http.MethodGet, "/api/v1/geocode?q=Moscow", nil)
	rr2 := httptest.NewRecorder()
	router.ServeHTTP(rr2, req2)
	if rr2.Code != http.StatusUnauthorized {
		t.Errorf("expected unauthorized code 401, got %d", rr2.Code)
	}

	// Case 3: Endpoint with incorrect key
	req3 := httptest.NewRequest(http.MethodGet, "/api/v1/geocode?q=Moscow", nil)
	req3.Header.Set("X-API-Key", "wrong-token")
	rr3 := httptest.NewRecorder()
	router.ServeHTTP(rr3, req3)
	if rr3.Code != http.StatusUnauthorized {
		t.Errorf("expected unauthorized code 401, got %d", rr3.Code)
	}

	// Case 4: Endpoint with correct key in X-API-Key header
	req4 := httptest.NewRequest(http.MethodGet, "/api/v1/geocode?q=Moscow", nil)
	req4.Header.Set("X-API-Key", "secret-token")
	rr4 := httptest.NewRecorder()
	// Mock Nominatim response inside client is needed for full 200, but we can verify it doesn't fail with 401.
	// Since we hit the real internet or a timeout during geocode, let's just make sure it passes the auth stage.
	router.ServeHTTP(rr4, req4)
	if rr4.Code == http.StatusUnauthorized {
		t.Errorf("expected auth to pass, but got 401")
	}
}

func TestRateLimitMiddleware(t *testing.T) {
	cfg := &config.Config{
		Port:         "8080",
		RateLimitRPM: 1, // Only 1 request allowed per minute!
	}
	c := cache.New(10, "")
	h := NewHandler(c, cfg)
	router := NewRouter(h)

	// First request - allowed
	req1 := httptest.NewRequest(http.MethodGet, "/api/v1/geocode?q=Moscow", nil)
	req1.RemoteAddr = "1.2.3.4:1234"
	rr1 := httptest.NewRecorder()
	router.ServeHTTP(rr1, req1)
	if rr1.Code == http.StatusTooManyRequests {
		t.Errorf("expected first request to be allowed, but got 429")
	}

	// Second request within a minute - blocked
	req2 := httptest.NewRequest(http.MethodGet, "/api/v1/geocode?q=Moscow", nil)
	req2.RemoteAddr = "1.2.3.4:5678" // Same IP
	rr2 := httptest.NewRecorder()
	router.ServeHTTP(rr2, req2)
	if rr2.Code != http.StatusTooManyRequests {
		t.Errorf("expected rate limiter to return 429, got %d", rr2.Code)
	}

	// Request from a different IP - allowed
	req3 := httptest.NewRequest(http.MethodGet, "/api/v1/geocode?q=Moscow", nil)
	req3.RemoteAddr = "5.6.7.8:1234" // Different IP
	rr3 := httptest.NewRecorder()
	router.ServeHTTP(rr3, req3)
	if rr3.Code == http.StatusTooManyRequests {
		t.Errorf("expected request from new IP to be allowed, but got 429")
	}
}
