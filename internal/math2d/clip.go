package math2d

import (
	"github.com/evgeny/3d-maps/internal/geo"
	"github.com/evgeny/3d-maps/internal/triangulate"
)

// Rect представляет собой 2D ориентированный прямоугольник Bounding Box (AABB).
type Rect struct {
	MinX float64
	MinY float64
	MaxX float64
	MaxY float64
}

// Intersects проверяет, пересекается ли прямоугольник с другим.
func (r Rect) Intersects(other Rect) bool {
	return r.MinX <= other.MaxX && r.MaxX >= other.MinX &&
		r.MinY <= other.MaxY && r.MaxY >= other.MinY
}

// PointInRect проверяет, находится ли точка внутри прямоугольника.
func (r Rect) Contains(p triangulate.Point) bool {
	return p.X >= r.MinX && p.X <= r.MaxX && p.Y >= r.MinY && p.Y <= r.MaxY
}

// ClipPolygon принимает многоугольник (контур здания) и обрезает его 
// по границам прямоугольника r (алгоритм Sutherland-Hodgman).
func ClipPolygon(subjectPolygon []triangulate.Point, r Rect) []triangulate.Point {
	if len(subjectPolygon) == 0 {
		return nil
	}

	clipEdges := [4][2]triangulate.Point{
		{{r.MinX, r.MinY}, {r.MaxX, r.MinY}}, // Bottom edge
		{{r.MaxX, r.MinY}, {r.MaxX, r.MaxY}}, // Right edge
		{{r.MaxX, r.MaxY}, {r.MinX, r.MaxY}}, // Top edge
		{{r.MinX, r.MaxY}, {r.MinX, r.MinY}}, // Left edge
	}

	result := append([]triangulate.Point(nil), subjectPolygon...)

	for _, clipEdge := range clipEdges {
		if len(result) == 0 {
			break
		}
		
		input := result
		result = make([]triangulate.Point, 0, len(input))

		// clipEdge - это линия от p1 до p2 (образует границу)
		cp1, cp2 := clipEdge[0], clipEdge[1]
		
		s := input[len(input)-1] // последняя точка = "предыдущая"

		for _, e := range input { // e = "текущая"
			sInside := isInside(s, cp1, cp2)
			eInside := isInside(e, cp1, cp2)

			if eInside {
				if !sInside {
					// Входим в фигуру - добавляем точку пересечения
					result = append(result, computeIntersection(s, e, cp1, cp2))
				}
				// Добавляем саму точку
				result = append(result, e)
			} else if sInside {
				// Выходим из фигуры - добавляем только точку пересечения
				result = append(result, computeIntersection(s, e, cp1, cp2))
			}
			s = e
		}
	}
	
	// Очищаем потенциальные дубликаты на краях (часто бывает после клиппинга)
	if len(result) > 1 {
		cleaned := []triangulate.Point{result[0]}
		for i := 1; i < len(result); i++ {
			prev := cleaned[len(cleaned)-1]
			curr := result[i]
			if abs(curr.X-prev.X) > 1e-6 || abs(curr.Y-prev.Y) > 1e-6 {
				cleaned = append(cleaned, curr)
			}
		}
		// Проверка замыкания
		if len(cleaned) > 1 && abs(cleaned[0].X-cleaned[len(cleaned)-1].X) < 1e-6 && abs(cleaned[0].Y-cleaned[len(cleaned)-1].Y) < 1e-6 {
			cleaned = cleaned[:len(cleaned)-1]
		}
		result = cleaned
	}

	return result
}

// ClipLine обрезает линию (например, дорогу) прямоугольником (AABB).
// Возвращает массив отрезанных сегментов (так как линия может выходить и входить).
func ClipLine(line []geo.Coord, r Rect, centerLat, centerLon float64) [][]geo.Coord {
	var results [][]geo.Coord
	
	// Конвертируем в 2D поинты для расчетов
	pts := make([]triangulate.Point, len(line))
	for i, c := range line {
		p := geo.LatLonToMeters(c.Lat, c.Lon, centerLat, centerLon)
		pts[i] = triangulate.Point{X: p.X, Y: p.Y}
	}

	// Cohen-Sutherland Algorithm for each segment
	for i := 0; i < len(pts)-1; i++ {
		p1, p2 := pts[i], pts[i+1]
		c1, c2 := line[i], line[i+1]
		
		accept, clipped1, clipped2 := cohenSutherlandClip(p1, p2, r)
		
		if accept {
			// Если линия была обрезана, нам надо пересчитать lat/lon. 
			// Для простоты мы передаем geo.Coord прямо в road generator,
			// но здесь нам нужно вернуть контур внутри Rect в метрах.
			
			// Создадим новую Coord для clipped точек (аппроксимация обратной проекции 
			// либо использовать их прямо там. Но проще обрезать прямо в локальных координатах перед 3D генерацией!)
			
			// На практике для дорог это сложно перевести назад в LatLon, поэтому 
			// мы будем обрезать их в road.go прямо в локальных метрах!
			
			// (Функция не используется, обрезка дорог будет сделана в road.go)
			_ = clipped1
			_ = clipped2
			_ = c1
			_ = c2
		}
	}
	
	return results
}

// Вспомогательные функции

// Предикат: "с внутренней" стороны границы
func isInside(p, cp1, cp2 triangulate.Point) bool {
	return (cp2.X-cp1.X)*(p.Y-cp1.Y)-(cp2.Y-cp1.Y)*(p.X-cp1.X) >= -1e-8
}

// Пересечение прямых
func computeIntersection(s, e, cp1, cp2 triangulate.Point) triangulate.Point {
	dcx := cp1.X - cp2.X
	dcy := cp1.Y - cp2.Y
	dpx := s.X - e.X
	dpy := s.Y - e.Y
	
	n1 := cp1.X*cp2.Y - cp1.Y*cp2.X
	n2 := s.X*e.Y - s.Y*e.X
	n3 := 1.0 / (dcx*dpy - dcy*dpx)

	return triangulate.Point{
		X: (n1*dpx - n2*dcx) * n3,
		Y: (n1*dpy - n2*dcy) * n3,
	}
}

func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}

// Cohen-Sutherland line clipping
const (
	INSIDE = 0
	LEFT   = 1
	RIGHT  = 2
	BOTTOM = 4
	TOP    = 8
)

func computeOutCode(p triangulate.Point, r Rect) int {
	code := INSIDE
	if p.X < r.MinX {
		code |= LEFT
	} else if p.X > r.MaxX {
		code |= RIGHT
	}
	if p.Y < r.MinY {
		code |= BOTTOM
	} else if p.Y > r.MaxY {
		code |= TOP
	}
	return code
}

func cohenSutherlandClip(p1, p2 triangulate.Point, r Rect) (bool, triangulate.Point, triangulate.Point) {
	outcode1 := computeOutCode(p1, r)
	outcode2 := computeOutCode(p2, r)
	accept := false

	for {
		if outcode1 == 0 && outcode2 == 0 {
			accept = true
			break
		} else if (outcode1 & outcode2) != 0 {
			break
		} else {
			var currOutcode int
			if outcode1 != 0 {
				currOutcode = outcode1
			} else {
				currOutcode = outcode2
			}

			var x, y float64
			if (currOutcode & TOP) != 0 {
				x = p1.X + (p2.X-p1.X)*(r.MaxY-p1.Y)/(p2.Y-p1.Y)
				y = r.MaxY
			} else if (currOutcode & BOTTOM) != 0 {
				x = p1.X + (p2.X-p1.X)*(r.MinY-p1.Y)/(p2.Y-p1.Y)
				y = r.MinY
			} else if (currOutcode & RIGHT) != 0 {
				y = p1.Y + (p2.Y-p1.Y)*(r.MaxX-p1.X)/(p2.X-p1.X)
				x = r.MaxX
			} else if (currOutcode & LEFT) != 0 {
				y = p1.Y + (p2.Y-p1.Y)*(r.MinX-p1.X)/(p2.X-p1.X)
				x = r.MinX
			}

			if currOutcode == outcode1 {
				p1.X = x
				p1.Y = y
				outcode1 = computeOutCode(p1, r)
			} else {
				p2.X = x
				p2.Y = y
				outcode2 = computeOutCode(p2, r)
			}
		}
	}
	return accept, p1, p2
}
