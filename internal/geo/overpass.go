package geo

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// OverpassClient — клиент для Overpass API (OpenStreetMap).
type OverpassClient struct {
	baseURL    string
	httpClient *http.Client
}

// NewOverpassClient создаёт новый клиент Overpass API.
func NewOverpassClient(baseURL string) *OverpassClient {
	if baseURL == "" {
		baseURL = "https://overpass-api.de/api/interpreter"
	}
	return &OverpassClient{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 120 * time.Second,
		},
	}
}

// overpassResponse — структура ответа Overpass API.
type overpassResponse struct {
	Elements []overpassElement `json:"elements"`
}

type overpassElement struct {
	Type     string            `json:"type"`
	ID       int64             `json:"id"`
	Lat      float64           `json:"lat,omitempty"`
	Lon      float64           `json:"lon,omitempty"`
	Tags     map[string]string `json:"tags,omitempty"`
	Nodes    []int64           `json:"nodes,omitempty"`
	Geometry []overpassNode    `json:"geometry,omitempty"`
	Members  []overpassMember  `json:"members,omitempty"`
}

type overpassNode struct {
	Lat float64 `json:"lat"`
	Lon float64 `json:"lon"`
}

type overpassMember struct {
	Type     string         `json:"type"`
	Ref      int64          `json:"ref"`
	Role     string         `json:"role"`
	Geometry []overpassNode `json:"geometry,omitempty"`
}

// FetchBuildings загружает здания в указанном BBox.
func (c *OverpassClient) FetchBuildings(bbox BBox) ([]Building, error) {
	query := fmt.Sprintf(`[out:json][timeout:90];
(
  way["building"](%.6f,%.6f,%.6f,%.6f);
);
out body geom;`, bbox.MinLat, bbox.MinLon, bbox.MaxLat, bbox.MaxLon)

	data, err := c.execute(query)
	if err != nil {
		return nil, fmt.Errorf("fetch buildings: %w", err)
	}

	var buildings []Building
	for _, el := range data.Elements {
		if el.Type != "way" || len(el.Geometry) < 3 {
			continue
		}

		b := Building{
			ID:   el.ID,
			Name: el.Tags["name"],
			Type: el.Tags["building"],
		}

		// Извлечение контура
		for _, node := range el.Geometry {
			b.Outline = append(b.Outline, Coord{Lat: node.Lat, Lon: node.Lon})
		}

		// Определение высоты
		b.Height, b.Levels = parseBuildingHeight(el.Tags)

		buildings = append(buildings, b)
	}

	return buildings, nil
}

// FetchRoads загружает дороги в указанном BBox.
func (c *OverpassClient) FetchRoads(bbox BBox) ([]Road, error) {
	query := fmt.Sprintf(`[out:json][timeout:90];
(
  way["highway"~"^(motorway|trunk|primary|secondary|tertiary|residential|unclassified|service)$"](%.6f,%.6f,%.6f,%.6f);
);
out body geom;`, bbox.MinLat, bbox.MinLon, bbox.MaxLat, bbox.MaxLon)

	data, err := c.execute(query)
	if err != nil {
		return nil, fmt.Errorf("fetch roads: %w", err)
	}

	var roads []Road
	for _, el := range data.Elements {
		if el.Type != "way" || len(el.Geometry) < 2 {
			continue
		}

		r := Road{
			ID:   el.ID,
			Name: el.Tags["name"],
			Type: el.Tags["highway"],
		}

		for _, node := range el.Geometry {
			r.Points = append(r.Points, Coord{Lat: node.Lat, Lon: node.Lon})
		}

		r.Width, r.Lanes = parseRoadWidth(el.Tags)
		roads = append(roads, r)
	}

	return roads, nil
}

// FetchWater загружает водоёмы в указанном BBox.
func (c *OverpassClient) FetchWater(bbox BBox) ([]WaterArea, error) {
	query := fmt.Sprintf(`[out:json][timeout:90];
(
  way["natural"="water"](%.6f,%.6f,%.6f,%.6f);
  way["waterway"~"^(river|stream|canal)$"](%.6f,%.6f,%.6f,%.6f);
);
out body geom;`, bbox.MinLat, bbox.MinLon, bbox.MaxLat, bbox.MaxLon,
		bbox.MinLat, bbox.MinLon, bbox.MaxLat, bbox.MaxLon)

	data, err := c.execute(query)
	if err != nil {
		return nil, fmt.Errorf("fetch water: %w", err)
	}

	var water []WaterArea
	for _, el := range data.Elements {
		if el.Type != "way" || len(el.Geometry) < 3 {
			continue
		}

		w := WaterArea{
			ID:   el.ID,
			Name: el.Tags["name"],
			Type: el.Tags["natural"],
		}
		if w.Type == "" {
			w.Type = el.Tags["waterway"]
		}

		for _, node := range el.Geometry {
			w.Outline = append(w.Outline, Coord{Lat: node.Lat, Lon: node.Lon})
		}

		water = append(water, w)
	}

	return water, nil
}

// execute выполняет запрос к Overpass API.
func (c *OverpassClient) execute(query string) (*overpassResponse, error) {
	body := strings.NewReader("data=" + query)
	req, err := http.NewRequest(http.MethodPost, c.baseURL, body)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", "3d-maps-generator/1.0")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		log.Printf("SMTP HTTP request failed: %v", err) // Added log
		return nil, fmt.Errorf("overpass request: %w", err)
	}
	defer resp.Body.Close()

	log.Printf("Overpass response status: %d", resp.StatusCode) // Added log
	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("overpass status %d: %s", resp.StatusCode, string(respBody))
	}

	var result overpassResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return &result, nil
}

// parseBuildingHeight извлекает высоту здания из тегов.
func parseBuildingHeight(tags map[string]string) (height float64, levels int) {
	// Сначала пробуем tag "height"
	if h, ok := tags["height"]; ok {
		h = strings.TrimSuffix(h, " m")
		h = strings.TrimSuffix(h, "m")
		if v, err := strconv.ParseFloat(strings.TrimSpace(h), 64); err == nil && v > 0 {
			return v, 0
		}
	}

	// Затем "building:levels"
	if l, ok := tags["building:levels"]; ok {
		if v, err := strconv.Atoi(strings.TrimSpace(l)); err == nil && v > 0 {
			return float64(v) * MetersPerLevel, v
		}
	}

	// По умолчанию
	return DefaultBuildingHeight, 3
}

// parseRoadWidth извлекает ширину дороги из тегов.
func parseRoadWidth(tags map[string]string) (width float64, lanes int) {
	// Пробуем tag "width"
	if w, ok := tags["width"]; ok {
		w = strings.TrimSuffix(w, " m")
		w = strings.TrimSuffix(w, "m")
		if v, err := strconv.ParseFloat(strings.TrimSpace(w), 64); err == nil && v > 0 {
			return v, 0
		}
	}

	// По количеству полос
	if l, ok := tags["lanes"]; ok {
		if v, err := strconv.Atoi(strings.TrimSpace(l)); err == nil && v > 0 {
			return float64(v) * 3.5, v // ~3.5м на полосу
		}
	}

	// Ширина по типу дороги
	switch tags["highway"] {
	case "motorway", "trunk":
		return 14.0, 4
	case "primary":
		return 10.5, 3
	case "secondary":
		return 7.0, 2
	case "tertiary":
		return 7.0, 2
	default:
		return DefaultRoadWidth, 2
	}
}
