package generator

import (
	"fmt"
	"math"

	"github.com/evgeny/3d-maps/internal/geo"
)

// GenerateRoads создаёт 3D-меши дорог из линий.
func GenerateRoads(roads []geo.Road, centerLat, centerLon float64) []*Mesh {
	var meshes []*Mesh
	for _, r := range roads {
		if m := GenerateRoad(r, centerLat, centerLon); m != nil {
			meshes = append(meshes, m)
		}
	}
	return meshes
}

// GenerateRoad создаёт плоский полигон дороги из линии.
func GenerateRoad(r geo.Road, centerLat, centerLon float64) *Mesh {
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
		p1 := geo.LatLonToMeters(r.Points[i].Lat, r.Points[i].Lon, centerLat, centerLon)
		p2 := geo.LatLonToMeters(r.Points[i+1].Lat, r.Points[i+1].Lon, centerLat, centerLon)

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

		vIdx := uint32(len(mesh.Vertices) / 3)

		// Четыре угла сегмента дороги
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
