package generator

import (
	"math"
)

// PrintOptions — параметры для подготовки модели к 3D-печати.
type PrintOptions struct {
	Scale         float64 // масштаб, напр. 0.001 = 1:1000 (метры → мм / 1000)
	BaseThickness float64 // толщина основы в мм (после масштабирования)
	MinWallMM     float64 // минимальная толщина стен в мм
}

// DefaultPrintOptions — настройки по умолчанию для 3D-печати.
func DefaultPrintOptions() PrintOptions {
	return PrintOptions{
		Scale:         1.0, // 1:1 (1 метр в модели = 1 мм на выходе)
		BaseThickness: 3.0, // 3 мм основа
		MinWallMM:     0.8, // 0.8 мм минимальная стенка
	}
}

// PrepareForPrint подготавливает сцену к 3D-печати:
// 1. Объединяет все меши в один (watertight)
// 2. Добавляет solid base
// 3. Масштабирует
func PrepareForPrint(scene *Scene, widthM, heightM float64, opts PrintOptions) *Scene {
	printScene := NewScene()

	// 1. Объединяем все меши в один
	merged := MergeAllMeshes(scene)

	// 2. Находим bounds модели (до масштабирования)
	minX, minY, minZ, maxX, _, maxZ := meshBounds(merged)

	// 3. Сдвигаем модель чтобы минимум Y = baseThickness (в масштабированных единицах)
	yShift := float32(-minY) // сначала поднимаем на 0
	if opts.BaseThickness > 0 {
		yShift = float32(-minY) + float32(opts.BaseThickness/opts.Scale)
	}

	// Применяем сдвиг Y
	for i := 1; i < len(merged.Vertices); i += 3 {
		merged.Vertices[i] += yShift
	}

	// 4. Масштабируем
	if opts.Scale != 1.0 {
		scale := float32(opts.Scale)
		for i := range merged.Vertices {
			merged.Vertices[i] *= scale
		}
	}

	printScene.AddMesh(merged)

	// 5. Создаём solid base
	if opts.BaseThickness > 0 {
		base := generateSolidBase(
			(minX)*float32(opts.Scale), (minZ)*float32(opts.Scale),
			(maxX)*float32(opts.Scale), (maxZ)*float32(opts.Scale),
			float32(opts.BaseThickness),
		)
		printScene.AddMesh(base)
	}

	return printScene
}

// MergeAllMeshes объединяет все меши сцены в один.
func MergeAllMeshes(scene *Scene) *Mesh {
	merged := &Mesh{
		Name:  "merged_model",
		Color: [4]float32{0.8, 0.8, 0.8, 1.0},
	}

	vertexOffset := uint32(0)
	for _, m := range scene.Meshes {
		// Копируем вершины
		merged.Vertices = append(merged.Vertices, m.Vertices...)
		merged.Normals = append(merged.Normals, m.Normals...)

		// Копируем индексы со сдвигом
		for _, idx := range m.Indices {
			merged.Indices = append(merged.Indices, idx+vertexOffset)
		}

		vertexOffset += uint32(len(m.Vertices) / 3)
	}

	return merged
}

// generateSolidBase создаёт прямоугольный параллелепипед-основу.
// minX, minZ, maxX, maxZ — границы модели; thickness — толщина.
func generateSolidBase(minX, minZ, maxX, maxZ, thickness float32) *Mesh {
	// Небольшая выступающая кромка вокруг модели
	margin := thickness * 0.5
	x1 := minX - margin
	z1 := minZ - margin
	x2 := maxX + margin
	z2 := maxZ + margin

	y1 := float32(0)     // дно
	y2 := thickness       // верх основы

	mesh := &Mesh{
		Name:  "print_base",
		Color: [4]float32{0.6, 0.6, 0.6, 1.0},
	}

	// 6 граней × 4 вершины = 24 вершины
	// Верх (Y = y2)
	addQuad(mesh, 
		x1, y2, z1,  x2, y2, z1,  x2, y2, z2,  x1, y2, z2,
		0, 1, 0)
	// Дно (Y = y1)
	addQuad(mesh,
		x1, y1, z2,  x2, y1, z2,  x2, y1, z1,  x1, y1, z1,
		0, -1, 0)
	// Передняя стенка (Z = z2)
	addQuad(mesh,
		x1, y1, z2,  x1, y2, z2,  x2, y2, z2,  x2, y1, z2,
		0, 0, 1)
	// Задняя стенка (Z = z1)
	addQuad(mesh,
		x2, y1, z1,  x2, y2, z1,  x1, y2, z1,  x1, y1, z1,
		0, 0, -1)
	// Левая стенка (X = x1)
	addQuad(mesh,
		x1, y1, z1,  x1, y2, z1,  x1, y2, z2,  x1, y1, z2,
		-1, 0, 0)
	// Правая стенка (X = x2)
	addQuad(mesh,
		x2, y1, z2,  x2, y2, z2,  x2, y2, z1,  x2, y1, z1,
		1, 0, 0)

	return mesh
}

// addQuad добавляет четырёхугольник (2 треугольника) в меш.
func addQuad(mesh *Mesh, 
	x1, y1, z1, x2, y2, z2, x3, y3, z3, x4, y4, z4 float32,
	nx, ny, nz float32) {

	idx := uint32(len(mesh.Vertices) / 3)

	mesh.Vertices = append(mesh.Vertices,
		x1, y1, z1,
		x2, y2, z2,
		x3, y3, z3,
		x4, y4, z4,
	)

	for i := 0; i < 4; i++ {
		mesh.Normals = append(mesh.Normals, nx, ny, nz)
	}

	mesh.Indices = append(mesh.Indices,
		idx, idx+1, idx+2,
		idx, idx+2, idx+3,
	)
}

// meshBounds находит границы меша.
func meshBounds(m *Mesh) (minX, minY, minZ, maxX, maxY, maxZ float32) {
	minX = float32(math.MaxFloat32)
	minY = float32(math.MaxFloat32)
	minZ = float32(math.MaxFloat32)
	maxX = float32(-math.MaxFloat32)
	maxY = float32(-math.MaxFloat32)
	maxZ = float32(-math.MaxFloat32)

	numVerts := len(m.Vertices) / 3
	for i := 0; i < numVerts; i++ {
		x, y, z := m.Vertices[i*3], m.Vertices[i*3+1], m.Vertices[i*3+2]
		if x < minX {
			minX = x
		}
		if y < minY {
			minY = y
		}
		if z < minZ {
			minZ = z
		}
		if x > maxX {
			maxX = x
		}
		if y > maxY {
			maxY = y
		}
		if z > maxZ {
			maxZ = z
		}
	}

	return
}

// ScaleScene масштабирует все вершины сцены на заданный коэффициент.
func ScaleScene(scene *Scene, scale float64) {
	s := float32(scale)
	for _, m := range scene.Meshes {
		for i := range m.Vertices {
			m.Vertices[i] *= s
		}
	}
}
