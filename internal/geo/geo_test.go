package geo

import (
	"math"
	"testing"
)

func TestBBoxFromCenter(t *testing.T) {
	bbox := BBoxFromCenter(55.7558, 37.6173, 500, 500)

	if bbox.MinLat >= bbox.MaxLat {
		t.Fatalf("minLat should be less than maxLat: %f >= %f", bbox.MinLat, bbox.MaxLat)
	}
	if bbox.MinLon >= bbox.MaxLon {
		t.Fatalf("minLon should be less than maxLon: %f >= %f", bbox.MinLon, bbox.MaxLon)
	}

	// Проверяем примерный размер
	latDiffM := (bbox.MaxLat - bbox.MinLat) * 111320
	if math.Abs(latDiffM-500) > 1 {
		t.Fatalf("expected ~500m in latitude, got %f", latDiffM)
	}
}

func TestLatLonToMeters(t *testing.T) {
	center := Coord{Lat: 55.7558, Lon: 37.6173}

	// Та же точка → (0, 0)
	p := LatLonToMeters(center.Lat, center.Lon, center.Lat, center.Lon)
	if math.Abs(p.X) > 0.01 || math.Abs(p.Y) > 0.01 {
		t.Fatalf("center point should map to (0,0), got (%f, %f)", p.X, p.Y)
	}

	// Сдвиг на север
	p = LatLonToMeters(center.Lat+0.001, center.Lon, center.Lat, center.Lon)
	if p.Y < 100 { // ~111 метров
		t.Fatalf("expected ~111m north, got %f", p.Y)
	}
}

func TestParseBuildingHeight(t *testing.T) {
	tests := []struct {
		tags   map[string]string
		height float64
	}{
		{map[string]string{"height": "15"}, 15},
		{map[string]string{"height": "20 m"}, 20},
		{map[string]string{"building:levels": "5"}, 15},
		{map[string]string{}, DefaultBuildingHeight},
	}

	for _, tt := range tests {
		h, _ := parseBuildingHeight(tt.tags)
		if math.Abs(h-tt.height) > 0.01 {
			t.Errorf("tags=%v: expected height %f, got %f", tt.tags, tt.height, h)
		}
	}
}
