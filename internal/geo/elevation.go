package geo

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// ElevationClient — клиент для Open Elevation API.
type ElevationClient struct {
	baseURL    string
	httpClient *http.Client
}

// NewElevationClient создаёт клиент Open Elevation API.
func NewElevationClient(baseURL string) *ElevationClient {
	if baseURL == "" {
		baseURL = "https://api.open-elevation.com/api/v1/lookup"
	}
	return &ElevationClient{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

type elevationRequest struct {
	Locations []elevationLocation `json:"locations"`
}

type elevationLocation struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
}

type elevationResponse struct {
	Results []elevationResult `json:"results"`
}

type elevationResult struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
	Elevation float64 `json:"elevation"`
}

// FetchElevationGrid загружает сетку высот для заданного BBox.
// gridSize — количество точек по каждой оси (gridSize x gridSize).
func (c *ElevationClient) FetchElevationGrid(bbox BBox, gridSize int) (*ElevationGrid, error) {
	if gridSize < 2 {
		gridSize = 2
	}
	if gridSize > 50 {
		gridSize = 50 // ограничиваем, чтобы не перегрузить API
	}

	latStep := (bbox.MaxLat - bbox.MinLat) / float64(gridSize-1)
	lonStep := (bbox.MaxLon - bbox.MinLon) / float64(gridSize-1)

	// Формируем точки сетки
	var locations []elevationLocation
	for row := 0; row < gridSize; row++ {
		for col := 0; col < gridSize; col++ {
			locations = append(locations, elevationLocation{
				Latitude:  bbox.MinLat + float64(row)*latStep,
				Longitude: bbox.MinLon + float64(col)*lonStep,
			})
		}
	}

	// Отправляем запрос батчами (API ограничение)
	batchSize := 100
	allResults := make([]float64, 0, len(locations))

	for i := 0; i < len(locations); i += batchSize {
		end := i + batchSize
		if end > len(locations) {
			end = len(locations)
		}

		batch := locations[i:end]
		reqBody := elevationRequest{Locations: batch}
		jsonData, err := json.Marshal(reqBody)
		if err != nil {
			return nil, fmt.Errorf("marshal request: %w", err)
		}

		req, err := http.NewRequest(http.MethodPost, c.baseURL, bytes.NewReader(jsonData))
		if err != nil {
			return nil, fmt.Errorf("create request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("User-Agent", "3d-maps-generator/1.0")

		resp, err := c.httpClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("elevation request: %w", err)
		}

		var result elevationResponse
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			resp.Body.Close()
			return nil, fmt.Errorf("decode response: %w", err)
		}
		resp.Body.Close()

		for _, r := range result.Results {
			allResults = append(allResults, r.Elevation)
		}

		// Пауза между батчами
		if end < len(locations) {
			time.Sleep(200 * time.Millisecond)
		}
	}

	// Вычисляем размер ячейки в метрах
	centerLat := (bbox.MinLat + bbox.MaxLat) / 2
	cellSizeM := latStep * 111320.0 // приблизительно
	_ = centerLat

	return &ElevationGrid{
		Width:     gridSize,
		Height:    gridSize,
		CellSizeM: cellSizeM,
		Points:    allResults,
		OriginLat: bbox.MinLat,
		OriginLon: bbox.MinLon,
	}, nil
}
