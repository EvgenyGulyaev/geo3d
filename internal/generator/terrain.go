package generator

import (
	"fmt"
	"math"

	"github.com/evgeny/3d-maps/internal/geo"
)

// GenerateTerrain создаёт 3D-меш рельефа из сетки высот.
func GenerateTerrain(grid *geo.ElevationGrid, centerLat, centerLon float64) *Mesh {
	if grid == nil || len(grid.Points) < 4 {
		return nil
	}

	w := grid.Width
	h := grid.Height

	if len(grid.Points) < w*h {
		return nil
	}

	// Находим минимальную высоту для нормализации
	minElev := math.MaxFloat64
	for _, e := range grid.Points {
		if e < minElev {
			minElev = e
		}
	}

	mesh := &Mesh{
		Name:  "terrain",
		Color: [4]float32{0.45, 0.55, 0.35, 1.0}, // зелёный
	}

	// Вычисляем шаг сетки
	latStep := (grid.Points[0] - grid.OriginLat) // не нужен, используем CellSizeM
	_ = latStep

	// Создаём вершины
	for row := 0; row < h; row++ {
		for col := 0; col < w; col++ {
			// Позиция в мировых координатах
			lat := grid.OriginLat + float64(row)*(grid.CellSizeM/111320.0)
			lonScale := 111320.0 * math.Cos(centerLat*math.Pi/180.0)
			lon := grid.OriginLon + float64(col)*(grid.CellSizeM/lonScale)

			p := geo.LatLonToMeters(lat, lon, centerLat, centerLon)
			elev := grid.Points[row*w+col] - minElev // нормализуем

			mesh.Vertices = append(mesh.Vertices,
				float32(p.X), float32(elev), float32(p.Y),
			)
		}
	}

	// Вычисляем нормали
	for row := 0; row < h; row++ {
		for col := 0; col < w; col++ {
			nx, ny, nz := terrainNormal(grid, row, col, w, h)
			mesh.Normals = append(mesh.Normals, float32(nx), float32(ny), float32(nz))
		}
	}

	// Создаём индексы (два треугольника на ячейку)
	for row := 0; row < h-1; row++ {
		for col := 0; col < w-1; col++ {
			topLeft := uint32(row*w + col)
			topRight := uint32(row*w + col + 1)
			bottomLeft := uint32((row+1)*w + col)
			bottomRight := uint32((row+1)*w + col + 1)

			mesh.Indices = append(mesh.Indices,
				topLeft, bottomLeft, topRight,
				topRight, bottomLeft, bottomRight,
			)
		}
	}

	return mesh
}

// terrainNormal вычисляет нормаль для точки рельефа.
func terrainNormal(grid *geo.ElevationGrid, row, col, w, h int) (float64, float64, float64) {
	getElev := func(r, c int) float64 {
		if r < 0 {
			r = 0
		}
		if r >= h {
			r = h - 1
		}
		if c < 0 {
			c = 0
		}
		if c >= w {
			c = w - 1
		}
		return grid.Points[r*w+c]
	}

	// Центральные разности
	dzdx := (getElev(row, col+1) - getElev(row, col-1)) / (2.0 * grid.CellSizeM)
	dzdy := (getElev(row+1, col) - getElev(row-1, col)) / (2.0 * grid.CellSizeM)

	nx := -dzdx
	ny := 1.0
	nz := -dzdy

	length := math.Sqrt(nx*nx + ny*ny + nz*nz)
	if length > 0 {
		nx /= length
		ny /= length
		nz /= length
	}

	return nx, ny, nz
}

// GenerateFlatGround создаёт плоскую поверхность земли (фоллбэк без рельефа).
func GenerateFlatGround(widthM, heightM float64) *Mesh {
	halfW := float32(widthM / 2)
	halfH := float32(heightM / 2)

	return &Mesh{
		Name:  "ground",
		Color: [4]float32{0.45, 0.55, 0.35, 1.0},
		Vertices: []float32{
			-halfW, 0, -halfH,
			halfW, 0, -halfH,
			halfW, 0, halfH,
			-halfW, 0, halfH,
		},
		Normals: []float32{
			0, 1, 0,
			0, 1, 0,
			0, 1, 0,
			0, 1, 0,
		},
		Indices: []uint32{
			0, 1, 2,
			0, 2, 3,
		},
	}
}

// GenerateFlatGroundFromRect создаёт плоскую поверхность земли по координатам.
func GenerateFlatGroundFromRect(minX, minY, maxX, maxY float64) *Mesh {
	return &Mesh{
		Name:  "ground",
		Color: [4]float32{0.45, 0.55, 0.35, 1.0},
		Vertices: []float32{
			float32(minX), 0, float32(minY),
			float32(maxX), 0, float32(minY),
			float32(maxX), 0, float32(maxY),
			float32(minX), 0, float32(maxY),
		},
		Normals: []float32{
			0, 1, 0,
			0, 1, 0,
			0, 1, 0,
			0, 1, 0,
		},
		Indices: []uint32{
			0, 1, 2,
			0, 2, 3,
		},
	}
}

// terrainColorByElevation возвращает цвет по высоте рельефа.
func terrainColorByElevation(elevation float64) [4]float32 {
	_ = elevation
	return [4]float32{0.45, 0.55, 0.35, 1.0}
}

func init() {
	_ = fmt.Sprintf // suppress unused import
}
