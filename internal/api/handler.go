package api

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"net/http"
	"time"

	"github.com/evgeny/3d-maps/internal/cache"
	"github.com/evgeny/3d-maps/internal/config"
	"github.com/evgeny/3d-maps/internal/generator"
	"github.com/evgeny/3d-maps/internal/geo"
	"github.com/evgeny/3d-maps/internal/mail"
	"github.com/evgeny/3d-maps/internal/math2d"
)

// Handler содержит зависимости для HTTP-обработчиков.
type Handler struct {
	overpass  *geo.OverpassClient
	elevation *geo.ElevationClient
	nominatim *geo.NominatimClient
	cache     *cache.LRU
	mail      *mail.Mailer
	cfg       *config.Config
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
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var req geo.GenerateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json: "+err.Error())
		return
	}

	log.Printf("Incoming request: %+v", req)

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
	cacheKey := fmt.Sprintf("%.5f_%.5f_%.0f_%.0f_%s_%v_%v_%v_%.6f_%.1f_%v_%.1f",
		req.Lat, req.Lon, req.WidthM, req.HeightM, req.Format,
		req.IncludeTerrain, req.IncludeRoads, req.PrintReady,
		req.Scale, req.BaseThickness, req.SplitBoard, req.BoardSizeMM)

	// Проверяем кэш
	if data, ok := h.cache.Get(cacheKey); ok {
		log.Printf("Cache hit for %s", cacheKey)
		effectiveFormat := req.Format
		if req.SplitBoard {
			effectiveFormat = "zip"
		}
		writeModelResponse(w, data, effectiveFormat)
		return
	}

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
		log.Printf("Async generation error: %v", err)
		return
	}

	filename := "model." + format
	if format == "zip" {
		filename = "3d_model_tiles.zip"
	}

	err = h.mail.SendModelEmail(req.Email, filename, data)
	if err != nil {
		log.Printf("Failed to send email to %s: %v", req.Email, err)
	} else {
		log.Printf("Email successfully sent to %s", req.Email)
	}
}

func (h *Handler) generateModelSync(req geo.GenerateRequest, cacheKey string) (resultData []byte, resultFormat string, err error) {
	start := time.Now()
	// Проверяем кэш еще раз (на случай если за время пока мы думали он там появился)
	if data, ok := h.cache.Get(cacheKey); ok {
		log.Printf("Cache hit in sync worker for %s", cacheKey)
		effectiveFormat := req.Format
		if req.SplitBoard {
			effectiveFormat = "zip"
		}
		return data, effectiveFormat, nil
	}

	log.Printf("Generating model: lat=%.5f lon=%.5f size=%.0fx%.0f format=%s",
		req.Lat, req.Lon, req.WidthM, req.HeightM, req.Format)

	// Создаём BBox
	bbox := geo.BBoxFromCenter(req.Lat, req.Lon, req.WidthM, req.HeightM)

	// === Загрузка данных ===

	// Здания
	buildings, err := h.overpass.FetchBuildings(bbox)
	if err != nil {
		return nil, "", fmt.Errorf("fetch buildings: %w", err)
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

	// === Генерация ===
	resultFormat = req.Format

	// Если запрошено разделение на платы
	if req.SplitBoard && req.BoardSizeMM > 0 {
		log.Printf(">>> Branch: SPLIT BOARD (BoardSize=%.1f)", req.BoardSizeMM)
		// BoardSizeMM - размер платы в мм. Scale: например, 0.002 = 2мм на 1метр.
		// Значит, 1 плата в физическом мире покроет: BoardSizeMM / Scale (метров из геометрии)
		baseScale := req.Scale
		if baseScale <= 0 {
			baseScale = 1.0 // safeguard
		}
		
		tileSizeMeters := req.BoardSizeMM / baseScale
		
		numX := int(math.Ceil(req.WidthM / tileSizeMeters))
		numY := int(math.Ceil(req.HeightM / tileSizeMeters))

		log.Printf("Splitting into %dx%d tiles, tile size %.2fm (board %vmm, scale %.6f)", numX, numY, tileSizeMeters, req.BoardSizeMM, baseScale)

		var zipBuf bytes.Buffer
		zipWriter := zip.NewWriter(&zipBuf)
		
		validTilesCount := 0
		
		// Рельеф (скачиваем один раз на всю область, если нужно)
		var grid *geo.ElevationGrid
		if req.IncludeTerrain {
			var err error
			grid, err = h.elevation.FetchElevationGrid(bbox, 20)
			if err != nil {
				log.Printf("Warning: fetch elevation failed for split: %v", err)
				grid = nil // fallback flat
			}
		}

		// Генерация каждого тайла
		for y := 0; y < numY; y++ {
			for x := 0; x < numX; x++ {
				// Локальные метры для тайла (относительно центра всей карты)
				// Центр всей карты - (0,0).
				// Левый нижний угол всей карты: -Width/2, -Height/2
				startX := -req.WidthM/2.0 + float64(x)*tileSizeMeters
				startY := -req.HeightM/2.0 + float64(y)*tileSizeMeters
				endX := startX + tileSizeMeters
				endY := startY + tileSizeMeters

				clipRect := math2d.Rect{MinX: startX, MinY: startY, MaxX: endX, MaxY: endY}

				// Строим сцену для тайла
				scene := generator.NewScene()
				
				// Дороги
				if req.IncludeRoads {
					for _, m := range generator.GenerateRoads(roads, req.Lat, req.Lon, &clipRect) {
						if m != nil {
							scene.AddMesh(m)
						}
					}
				}

				// Здания
				buildingsAdded := 0
				for _, m := range generator.GenerateBuildings(buildings, req.Lat, req.Lon, &clipRect) {
					if m != nil {
						scene.AddMesh(m)
						buildingsAdded++
					}
				}

				// Рельеф (упрощенно: плоская земля ровно под размер тайла, если не включен terrain)
				if req.IncludeTerrain && grid != nil && grid.Width > 0 {
					// TODO: полноценная обрезка рельефа (пока fallback на кусок плоскости для демо)
					scene.AddMesh(generator.GenerateFlatGroundFromRect(clipRect.MinX, clipRect.MinY, clipRect.MaxX, clipRect.MaxY))
				} else {
					scene.AddMesh(generator.GenerateFlatGroundFromRect(clipRect.MinX, clipRect.MinY, clipRect.MaxX, clipRect.MaxY))
				}

				// Если пусто (нет зданий и дорог) - пропускаем
				if buildingsAdded == 0 && !req.IncludeRoads {
					continue
				}

				// Подготовка к печати тайла
				if req.PrintReady {
					opts := generator.PrintOptions{
						Scale:         req.Scale,
						BaseThickness: req.BaseThickness,
						MinWallMM:     req.MinWall,
					}
					// Сдвигаем все вершины тайла к (0,0) чтобы он печатался по центру стола
					offsetX := -(startX + tileSizeMeters/2.0)
					offsetY := -(startY + tileSizeMeters/2.0)
					
					shiftScene(scene, float32(offsetX), float32(offsetY))
					scene = generator.PrepareForPrint(scene, tileSizeMeters, tileSizeMeters, opts)
				}
				
				// Запись тайла в архив
				filename := fmt.Sprintf("tile_%d_%d.%s", x, y, req.Format)
				fWriter, err := zipWriter.Create(filename)
				if err != nil {
					log.Printf("failed to create zip file %s: %v", filename, err)
					continue
				}

				var buf bytes.Buffer
				if req.Format == "stl" {
					generator.ExportSTL(scene, &buf)
				} else if req.Format == "obj" {
					generator.ExportOBJ(scene, &buf)
				} else {
					generator.ExportGLB(scene, &buf)
				}
				
				fWriter.Write(buf.Bytes())
				validTilesCount++
			}
		}
		
		// Генерация SVG карты сборки
		if validTilesCount > 0 {
			svgFilename := "layout_map.svg"
			svgWriter, _ := zipWriter.Create(svgFilename)
			svgContent := generateLayoutSVG(numX, numY, tileSizeMeters, req.BoardSizeMM)
			svgWriter.Write([]byte(svgContent))
		}

		zipWriter.Close()
		resultData = zipBuf.Bytes()
		resultFormat = "zip"
		
		log.Printf("ZIP created with %d tiles in %v", validTilesCount, time.Since(start))

	} else {
		log.Printf(">>> Branch: SINGLE MODEL (Split=%v Size=%v)", req.SplitBoard, req.BoardSizeMM)
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

		for _, m := range generator.GenerateBuildings(buildings, req.Lat, req.Lon, nil) {
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
