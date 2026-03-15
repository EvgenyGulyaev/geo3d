package generator

import (
	"fmt"
	"math"

	"github.com/evgeny/3d-maps/internal/geo"
	"github.com/evgeny/3d-maps/internal/math2d"
	"github.com/evgeny/3d-maps/internal/triangulate"
)

// GenerateRoads создаёт 3D-меши дорог из линий.
func GenerateRoads(roads []geo.Road, centerLat, centerLon float64, clipRect *math2d.Rect) []*Mesh {
	var meshes []*Mesh
	for _, r := range roads {
		if m := GenerateRoad(r, centerLat, centerLon, clipRect); m != nil {
			meshes = append(meshes, m)
		}
	}
	return meshes
}

// GenerateRoad создаёт плоский полигон дороги из линии.
func GenerateRoad(r geo.Road, centerLat, centerLon float64, clipRect *math2d.Rect) *Mesh {
	if len(r.Points) < 2 {
		return nil
	}

	halfWidth := r.Width / 2.0
	groundHeight := float32(0.0) // земля
	roadThickness := float32(0.3) // толщина дороги в метрах
	roadTopY := groundHeight + roadThickness

	mesh := &Mesh{
		Name:  fmt.Sprintf("road_%d", r.ID),
		Color: roadColor(r.Type),
	}

	// Для каждого сегмента дороги создаём 3D параллелепипед
	for i := 0; i < len(r.Points)-1; i++ {
		p1geo := geo.LatLonToMeters(r.Points[i].Lat, r.Points[i].Lon, centerLat, centerLon)
		p2geo := geo.LatLonToMeters(r.Points[i+1].Lat, r.Points[i+1].Lon, centerLat, centerLon)

		p1 := triangulate.Point{X: p1geo.X, Y: p1geo.Y}
		p2 := triangulate.Point{X: p2geo.X, Y: p2geo.Y}

		dx := p2.X - p1.X
		dy := p2.Y - p1.Y
		length := math.Sqrt(dx*dx + dy*dy)
		if length < 0.001 {
			continue
		}

		nx := -dy / length * halfWidth
		ny := dx / length * halfWidth

		// Контур сегмента дороги (4 точки)
		poly := []triangulate.Point{
			{X: p1.X + nx, Y: p1.Y + ny},
			{X: p2.X + nx, Y: p2.Y + ny},
			{X: p2.X - nx, Y: p2.Y - ny},
			{X: p1.X - nx, Y: p1.Y - ny},
		}

		if clipRect != nil {
			poly = math2d.ClipPolygon(poly, *clipRect)
			if len(poly) < 3 {
				continue
			}
		}

		// --- Крыша (верх дороги) ---
		vOffset := uint32(len(mesh.Vertices) / 3)
		indices := triangulate.Triangulate(poly)
		if len(indices) == 0 {
			continue
		}
		for _, pt := range poly {
			mesh.Vertices = append(mesh.Vertices, float32(pt.X), roadTopY, float32(pt.Y))
			mesh.Normals = append(mesh.Normals, 0, 1, 0)
		}
		for _, idx := range indices {
			mesh.Indices = append(mesh.Indices, vOffset+uint32(idx))
		}

		// --- Стенки (бока дороги) ---
		n := len(poly)
		for j := 0; j < n; j++ {
			next := (j + 1) % n
			pp1 := poly[j]
			pp2 := poly[next]

			vIdx := uint32(len(mesh.Vertices) / 3)
			// Четыре вершины стенки (нижний левый, нижний правый, верхний правый, верхний левый)
			mesh.Vertices = append(mesh.Vertices,
				float32(pp1.X), groundHeight, float32(pp1.Y),
				float32(pp2.X), groundHeight, float32(pp2.Y),
				float32(pp2.X), roadTopY, float32(pp2.Y),
				float32(pp1.X), roadTopY, float32(pp1.Y),
			)
			// Упрощенная нормаль (вбок)
			mesh.Normals = append(mesh.Normals, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0)
			
			mesh.Indices = append(mesh.Indices,
				vIdx, vIdx+1, vIdx+2,
				vIdx, vIdx+2, vIdx+3,
			)
		}
	}

	if len(mesh.Vertices) == 0 {
		return nil
	}

	return mesh
}

// roadColor возвращает цвет по типу дороги.
func roadColor(roadType string) [4]float32 {
	switch roadType {
	case "motorway", "trunk":
		return [4]float32{0.35, 0.35, 0.40, 1.0} // тёмно-серый
	case "primary":
		return [4]float32{0.40, 0.40, 0.45, 1.0}
	case "secondary", "tertiary":
		return [4]float32{0.50, 0.50, 0.52, 1.0}
	default:
		return [4]float32{0.55, 0.55, 0.55, 1.0} // светло-серый
	}
}
