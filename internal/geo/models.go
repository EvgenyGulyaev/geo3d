package geo

import "math"

// Coord представляет географические координаты.
type Coord struct {
	Lat float64 `json:"lat"`
	Lon float64 `json:"lon"`
}

// Point3D представляет точку в 3D-пространстве (локальные метры).
type Point3D struct {
	X float64
	Y float64
	Z float64
}

// Point2D представляет точку в 2D-пространстве (локальные метры).
type Point2D struct {
	X float64
	Y float64
}

// Building представляет здание с контуром и высотой.
type Building struct {
	ID       int64     `json:"id"`
	Outline  []Coord   `json:"outline"`  // контур здания (lat/lon)
	Height   float64   `json:"height"`   // высота в метрах
	Levels   int       `json:"levels"`   // этажность
	Name     string    `json:"name"`
	Type     string    `json:"type"`     // residential, commercial, etc.
}

// Road представляет дорогу.
type Road struct {
	ID     int64   `json:"id"`
	Points []Coord `json:"points"` // точки линии дороги
	Width  float64 `json:"width"`  // ширина в метрах
	Lanes  int     `json:"lanes"`
	Type   string  `json:"type"` // primary, secondary, residential, etc.
	Name   string  `json:"name"`
}

// WaterArea представляет водоём.
type WaterArea struct {
	ID      int64   `json:"id"`
	Outline []Coord `json:"outline"`
	Name    string  `json:"name"`
	Type    string  `json:"type"` // river, lake, pond, etc.
}

// ElevationGrid представляет сетку высот рельефа.
type ElevationGrid struct {
	Width      int       // количество точек по X
	Height     int       // количество точек по Y
	CellSizeM  float64   // размер ячейки в метрах
	Points     []float64 // высоты (row-major, длина Width*Height)
	OriginLat  float64   // координата начала сетки
	OriginLon  float64
}

// BBox представляет ограничивающий прямоугольник.
type BBox struct {
	MinLat float64 `json:"min_lat"`
	MinLon float64 `json:"min_lon"`
	MaxLat float64 `json:"max_lat"`
	MaxLon float64 `json:"max_lon"`
}

// CityData содержит все геоданные для участка города.
type CityData struct {
	BBox      BBox          `json:"bbox"`
	Buildings []Building    `json:"buildings"`
	Roads     []Road        `json:"roads"`
	Water     []WaterArea   `json:"water"`
	Elevation ElevationGrid `json:"elevation"`
}

// GenerateRequest — параметры запроса на генерацию модели.
type GenerateRequest struct {
	City           string  `json:"city"`
	Lat            float64 `json:"lat"`
	Lon            float64 `json:"lon"`
	WidthM         float64 `json:"width"`          // размер области в метрах
	HeightM        float64 `json:"height"`         // размер области в метрах
	Format         string  `json:"format"`         // glb | obj | stl
	IncludeTerrain bool    `json:"include_terrain"`
	IncludeRoads   bool    `json:"include_roads"`
	// Параметры 3D-печати
	PrintReady     bool    `json:"print_ready"`      // подготовить для 3D-печати
	Scale          float64 `json:"scale"`            // масштаб, напр. 0.002 = 1:500
	BaseThickness  float64 `json:"base_thickness"`   // толщина основы в мм (по умолч. 3)
	MinWall        float64 `json:"min_wall"`         // мин. толщина стены мм (по умолч. 0.8)

	SplitBoard     bool    `json:"split_board"`      // разбить модель на тайлы
	BoardSizeMM    float64 `json:"board_size_mm"`    // размер 1 платы в мм (напр. 160)
	MergeTiles     bool    `json:"merge_tiles"`
	MergeGapMM     float64 `json:"merge_gap_mm"`
	Email          string  `json:"email"`            // email для отправки результата
}

const (
	// DefaultBuildingHeight — высота здания по умолчанию, если нет данных.
	DefaultBuildingHeight = 9.0
	// MetersPerLevel — средняя высота одного этажа.
	MetersPerLevel = 3.0
	// DefaultRoadWidth — ширина дороги по умолчанию.
	DefaultRoadWidth = 6.0
)

// BBoxFromCenter создаёт BBox из центра и размеров в метрах.
func BBoxFromCenter(lat, lon, widthM, heightM float64) BBox {
	// Приблизительный расчёт: 1 градус широты ≈ 111320 м
	// 1 градус долготы ≈ 111320 * cos(lat) м
	dLat := (heightM / 2.0) / 111320.0
	dLon := (widthM / 2.0) / (111320.0 * math.Cos(lat*math.Pi/180.0))

	return BBox{
		MinLat: lat - dLat,
		MinLon: lon - dLon,
		MaxLat: lat + dLat,
		MaxLon: lon + dLon,
	}
}

// LatLonToMeters преобразует lat/lon в локальные метры относительно центра.
func LatLonToMeters(lat, lon, centerLat, centerLon float64) Point2D {
	x := (lon - centerLon) * 111320.0 * math.Cos(centerLat*math.Pi/180.0)
	y := (lat - centerLat) * 111320.0
	return Point2D{X: x, Y: y}
}
