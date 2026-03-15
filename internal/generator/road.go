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
	roadHeight := float32(0.05) // чуть выше земли

	mesh := &Mesh{
		Name:  fmt.Sprintf("road_%d", r.ID),
		Color: roadColor(r.Type),
	}

	// Для каждого сегмента дороги создаём прямоугольник
	for i := 0; i < len(r.Points)-1; i++ {
		p1geo := geo.LatLonToMeters(r.Points[i].Lat, r.Points[i].Lon, centerLat, centerLon)
		p2geo := geo.LatLonToMeters(r.Points[i+1].Lat, r.Points[i+1].Lon, centerLat, centerLon)

		p1 := triangulate.Point{X: p1geo.X, Y: p1geo.Y}
		p2 := triangulate.Point{X: p2geo.X, Y: p2geo.Y}

		// Направление сегмента
		dx := p2.X - p1.X
		dy := p2.Y - p1.Y
		length := math.Sqrt(dx*dx + dy*dy)
		if length < 0.001 {
			continue
		}

		// Перпендикуляр (нормаль)
		nx := -dy / length * halfWidth
		ny := dx / length * halfWidth

		// Контур сегмента дороги
		roadPoly := []triangulate.Point{
			{X: p1.X + nx, Y: p1.Y + ny},
			{X: p1.X - nx, Y: p1.Y - ny},
			{X: p2.X - nx, Y: p2.Y - ny},
			{X: p2.X + nx, Y: p2.Y + ny},
		}

		if clipRect != nil {
			roadPoly = math2d.ClipPolygon(roadPoly, *clipRect)
			if len(roadPoly) < 3 {
				continue
			}
			
			// Триангуляция обрезанного полигона (так как он может стать не 4-угольником)
			triIndices := triangulate.Triangulate(roadPoly)
			if len(triIndices) == 0 {
				continue
			}

			vOffset := uint32(len(mesh.Vertices) / 3)
			for _, pt := range roadPoly {
				mesh.Vertices = append(mesh.Vertices, float32(pt.X), roadHeight, float32(pt.Y))
				mesh.Normals = append(mesh.Normals, 0, 1, 0)
			}
			for _, idx := range triIndices {
				mesh.Indices = append(mesh.Indices, vOffset+uint32(idx))
			}
			
		} else {
			// Оригинальное добавление без обрезки (квадратный сегмент)
			vIdx := uint32(len(mesh.Vertices) / 3)

			mesh.Vertices = append(mesh.Vertices,
				float32(p1.X+nx), roadHeight, float32(p1.Y+ny),
				float32(p1.X-nx), roadHeight, float32(p1.Y-ny),
				float32(p2.X-nx), roadHeight, float32(p2.Y-ny),
				float32(p2.X+nx), roadHeight, float32(p2.Y+ny),
			)

			// Нормали вверх
			for j := 0; j < 4; j++ {
				mesh.Normals = append(mesh.Normals, 0, 1, 0)
			}

			// Два треугольника
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
