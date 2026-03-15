package triangulate

import (
	"testing"
)

func TestTriangulateTriangle(t *testing.T) {
	polygon := []Point{
		{0, 0},
		{1, 0},
		{0.5, 1},
	}
	result := Triangulate(polygon)
	if len(result) != 3 {
		t.Fatalf("expected 3 indices, got %d", len(result))
	}
}

func TestTriangulateSquare(t *testing.T) {
	polygon := []Point{
		{0, 0},
		{1, 0},
		{1, 1},
		{0, 1},
	}
	result := Triangulate(polygon)
	if len(result) != 6 { // 2 triangles × 3 indices
		t.Fatalf("expected 6 indices, got %d", len(result))
	}
}

func TestTriangulatePentagon(t *testing.T) {
	polygon := []Point{
		{0, 0},
		{2, 0},
		{3, 1},
		{1.5, 3},
		{-0.5, 1},
	}
	result := Triangulate(polygon)
	if len(result) != 9 { // 3 triangles × 3 indices
		t.Fatalf("expected 9 indices, got %d", len(result))
	}
}

func TestTriangulateTooFewPoints(t *testing.T) {
	polygon := []Point{
		{0, 0},
		{1, 0},
	}
	result := Triangulate(polygon)
	if result != nil {
		t.Fatalf("expected nil, got %v", result)
	}
}

func TestNormal(t *testing.T) {
	nx, ny, nz := Normal(0, 0, 0, 1, 0, 0, 0, 1, 0)
	// Normal should point in Z direction
	if abs64(nz) < 0.99 {
		t.Fatalf("expected normal in Z direction, got (%f, %f, %f)", nx, ny, nz)
	}
}

func abs64(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}
