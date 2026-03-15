package generator

import (
	"encoding/binary"
	"fmt"
	"io"
	"math"
)

const (
	FormatSTL ExportFormat = "stl"
)

// ExportSTL экспортирует сцену в бинарный STL формат (стандарт для 3D-печати).
func ExportSTL(scene *Scene, w io.Writer) error {
	if len(scene.Meshes) == 0 {
		return fmt.Errorf("scene has no meshes")
	}

	// Подсчёт общего числа треугольников
	totalTriangles := uint32(0)
	for _, m := range scene.Meshes {
		totalTriangles += uint32(len(m.Indices) / 3)
	}

	// STL Header (80 bytes)
	header := make([]byte, 80)
	copy(header, []byte("3D Maps Generator - STL for 3D printing"))
	if _, err := w.Write(header); err != nil {
		return fmt.Errorf("write header: %w", err)
	}

	// Number of triangles (4 bytes, little-endian)
	if err := binary.Write(w, binary.LittleEndian, totalTriangles); err != nil {
		return fmt.Errorf("write triangle count: %w", err)
	}

	// Каждый треугольник: нормаль (3×f32) + 3 вершины (9×f32) + attribute (uint16) = 50 bytes
	for _, m := range scene.Meshes {
		numTris := len(m.Indices) / 3
		for t := 0; t < numTris; t++ {
			i0 := m.Indices[t*3]
			i1 := m.Indices[t*3+1]
			i2 := m.Indices[t*3+2]

			// Вершины: меняем Y и Z местами для Z-up (стандарт STL для принтеров)
			// Оригинальный порядок в Go: X, Y(высота), Z(глубина)
			// Целевой порядок STL: X, Y(глубина), Z(высота)
			v0x, v0z, v0y := m.Vertices[i0*3], m.Vertices[i0*3+1], m.Vertices[i0*3+2]
			v1x, v1z, v1y := m.Vertices[i1*3], m.Vertices[i1*3+1], m.Vertices[i1*3+2]
			v2x, v2z, v2y := m.Vertices[i2*3], m.Vertices[i2*3+1], m.Vertices[i2*3+2]

			// Вычисляем нормаль из вершин (с учетом нового порядка)
			nx, ny, nz := computeNormal(v0x, v0y, v0z, v1x, v1y, v1z, v2x, v2y, v2z)

			// Записываем треугольник
			tri := stlTriangle{
				Normal:    [3]float32{nx, ny, nz},
				Vertex1:   [3]float32{v0x, v0y, v0z},
				Vertex2:   [3]float32{v1x, v1y, v1z},
				Vertex3:   [3]float32{v2x, v2y, v2z},
				AttrCount: 0,
			}

			if err := binary.Write(w, binary.LittleEndian, &tri); err != nil {
				return fmt.Errorf("write triangle: %w", err)
			}
		}
	}

	return nil
}

type stlTriangle struct {
	Normal    [3]float32
	Vertex1   [3]float32
	Vertex2   [3]float32
	Vertex3   [3]float32
	AttrCount uint16
}

// computeNormal вычисляет нормаль треугольника из трёх вершин.
func computeNormal(v0x, v0y, v0z, v1x, v1y, v1z, v2x, v2y, v2z float32) (float32, float32, float32) {
	// Векторы рёбер
	ux, uy, uz := v1x-v0x, v1y-v0y, v1z-v0z
	vx, vy, vz := v2x-v0x, v2y-v0y, v2z-v0z

	// Векторное произведение
	nx := uy*vz - uz*vy
	ny := uz*vx - ux*vz
	nz := ux*vy - uy*vx

	// Нормализация
	length := float32(math.Sqrt(float64(nx*nx + ny*ny + nz*nz)))
	if length > 0 {
		nx /= length
		ny /= length
		nz /= length
	}

	return nx, ny, nz
}
