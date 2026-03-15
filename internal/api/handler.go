package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/evgeny/3d-maps/internal/cache"
	"github.com/evgeny/3d-maps/internal/generator"
	"github.com/evgeny/3d-maps/internal/geo"
)

// Handler содержит зависимости для HTTP-обработчиков.
type Handler struct {
	overpass  *geo.OverpassClient
	elevation *geo.ElevationClient
	nominatim *geo.NominatimClient
	cache     *cache.LRU
}

// NewHandler создаёт обработчик.
func NewHandler(c *cache.LRU) *Handler {
	return &Handler{
		overpass:  geo.NewOverpassClient(),
		elevation: geo.NewElevationClient(),
		nominatim: geo.NewNominatimClient(),
		cache:     c,
	}
}

// HandleGenerate обрабатывает POST /api/v1/generate.
func (h *Handler) HandleGenerate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var req geo.GenerateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json: "+err.Error())
		return
	}

	// Валидация
	if err := validateRequest(&req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Если указан город, но нет координат — геокодируем
	if req.Lat == 0 && req.Lon == 0 && req.City != "" {
		result, err := h.nominatim.Geocode(req.City)
		if err != nil {
			writeError(w, http.StatusBadRequest, "geocode error: "+err.Error())
			return
		}
		req.Lat = result.Lat
		req.Lon = result.Lon
	}

	// Ключ кэша
	cacheKey := fmt.Sprintf("%.5f_%.5f_%.0f_%.0f_%s_%v_%v_%v_%.6f_%.1f",
		req.Lat, req.Lon, req.WidthM, req.HeightM, req.Format,
		req.IncludeTerrain, req.IncludeRoads, req.PrintReady,
		req.Scale, req.BaseThickness)

	// Проверяем кэш
	if data, ok := h.cache.Get(cacheKey); ok {
		log.Printf("Cache hit for %s", cacheKey)
		writeModelResponse(w, data, req.Format)
		return
	}

	start := time.Now()
	log.Printf("Generating model: lat=%.5f lon=%.5f size=%.0fx%.0f format=%s",
		req.Lat, req.Lon, req.WidthM, req.HeightM, req.Format)

	// Создаём BBox
	bbox := geo.BBoxFromCenter(req.Lat, req.Lon, req.WidthM, req.HeightM)

	// === Загрузка данных ===

	// Здания
	buildings, err := h.overpass.FetchBuildings(bbox)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "fetch buildings: "+err.Error())
		return
	}
	log.Printf("Fetched %d buildings", len(buildings))

	// Дороги
	var roads []geo.Road
	if req.IncludeRoads {
		roads, err = h.overpass.FetchRoads(bbox)
		if err != nil {
			log.Printf("Warning: fetch roads failed: %v", err)
		} else {
			log.Printf("Fetched %d roads", len(roads))
		}
	}

	// === Генерация 3D ===
	scene := generator.NewScene()

	// Земля
	if req.IncludeTerrain {
		grid, err := h.elevation.FetchElevationGrid(bbox, 20)
		if err != nil {
			log.Printf("Warning: fetch elevation failed: %v, using flat ground", err)
			scene.AddMesh(generator.GenerateFlatGround(req.WidthM, req.HeightM))
		} else {
			scene.AddMesh(generator.GenerateTerrain(grid, req.Lat, req.Lon))
		}
	} else {
		scene.AddMesh(generator.GenerateFlatGround(req.WidthM, req.HeightM))
	}

	// Дороги
	if req.IncludeRoads {
		for _, m := range generator.GenerateRoads(roads, req.Lat, req.Lon) {
			scene.AddMesh(m)
		}
	}

	// Здания
	for _, m := range generator.GenerateBuildings(buildings, req.Lat, req.Lon) {
		scene.AddMesh(m)
	}

	log.Printf("Scene: %d vertices, %d triangles", scene.TotalVertices(), scene.TotalTriangles())

	// === Подготовка к 3D-печати ===
	if req.PrintReady {
		opts := generator.PrintOptions{
			Scale:         req.Scale,
			BaseThickness: req.BaseThickness,
			MinWallMM:     req.MinWall,
		}
		scene = generator.PrepareForPrint(scene, req.WidthM, req.HeightM, opts)
		log.Printf("Print-ready: scale=%.6f, base=%.1fmm, scene: %d vertices, %d triangles",
			req.Scale, req.BaseThickness, scene.TotalVertices(), scene.TotalTriangles())
	}

	// === Экспорт ===
	var buf bytes.Buffer
	switch generator.ExportFormat(req.Format) {
	case generator.FormatOBJ:
		if err := generator.ExportOBJ(scene, &buf); err != nil {
			writeError(w, http.StatusInternalServerError, "export obj: "+err.Error())
			return
		}
	case generator.FormatSTL:
		if err := generator.ExportSTL(scene, &buf); err != nil {
			writeError(w, http.StatusInternalServerError, "export stl: "+err.Error())
			return
		}
	default:
		if err := generator.ExportGLB(scene, &buf); err != nil {
			writeError(w, http.StatusInternalServerError, "export glb: "+err.Error())
			return
		}
	}

	data := buf.Bytes()
	log.Printf("Generated %d bytes in %v", len(data), time.Since(start))

	// Кэшируем
	h.cache.Set(cacheKey, data)

	writeModelResponse(w, data, req.Format)
}

// HandleGeocode обрабатывает GET /api/v1/geocode.
func (h *Handler) HandleGeocode(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	query := r.URL.Query().Get("q")
	if query == "" {
		writeError(w, http.StatusBadRequest, "missing 'q' parameter")
		return
	}

	result, err := h.nominatim.Geocode(query)
	if err != nil {
		writeError(w, http.StatusBadRequest, "geocode error: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, result)
}

// HandleHealth — проверка статуса сервера.
func (h *Handler) HandleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{
		"status": "ok",
		"time":   time.Now().Format(time.RFC3339),
	})
}

func validateRequest(req *geo.GenerateRequest) error {
	if req.Lat == 0 && req.Lon == 0 && req.City == "" {
		return fmt.Errorf("specify 'city' or 'lat'/'lon'")
	}
	if req.WidthM <= 0 {
		req.WidthM = 500
	}
	if req.HeightM <= 0 {
		req.HeightM = 500
	}
	if req.WidthM > 2000 {
		return fmt.Errorf("width must be <= 2000 meters")
	}
	if req.HeightM > 2000 {
		return fmt.Errorf("height must be <= 2000 meters")
	}
	if req.Format == "" {
		if req.PrintReady {
			req.Format = "stl"
		} else {
			req.Format = "glb"
		}
	}
	if req.Format != "glb" && req.Format != "obj" && req.Format != "stl" {
		return fmt.Errorf("format must be 'glb', 'obj', or 'stl'")
	}
	// Дефолты для 3D-печати
	if req.PrintReady {
		if req.Scale <= 0 {
			req.Scale = 1.0 // 1 метр → 1 мм
		}
		if req.BaseThickness <= 0 {
			req.BaseThickness = 3.0 // 3 мм
		}
		if req.MinWall <= 0 {
			req.MinWall = 0.8 // 0.8 мм
		}
	}
	return nil
}

func writeModelResponse(w http.ResponseWriter, data []byte, format string) {
	switch format {
	case "obj":
		w.Header().Set("Content-Type", "text/plain")
		w.Header().Set("Content-Disposition", "attachment; filename=model.obj")
	case "stl":
		w.Header().Set("Content-Type", "application/sla")
		w.Header().Set("Content-Disposition", "attachment; filename=model.stl")
	default:
		w.Header().Set("Content-Type", "model/gltf-binary")
		w.Header().Set("Content-Disposition", "attachment; filename=model.glb")
	}
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(data)))
	w.Write(data)
}

func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
