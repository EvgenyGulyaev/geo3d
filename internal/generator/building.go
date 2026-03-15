package generator

import (
	"fmt"

	"github.com/evgeny/3d-maps/internal/geo"
	"github.com/evgeny/3d-maps/internal/math2d"
	"github.com/evgeny/3d-maps/internal/triangulate"
)

// GenerateBuilding создаёт 3D-меш здания из его контура и высоты.
// centerLat, centerLon — центр карты для преобразования координат в метры.
// clipRect — (опционально) прямоугольник для отсечения контура (или nil).
func GenerateBuilding(b geo.Building, centerLat, centerLon float64, clipRect *math2d.Rect, heightMultiplier float64) *Mesh {
	if len(b.Outline) < 3 {
		return nil
	}

	// Преобразуем контур в локальные метры
	points2D := make([]triangulate.Point, 0, len(b.Outline))
	for _, c := range b.Outline {
		p := geo.LatLonToMeters(c.Lat, c.Lon, centerLat, centerLon)
		points2D = append(points2D, triangulate.Point{X: p.X, Y: p.Y})
	}

	// Отсечение полигона по BBox тайла, если задан
	if clipRect != nil {
		points2D = math2d.ClipPolygon(points2D, *clipRect)
	}

	// Убираем замыкающую точку, если она совпадает с первой
	if len(points2D) > 1 {
		first := points2D[0]
		last := points2D[len(points2D)-1]
		if abs(first.X-last.X) < 1e-8 && abs(first.Y-last.Y) < 1e-8 {
			points2D = points2D[:len(points2D)-1]
		}
	}

	if len(points2D) < 3 {
		return nil
	}

	// Триангулируем крышу
	triIndices := triangulate.Triangulate(points2D)
	if len(triIndices) == 0 {
		return nil
	}

	n := len(points2D)
	if heightMultiplier <= 0 {
		heightMultiplier = 1.0
	}
	height := float32(b.Height * heightMultiplier)

	mesh := &Mesh{
		Name:  fmt.Sprintf("building_%d", b.ID),
		Color: buildingColor(b.Type),
	}

	// === Крыша (верх) ===
	// Вершины крыши
	for _, p := range points2D {
		mesh.Vertices = append(mesh.Vertices, float32(p.X), height, float32(p.Y))
		mesh.Normals = append(mesh.Normals, 0, 1, 0) // нормаль вверх
	}
	for _, idx := range triIndices {
		mesh.Indices = append(mesh.Indices, uint32(idx))
	}

	// === Нижняя грань (пол) ===
	baseOffset := uint32(n)
	for _, p := range points2D {
		mesh.Vertices = append(mesh.Vertices, float32(p.X), 0, float32(p.Y))
		mesh.Normals = append(mesh.Normals, 0, -1, 0) // нормаль вниз
	}
	// Индексы пола (обратный порядок для правильной ориентации)
	for i := len(triIndices) - 1; i >= 0; i -= 3 {
		if i-2 >= 0 {
			mesh.Indices = append(mesh.Indices,
				baseOffset+uint32(triIndices[i-2]),
				baseOffset+uint32(triIndices[i]),
				baseOffset+uint32(triIndices[i-1]),
			)
		}
	}

	// === Стены ===
	wallOffset := uint32(2 * n)
	for i := 0; i < n; i++ {
		next := (i + 1) % n
		p1 := points2D[i]
		p2 := points2D[next]

		// Нормаль стены (направлена наружу)
		dx := float32(p2.X - p1.X)
		dy := float32(p2.Y - p1.Y)
		nx := dy
		ny := -dx
		length := float32(0)
		length = float32(sqrt64(float64(nx*nx + ny*ny)))
		if length > 0 {
			nx /= length
			ny /= length
		}

		// Четыре вершины стены
		vIdx := wallOffset + uint32(i*4)

		// Нижний левый, нижний правый, верхний правый, верхний левый
		mesh.Vertices = append(mesh.Vertices,
			float32(p1.X), 0, float32(p1.Y),
			float32(p2.X), 0, float32(p2.Y),
			float32(p2.X), height, float32(p2.Y),
			float32(p1.X), height, float32(p1.Y),
		)
		mesh.Normals = append(mesh.Normals,
			nx, 0, ny,
			nx, 0, ny,
			nx, 0, ny,
			nx, 0, ny,
		)

		// Два треугольника стены
		mesh.Indices = append(mesh.Indices,
			vIdx, vIdx+1, vIdx+2,
			vIdx, vIdx+2, vIdx+3,
		)
	}

	return mesh
}

// GenerateBuildings генерирует меши для списка зданий.
func GenerateBuildings(buildings []geo.Building, centerLat, centerLon float64, clipRect *math2d.Rect, heightMultiplier float64) []*Mesh {
	var meshes []*Mesh
	for _, b := range buildings {
		if m := GenerateBuilding(b, centerLat, centerLon, clipRect, heightMultiplier); m != nil {
			meshes = append(meshes, m)
		}
	}
	return meshes
}

// buildingColor возвращает цвет по типу здания.
func buildingColor(buildingType string) [4]float32 {
	switch buildingType {
	case "residential", "apartments":
		return [4]float32{0.85, 0.82, 0.75, 1.0} // тёплый бежевый
	case "commercial", "retail":
		return [4]float32{0.70, 0.75, 0.82, 1.0} // голубовато-серый
	case "industrial":
		return [4]float32{0.65, 0.65, 0.65, 1.0} // серый
	case "church", "cathedral", "chapel":
		return [4]float32{0.90, 0.85, 0.70, 1.0} // песочный
	default:
		return [4]float32{0.80, 0.78, 0.75, 1.0} // светло-серый
	}
}

func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}

func sqrt64(x float64) float64 {
	if x <= 0 {
		return 0
	}
	// Newton's method
	z := x / 2
	for i := 0; i < 20; i++ {
		z = (z + x/z) / 2
	}
	return z
}
