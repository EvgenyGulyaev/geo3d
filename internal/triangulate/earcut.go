package triangulate

import "math"

// Point представляет 2D-точку.
type Point struct {
	X, Y float64
}

// Triangulate выполняет триангуляцию простого полигона методом Ear Clipping.
// Возвращает индексы треугольников (каждые три — один треугольник).
func Triangulate(polygon []Point) []int {
	n := len(polygon)
	if n < 3 {
		return nil
	}

	// Создаём список индексов
	indices := make([]int, n)
	if area(polygon) > 0 {
		// против часовой стрелки
		for i := 0; i < n; i++ {
			indices[i] = i
		}
	} else {
		// по часовой стрелке — реверсируем
		for i := 0; i < n; i++ {
			indices[i] = n - 1 - i
		}
	}

	var result []int
	nv := n
	count := 2 * nv // защита от бесконечного цикла

	for v := nv - 1; nv > 2; {
		if count--; count <= 0 {
			// Не удалось триангулировать — возвращаем что есть
			break
		}

		// Три последовательных вершины
		u := v
		if u >= nv {
			u = 0
		}
		v = u + 1
		if v >= nv {
			v = 0
		}
		w := v + 1
		if w >= nv {
			w = 0
		}

		if isEar(polygon, indices, u, v, w, nv) {
			result = append(result, indices[u], indices[v], indices[w])

			// Удаляем вершину v
			for s := v; s < nv-1; s++ {
				indices[s] = indices[s+1]
			}
			nv--
			count = 2 * nv
		}
	}

	return result
}

// area вычисляет площадь полигона (знаковую).
func area(polygon []Point) float64 {
	n := len(polygon)
	a := 0.0
	for p, q := n-1, 0; q < n; p, q = q, q+1 {
		a += polygon[p].X*polygon[q].Y - polygon[q].X*polygon[p].Y
	}
	return a * 0.5
}

// isEar проверяет, является ли тройка вершин (u, v, w) ухом.
func isEar(polygon []Point, indices []int, u, v, w, n int) bool {
	ax := polygon[indices[u]].X
	ay := polygon[indices[u]].Y
	bx := polygon[indices[v]].X
	by := polygon[indices[v]].Y
	cx := polygon[indices[w]].X
	cy := polygon[indices[w]].Y

	// Проверка выпуклости
	cross := (bx-ax)*(cy-ay) - (by-ay)*(cx-ax)
	if cross < 1e-10 {
		return false
	}

	// Проверка, что ни одна другая вершина не лежит внутри треугольника
	for p := 0; p < n; p++ {
		if p == u || p == v || p == w {
			continue
		}
		px := polygon[indices[p]].X
		py := polygon[indices[p]].Y
		if pointInTriangle(px, py, ax, ay, bx, by, cx, cy) {
			return false
		}
	}

	return true
}

// pointInTriangle проверяет, лежит ли точка (px,py) внутри треугольника.
func pointInTriangle(px, py, ax, ay, bx, by, cx, cy float64) bool {
	d1 := sign(px, py, ax, ay, bx, by)
	d2 := sign(px, py, bx, by, cx, cy)
	d3 := sign(px, py, cx, cy, ax, ay)

	hasNeg := (d1 < 0) || (d2 < 0) || (d3 < 0)
	hasPos := (d1 > 0) || (d2 > 0) || (d3 > 0)

	return !(hasNeg && hasPos)
}

func sign(x1, y1, x2, y2, x3, y3 float64) float64 {
	return (x1-x3)*(y2-y3) - (x2-x3)*(y1-y3)
}

// Normal вычисляет нормаль к треугольнику.
func Normal(ax, ay, az, bx, by, bz, cx, cy, cz float64) (float64, float64, float64) {
	ux, uy, uz := bx-ax, by-ay, bz-az
	vx, vy, vz := cx-ax, cy-ay, cz-az

	nx := uy*vz - uz*vy
	ny := uz*vx - ux*vz
	nz := ux*vy - uy*vx

	length := math.Sqrt(nx*nx + ny*ny + nz*nz)
	if length > 0 {
		nx /= length
		ny /= length
		nz /= length
	}

	return nx, ny, nz
}
