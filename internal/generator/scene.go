package generator

// Mesh представляет 3D-модель: вершины, нормали, индексы, цвет.
type Mesh struct {
	Vertices []float32 // x, y, z (по три компонента на вершину)
	Normals  []float32 // nx, ny, nz
	Indices  []uint32
	Color    [4]float32 // RGBA
	Name     string
}

// Scene — полная 3D-сцена из нескольких мешей.
type Scene struct {
	Meshes []*Mesh
}

// NewScene создаёт пустую сцену.
func NewScene() *Scene {
	return &Scene{}
}

// AddMesh добавляет меш в сцену.
func (s *Scene) AddMesh(m *Mesh) {
	if m != nil && len(m.Vertices) > 0 {
		s.Meshes = append(s.Meshes, m)
	}
}

// TotalVertices возвращает общее количество вершин.
func (s *Scene) TotalVertices() int {
	total := 0
	for _, m := range s.Meshes {
		total += len(m.Vertices) / 3
	}
	return total
}

// TotalTriangles возвращает общее количество треугольников.
func (s *Scene) TotalTriangles() int {
	total := 0
	for _, m := range s.Meshes {
		total += len(m.Indices) / 3
	}
	return total
}
