package generator

import (
	"bytes"
	"testing"
)

func TestExportSTL(t *testing.T) {
	scene := NewScene()
	scene.AddMesh(GenerateFlatGround(100, 100))

	var buf bytes.Buffer
	err := ExportSTL(scene, &buf)
	if err != nil {
		t.Fatalf("export stl error: %v", err)
	}

	data := buf.Bytes()
	// STL header = 80 bytes + 4 bytes triangle count + 50 bytes per triangle
	// Ground = 2 triangles → 80 + 4 + 100 = 184 bytes
	expectedSize := 80 + 4 + 2*50
	if len(data) != expectedSize {
		t.Fatalf("expected %d bytes, got %d", expectedSize, len(data))
	}
}

func TestPrepareForPrint(t *testing.T) {
	scene := NewScene()
	scene.AddMesh(GenerateFlatGround(100, 100))

	opts := PrintOptions{
		Scale:         1.0,
		BaseThickness: 3.0,
		MinWallMM:     0.8,
	}

	printScene := PrepareForPrint(scene, 100, 100, opts)
	if printScene == nil {
		t.Fatal("expected print scene")
	}
	if len(printScene.Meshes) == 0 {
		t.Fatal("expected meshes in print scene")
	}

	// Должен быть merged mesh + base mesh
	if len(printScene.Meshes) < 2 {
		t.Fatalf("expected at least 2 meshes (merged + base), got %d", len(printScene.Meshes))
	}

	// Проверяем, что base имеет 6 граней × 2 треугольника = 12 треугольников
	baseMesh := printScene.Meshes[1]
	if baseMesh.Name != "print_base" {
		t.Fatalf("expected 'print_base', got '%s'", baseMesh.Name)
	}
	baseTris := len(baseMesh.Indices) / 3
	if baseTris != 12 {
		t.Fatalf("expected 12 triangles in base, got %d", baseTris)
	}
}

func TestMergeAllMeshes(t *testing.T) {
	scene := NewScene()
	scene.AddMesh(GenerateFlatGround(50, 50))
	scene.AddMesh(GenerateFlatGround(50, 50))

	merged := MergeAllMeshes(scene)
	if merged == nil {
		t.Fatal("expected merged mesh")
	}

	// Два квадрата = 8 вершин × 3 компоненты
	expectedVerts := 24
	if len(merged.Vertices) != expectedVerts {
		t.Fatalf("expected %d vertex components, got %d", expectedVerts, len(merged.Vertices))
	}

	// 4 треугольника
	expectedIndices := 12
	if len(merged.Indices) != expectedIndices {
		t.Fatalf("expected %d indices, got %d", expectedIndices, len(merged.Indices))
	}
}

func TestPrintSTLWithBuilding(t *testing.T) {
	scene := NewScene()
	scene.AddMesh(GenerateFlatGround(200, 200))

	// Добавляем простое здание
	building := &Mesh{
		Name:  "test_building",
		Color: [4]float32{0.8, 0.8, 0.8, 1.0},
		Vertices: []float32{
			-5, 0, -5, 5, 0, -5, 5, 0, 5, -5, 0, 5, // пол
			-5, 10, -5, 5, 10, -5, 5, 10, 5, -5, 10, 5, // крыша
		},
		Normals: []float32{
			0, -1, 0, 0, -1, 0, 0, -1, 0, 0, -1, 0,
			0, 1, 0, 0, 1, 0, 0, 1, 0, 0, 1, 0,
		},
		Indices: []uint32{
			0, 1, 2, 0, 2, 3, // пол
			4, 5, 6, 4, 6, 7, // крыша
		},
	}
	scene.AddMesh(building)

	opts := PrintOptions{Scale: 0.5, BaseThickness: 2.0, MinWallMM: 0.8}
	printScene := PrepareForPrint(scene, 200, 200, opts)

	var buf bytes.Buffer
	err := ExportSTL(printScene, &buf)
	if err != nil {
		t.Fatalf("export print stl error: %v", err)
	}

	if buf.Len() < 200 {
		t.Fatalf("STL too small: %d bytes", buf.Len())
	}
}
