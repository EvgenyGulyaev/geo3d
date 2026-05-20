package api

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"runtime"
	"sync/atomic"
	"time"

	"github.com/evgeny/3d-maps/internal/cache"
	"github.com/evgeny/3d-maps/internal/config"
	"github.com/evgeny/3d-maps/internal/generator"
	"github.com/evgeny/3d-maps/internal/geo"
	"github.com/evgeny/3d-maps/internal/mail"
	"github.com/evgeny/3d-maps/internal/math2d"
)

var startTime = time.Now()

type metricsRegistry struct {
	totalRequests  uint64
	activeRequests int64
	cacheHits      uint64
	cacheMisses    uint64
}

// Handler содержит зависимости для HTTP-обработчиков.
type Handler struct {
	overpass  *geo.OverpassClient
	elevation *geo.ElevationClient
	nominatim *geo.NominatimClient
	cache     *cache.LRU
	mail      *mail.Mailer
	cfg       *config.Config
	metrics   metricsRegistry
}

// NewHandler создаёт обработчик.
func NewHandler(c *cache.LRU, cfg *config.Config) *Handler {
	return &Handler{
		overpass:  geo.NewOverpassClient(cfg.OverpassAPIURL),
		elevation: geo.NewElevationClient(cfg.ElevationAPIURL),
		nominatim: geo.NewNominatimClient(cfg.NominatimAPIURL),
		cache:     c,
		mail:      mail.NewMailer(cfg),
		cfg:       cfg,
	}
}

// HandleGenerate обрабатывает POST /api/v1/generate.
func (h *Handler) HandleGenerate(w http.ResponseWriter, r *http.Request) {
	atomic.AddUint64(&h.metrics.totalRequests, 1)
	atomic.AddInt64(&h.metrics.activeRequests, 1)
	defer atomic.AddInt64(&h.metrics.activeRequests, -1)

	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var req geo.GenerateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json: "+err.Error())
		return
	}

	slog.Info("Incoming request", "req", req)

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
	cacheKey := fmt.Sprintf("%.5f_%.5f_%.0f_%.0f_%s_%v_%v_%v_%.6f_%.1f_%v_%.1f_%v_%.1f",
		req.Lat, req.Lon, req.WidthM, req.HeightM, req.Format,
		req.IncludeTerrain, req.IncludeRoads, req.PrintReady,
		req.Scale, req.BaseThickness, req.SplitBoard, req.BoardSizeMM,
		req.MergeTiles, req.MergeGapMM)

	// Проверяем кэш
	if data, ok := h.cache.Get(cacheKey); ok {
		atomic.AddUint64(&h.metrics.cacheHits, 1)
		slog.Info("Cache hit", "key", cacheKey)
		effectiveFormat := req.Format
		if req.SplitBoard {
			effectiveFormat = "zip"
		}
		writeModelResponse(w, data, effectiveFormat)
		return
	}

	atomic.AddUint64(&h.metrics.cacheMisses, 1)

	if req.Email != "" {
		// Асинхронная обработка
		go h.processGenerateAsync(req, cacheKey)
		writeJSON(w, http.StatusAccepted, map[string]string{
			"message": "Generation started. You will receive an email once it is finished.",
			"status":  "processing",
		})
		return
	}

	resultData, resultFormat, err := h.generateModelSync(req, cacheKey)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeModelResponse(w, resultData, resultFormat)
}

func (h *Handler) processGenerateAsync(req geo.GenerateRequest, cacheKey string) {
	data, format, err := h.generateModelSync(req, cacheKey)
	if err != nil {
		slog.Error("Async generation error", "error", err)
		return
	}

	filename := "model." + format
	if format == "zip" {
		filename = "3d_model_tiles.zip"
	}

	err = h.mail.SendModelEmail(req.Email, filename, data)
	if err != nil {
		slog.Error("Failed to send email", "email", req.Email, "error", err)
	} else {
		slog.Info("Email successfully sent", "email", req.Email)
	}
}

func (h *Handler) generateModelSync(req geo.GenerateRequest, cacheKey string) (resultData []byte, resultFormat string, err error) {
	start := time.Now()
	// Проверяем кэш еще раз (на случай если за время пока мы думали он там появился)
	if data, ok := h.cache.Get(cacheKey); ok {
		slog.Info("Cache hit in sync worker", "key", cacheKey)
		effectiveFormat := req.Format
		if req.SplitBoard && !req.MergeTiles {
			effectiveFormat = "zip"
		}
		return data, effectiveFormat, nil
	}

	slog.Info("Generating model",
		"lat", req.Lat,
		"lon", req.Lon,
		"width_m", req.WidthM,
		"height_m", req.HeightM,
		"format", req.Format)

	// Создаём BBox
	bbox := geo.BBoxFromCenter(req.Lat, req.Lon, req.WidthM, req.HeightM)

	// === Загрузка данных ===

	// Здания
	buildings, err := h.overpass.FetchBuildings(bbox)
	if err != nil {
		return nil, "", fmt.Errorf("fetch buildings: %w", err)
	}
	slog.Info("Fetched buildings", "count", len(buildings))

	// Дороги
	var roads []geo.Road
	if req.IncludeRoads {
		roads, err = h.overpass.FetchRoads(bbox)
		if err != nil {
			slog.Warn("Fetch roads failed", "error", err)
		} else {
			slog.Info("Fetched roads", "count", len(roads))
		}
	}

	// === Генерация ===
	resultFormat = req.Format

	// Если запрошено разделение на платы
	if req.SplitBoard && req.BoardSizeMM > 0 {
		slog.Info("Branch: SPLIT BOARD", "board_size_mm", req.BoardSizeMM)
		// BoardSizeMM - размер платы в мм. Scale: например, 0.002 = 2мм на 1метр.
		// Значит, 1 плата в физическом мире покроет: BoardSizeMM / Scale (метров из геометрии)
		baseScale := req.Scale
		if baseScale <= 0 {
			baseScale = 1.0 // safeguard
		}
		
		tileSizeMeters := req.BoardSizeMM / (baseScale * 1000.0)
		
		numX := int(math.Ceil(req.WidthM / tileSizeMeters))
		numY := int(math.Ceil(req.HeightM / tileSizeMeters))

		slog.Info("Splitting into tiles",
			"num_x", numX,
			"num_y", numY,
			"tile_size_meters", tileSizeMeters,
			"board_size_mm", req.BoardSizeMM,
			"scale", baseScale)

		var zipBuf bytes.Buffer
		zipWriter := zip.NewWriter(&zipBuf)
		
		mergedScene := generator.NewScene()
		validTilesCount := 0
		gapMM := req.MergeGapMM
		if gapMM <= 0 {
			gapMM = 10.0
		}
		
		// Рельеф (скачиваем один раз на всю область, если нужно)
		var _ *geo.ElevationGrid
		if req.IncludeTerrain {
			var err error
			_, err = h.elevation.FetchElevationGrid(bbox, 20)
			if err != nil {
				slog.Warn("Fetch elevation failed for split", "error", err)
			}
		}

		// Генерация каждого тайла
		for y := 0; y < numY; y++ {
			for x := 0; x < numX; x++ {
				startX := -req.WidthM/2.0 + float64(x)*tileSizeMeters
				startY := -req.HeightM/2.0 + float64(y)*tileSizeMeters
				endX := startX + tileSizeMeters
				endY := startY + tileSizeMeters

				clipRect := math2d.Rect{MinX: startX, MinY: startY, MaxX: endX, MaxY: endY}
				scene := generator.NewScene()
				
				if req.IncludeRoads {
					for _, m := range generator.GenerateRoads(roads, req.Lat, req.Lon, &clipRect) {
						if m != nil { scene.AddMesh(m) }
					}
				}

				buildingsAdded := 0
				for _, m := range generator.GenerateBuildings(buildings, req.Lat, req.Lon, &clipRect, req.HeightMultiplier) {
					if m != nil {
						scene.AddMesh(m)
						buildingsAdded++
					}
				}

				if buildingsAdded == 0 && !req.IncludeRoads {
					continue
				}

				scene.AddMesh(generator.GenerateFlatGroundFromRect(clipRect.MinX, clipRect.MinY, clipRect.MaxX, clipRect.MaxY))

				if req.PrintReady {
					opts := generator.PrintOptions{
						Scale: req.Scale,
						BaseThickness: req.BaseThickness,
						MinWallMM: req.MinWall,
					}
					// Сдвигаем к 0,0 для отдельной плитки
					offsetX := -(startX + tileSizeMeters/2.0)
					offsetY := -(startY + tileSizeMeters/2.0)
					shiftScene(scene, float32(offsetX), float32(offsetY))
					scene = generator.PrepareForPrint(scene, tileSizeMeters, tileSizeMeters, opts)
				}
				
				if req.MergeTiles {
					// Сдвигаем плитку в сетку для общего файла
					// xMM = x * (boardSize + gap)
					gridX := float32(float64(x) * (req.BoardSizeMM + gapMM))
					gridY := float32(float64(y) * (req.BoardSizeMM + gapMM))
					shiftScene(scene, gridX, gridY)
					for _, m := range scene.Meshes {
						mergedScene.AddMesh(m)
					}
				} else {
					filename := fmt.Sprintf("tile_%d_%d.%s", x, y, req.Format)
					fWriter, _ := zipWriter.Create(filename)
					var buf bytes.Buffer
					if req.Format == "stl" { generator.ExportSTL(scene, &buf) } else { generator.ExportGLB(scene, &buf) }
					fWriter.Write(buf.Bytes())
				}
				validTilesCount++
			}
		}
		
		if req.MergeTiles {
			var buf bytes.Buffer
			generator.ExportSTL(mergedScene, &buf)
			resultData = buf.Bytes()
			resultFormat = "stl"
		} else {
			if validTilesCount > 0 {
				svgWriter, _ := zipWriter.Create("layout_map.svg")
				svgWriter.Write([]byte(generateLayoutSVG(numX, numY, tileSizeMeters, req.BoardSizeMM)))
			}
			zipWriter.Close()
			resultData = zipBuf.Bytes()
			resultFormat = "zip"
		}
		
		slog.Info("Final format",
			"format", resultFormat,
			"merged", req.MergeTiles,
			"tiles_count", validTilesCount)

	} else {
		slog.Info("Branch: SINGLE MODEL",
			"split", req.SplitBoard,
			"size", req.BoardSizeMM)
		// Обычная генерация единой модели...
		scene := generator.NewScene()
		
		if req.IncludeTerrain {
			grid, err := h.elevation.FetchElevationGrid(bbox, 20)
			if err != nil {
				scene.AddMesh(generator.GenerateFlatGround(req.WidthM, req.HeightM))
			} else {
				scene.AddMesh(generator.GenerateTerrain(grid, req.Lat, req.Lon))
			}
		} else {
			scene.AddMesh(generator.GenerateFlatGround(req.WidthM, req.HeightM))
		}

		if req.IncludeRoads {
			for _, m := range generator.GenerateRoads(roads, req.Lat, req.Lon, nil) {
				scene.AddMesh(m)
			}
		}

		for _, m := range generator.GenerateBuildings(buildings, req.Lat, req.Lon, nil, req.HeightMultiplier) {
			scene.AddMesh(m)
		}

		if req.PrintReady {
			opts := generator.PrintOptions{
				Scale:         req.Scale,
				BaseThickness: req.BaseThickness,
				MinWallMM:     req.MinWall,
			}
			scene = generator.PrepareForPrint(scene, req.WidthM, req.HeightM, opts)
		}

		var buf bytes.Buffer
		switch req.Format {
		case "obj":
			generator.ExportOBJ(scene, &buf)
		case "stl":
			generator.ExportSTL(scene, &buf)
		default:
			generator.ExportGLB(scene, &buf)
		}
		resultData = buf.Bytes()
	}

	// Кэшируем
	h.cache.Set(cacheKey, resultData)
	slog.Info("Model generated and cached", "duration", time.Since(start))
	return resultData, resultFormat, nil
}

func shiftScene(scene *generator.Scene, offsetX, offsetY float32) {
	for _, m := range scene.Meshes {
		for i := 0; i < len(m.Vertices); i += 3 {
			m.Vertices[i] += offsetX
			m.Vertices[i+2] += offsetY
		}
	}
}

func generateLayoutSVG(numX, numY int, tileSizeMeters float64, boardSizeMM float64) string {
	svg := fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %d %d">
	<style>
		.tile { fill: #f0f0f0; stroke: #333; stroke-width: 2; }
		.text { font-family: sans-serif; font-size: 14px; text-anchor: middle; dominant-baseline: middle; fill: #333; }
		.title { font-family: sans-serif; font-size: 20px; font-weight: bold; }
	</style>
	<text x="20" y="30" class="title">Схема склейки плат (%dx%d)</text>
	<text x="20" y="55" font-family="sans-serif" font-size="14px">Размер одной платы: %.1f мм</text>
	<g transform="translate(20, 80)">
`, numX*100+40, numY*100+100, numX, numY, boardSizeMM)

	for y := 0; y < numY; y++ {
		for x := 0; x < numX; x++ {
			// Инвертируем Y для привычного 2D отображения (сверху-вниз на SVG, снизу-вверх на карте)
			drawY := (numY - 1 - y) * 100
			drawX := x * 100
			
			svg += fmt.Sprintf(`		<rect x="%d" y="%d" width="100" height="100" class="tile" />
		<text x="%d" y="%d" class="text">%d_%d</text>
`, drawX, drawY, drawX+50, drawY+50, x, y)
		}
	}

	svg += "\n\t</g>\n</svg>"
	return svg
}

// HandleGeocode обрабатывает GET /api/v1/geocode.
func (h *Handler) HandleGeocode(w http.ResponseWriter, r *http.Request) {
	atomic.AddUint64(&h.metrics.totalRequests, 1)
	atomic.AddInt64(&h.metrics.activeRequests, 1)
	defer atomic.AddInt64(&h.metrics.activeRequests, -1)

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

// HandleMetrics возвращает метрики приложения в формате JSON.
func (h *Handler) HandleMetrics(w http.ResponseWriter, r *http.Request) {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	metricsData := map[string]interface{}{
		"uptime_seconds":  time.Since(startTime).Seconds(),
		"total_requests":  atomic.LoadUint64(&h.metrics.totalRequests),
		"active_requests": atomic.LoadInt64(&h.metrics.activeRequests),
		"cache_hits":      atomic.LoadUint64(&h.metrics.cacheHits),
		"cache_misses":    atomic.LoadUint64(&h.metrics.cacheMisses),
		"goroutines":      runtime.NumGoroutine(),
		"mem_alloc_mb":    float64(m.Alloc) / 1024 / 1024,
		"mem_sys_mb":      float64(m.Sys) / 1024 / 1024,
		"mem_num_gc":      m.NumGC,
	}

	writeJSON(w, http.StatusOK, metricsData)
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
	if req.WidthM > 15000 {
		return fmt.Errorf("width must be <= 15000 meters")
	}
	if req.HeightM > 15000 {
		return fmt.Errorf("height must be <= 15000 meters")
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
	case "zip":
		w.Header().Set("Content-Type", "application/zip")
		w.Header().Set("Content-Disposition", "attachment; filename=model.zip")
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
