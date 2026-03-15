package generator

import (
	"bytes"
	"testing"

	"github.com/evgeny/3d-maps/internal/geo"
)

func TestGenerateBuilding(t *testing.T) {
	building := geo.Building{
		ID:     1,
		Height: 12.0,
		Type:   "residential",
		Outline: []geo.Coord{
			{Lat: 55.7550, Lon: 37.6170},
			{Lat: 55.7550, Lon: 37.6175},
			{Lat: 55.7555, Lon: 37.6175},
			{Lat: 55.7555, Lon: 37.6170},
			{Lat: 55.7550, Lon: 37.6170}, // замыкающая точка
		},
	}

	mesh := GenerateBuilding(building, 55.7553, 37.6172)
	if mesh == nil {
		t.Fatal("expected mesh, got nil")
	}
	if len(mesh.Vertices) == 0 {
		t.Fatal("expected vertices")
	}
	if len(mesh.Indices) == 0 {
		t.Fatal("expected indices")
	}
	if len(mesh.Normals) == 0 {
		t.Fatal("expected normals")
	}
	if mesh.Name != "building_1" {
		t.Fatalf("expected name 'building_1', got '%s'", mesh.Name)
	}
}

func TestGenerateFlatGround(t *testing.T) {
	mesh := GenerateFlatGround(500, 500)
	if mesh == nil {
		t.Fatal("expected mesh, got nil")
	}
	if len(mesh.Vertices) != 12 { // 4 vertices × 3 components
		t.Fatalf("expected 12 vertex components, got %d", len(mesh.Vertices))
	}
	if len(mesh.Indices) != 6 { // 2 triangles
		t.Fatalf("expected 6 indices, got %d", len(mesh.Indices))
	}
}

func TestExportGLB(t *testing.T) {
	scene := NewScene()
	scene.AddMesh(GenerateFlatGround(100, 100))

	var buf bytes.Buffer
	err := ExportGLB(scene, &buf)
	if err != nil {
		t.Fatalf("export glb error: %v", err)
	}

	data := buf.Bytes()
	if len(data) < 12 {
		t.Fatal("glb too small")
	}

	// Проверяем magic: "glTF"
	magic := string(data[0:4])
	if magic != "glTF" {
		t.Fatalf("expected glTF magic, got '%s'", magic)
	}
}

func TestExportOBJ(t *testing.T) {
	scene := NewScene()
	scene.AddMesh(GenerateFlatGround(100, 100))

	var buf bytes.Buffer
	err := ExportOBJ(scene, &buf)
	if err != nil {
		t.Fatalf("export obj error: %v", err)
	}

	content := buf.String()
	if len(content) == 0 {
		t.Fatal("obj output empty")
	}
	if !bytes.Contains([]byte(content), []byte("v ")) {
		t.Fatal("obj should contain vertex lines")
	}
	if !bytes.Contains([]byte(content), []byte("f ")) {
		t.Fatal("obj should contain face lines")
	}
}

func TestExportEmptyScene(t *testing.T) {
	scene := NewScene()

	var buf bytes.Buffer
	err := ExportGLB(scene, &buf)
	if err == nil {
		t.Fatal("expected error for empty scene")
	}
}
