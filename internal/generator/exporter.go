package generator

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"math"
)

// ExportFormat — формат экспорта.
type ExportFormat string

const (
	FormatGLB ExportFormat = "glb"
	FormatOBJ ExportFormat = "obj"
)

type accessorInfo struct {
	BufferView int
	Count      int
	CompType   int // 5126=float, 5125=uint32
	Type       string
	Min        []float64
	Max        []float64
}

type bufferViewInfo struct {
	Offset int
	Length int
	Target int // 34962=array_buffer, 34963=element_array_buffer
}

// ExportGLB экспортирует сцену в формат glTF 2.0 Binary (.glb).
func ExportGLB(scene *Scene, w io.Writer) error {
	if len(scene.Meshes) == 0 {
		return fmt.Errorf("scene has no meshes")
	}

	// Собираем бинарные данные
	var binBuf bytes.Buffer

	var accessors []accessorInfo
	var bufferViews []bufferViewInfo

	type meshPrimitive struct {
		Attributes map[string]int `json:"attributes"`
		Indices    int            `json:"indices"`
		Material   int            `json:"material"`
	}

	type gltfMesh struct {
		Name       string          `json:"name"`
		Primitives []meshPrimitive `json:"primitives"`
	}

	type gltfMaterial struct {
		Name             string                 `json:"name"`
		PBRMetallicRough map[string]interface{} `json:"pbrMetallicRoughness"`
	}

	var gltfMeshes []gltfMesh
	var materials []gltfMaterial
	var nodes []map[string]interface{}

	for meshIdx, m := range scene.Meshes {
		if len(m.Vertices) == 0 || len(m.Indices) == 0 {
			continue
		}

		// Material
		matIdx := len(materials)
		materials = append(materials, gltfMaterial{
			Name: m.Name + "_mat",
			PBRMetallicRough: map[string]interface{}{
				"baseColorFactor": []float64{
					float64(m.Color[0]),
					float64(m.Color[1]),
					float64(m.Color[2]),
					float64(m.Color[3]),
				},
				"metallicFactor":  0.1,
				"roughnessFactor": 0.8,
			},
		})

		// === Vertices buffer view ===
		vertBVIdx := len(bufferViews)
		vertOffset := binBuf.Len()
		vertData := make([]byte, len(m.Vertices)*4)
		for i, v := range m.Vertices {
			binary.LittleEndian.PutUint32(vertData[i*4:], math.Float32bits(v))
		}
		binBuf.Write(vertData)
		// Pad to 4-byte alignment
		for binBuf.Len()%4 != 0 {
			binBuf.WriteByte(0)
		}
		bufferViews = append(bufferViews, bufferViewInfo{
			Offset: vertOffset,
			Length: len(vertData),
			Target: 34962,
		})

		// Vertices accessor
		vertAccIdx := len(accessors)
		numVerts := len(m.Vertices) / 3
		minX, minY, minZ := float64(math.MaxFloat32), float64(math.MaxFloat32), float64(math.MaxFloat32)
		maxX, maxY, maxZ := float64(-math.MaxFloat32), float64(-math.MaxFloat32), float64(-math.MaxFloat32)
		for i := 0; i < numVerts; i++ {
			x, y, z := float64(m.Vertices[i*3]), float64(m.Vertices[i*3+1]), float64(m.Vertices[i*3+2])
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
		accessors = append(accessors, accessorInfo{
			BufferView: vertBVIdx,
			Count:      numVerts,
			CompType:   5126,
			Type:       "VEC3",
			Min:        []float64{minX, minY, minZ},
			Max:        []float64{maxX, maxY, maxZ},
		})

		// === Normals buffer view ===
		normBVIdx := len(bufferViews)
		normOffset := binBuf.Len()
		normData := make([]byte, len(m.Normals)*4)
		for i, v := range m.Normals {
			binary.LittleEndian.PutUint32(normData[i*4:], math.Float32bits(v))
		}
		binBuf.Write(normData)
		for binBuf.Len()%4 != 0 {
			binBuf.WriteByte(0)
		}
		bufferViews = append(bufferViews, bufferViewInfo{
			Offset: normOffset,
			Length: len(normData),
			Target: 34962,
		})

		// Normals accessor
		normAccIdx := len(accessors)
		accessors = append(accessors, accessorInfo{
			BufferView: normBVIdx,
			Count:      len(m.Normals) / 3,
			CompType:   5126,
			Type:       "VEC3",
		})

		// === Indices buffer view ===
		idxBVIdx := len(bufferViews)
		idxOffset := binBuf.Len()
		idxData := make([]byte, len(m.Indices)*4)
		for i, v := range m.Indices {
			binary.LittleEndian.PutUint32(idxData[i*4:], v)
		}
		binBuf.Write(idxData)
		for binBuf.Len()%4 != 0 {
			binBuf.WriteByte(0)
		}
		bufferViews = append(bufferViews, bufferViewInfo{
			Offset: idxOffset,
			Length: len(idxData),
			Target: 34963,
		})

		// Indices accessor
		idxAccIdx := len(accessors)
		accessors = append(accessors, accessorInfo{
			BufferView: idxBVIdx,
			Count:      len(m.Indices),
			CompType:   5125,
			Type:       "SCALAR",
		})

		// Mesh
		gltfMeshes = append(gltfMeshes, gltfMesh{
			Name: m.Name,
			Primitives: []meshPrimitive{
				{
					Attributes: map[string]int{
						"POSITION": vertAccIdx,
						"NORMAL":   normAccIdx,
					},
					Indices:  idxAccIdx,
					Material: matIdx,
				},
			},
		})

		// Node
		nodes = append(nodes, map[string]interface{}{
			"mesh": meshIdx,
			"name": m.Name,
		})
	}

	// Корневая нода — содержит все остальные
	sceneNodeIndices := make([]int, len(nodes))
	for i := range nodes {
		sceneNodeIndices[i] = i
	}

	// JSON chunk
	gltf := map[string]interface{}{
		"asset": map[string]string{
			"version":   "2.0",
			"generator": "3d-maps-generator",
		},
		"scene":  0,
		"scenes": []map[string]interface{}{{"nodes": sceneNodeIndices}},
		"nodes":  nodes,
		"meshes": gltfMeshes,
		"materials": materials,
		"accessors":  buildAccessorsJSON(accessors),
		"bufferViews": buildBufferViewsJSON(bufferViews),
		"buffers": []map[string]interface{}{
			{"byteLength": binBuf.Len()},
		},
	}

	jsonData, err := json.Marshal(gltf)
	if err != nil {
		return fmt.Errorf("marshal gltf json: %w", err)
	}

	// Pad JSON to 4-byte alignment
	for len(jsonData)%4 != 0 {
		jsonData = append(jsonData, ' ')
	}

	// Pad BIN to 4-byte alignment
	for binBuf.Len()%4 != 0 {
		binBuf.WriteByte(0)
	}

	// GLB Header
	totalLength := 12 + 8 + len(jsonData) + 8 + binBuf.Len()

	// Header: magic, version, length
	header := make([]byte, 12)
	binary.LittleEndian.PutUint32(header[0:4], 0x46546C67) // "glTF"
	binary.LittleEndian.PutUint32(header[4:8], 2)           // version
	binary.LittleEndian.PutUint32(header[8:12], uint32(totalLength))

	// JSON chunk header
	jsonChunkHeader := make([]byte, 8)
	binary.LittleEndian.PutUint32(jsonChunkHeader[0:4], uint32(len(jsonData)))
	binary.LittleEndian.PutUint32(jsonChunkHeader[4:8], 0x4E4F534A) // "JSON"

	// BIN chunk header
	binChunkHeader := make([]byte, 8)
	binary.LittleEndian.PutUint32(binChunkHeader[0:4], uint32(binBuf.Len()))
	binary.LittleEndian.PutUint32(binChunkHeader[4:8], 0x004E4942) // "BIN\0"

	// Write all
	if _, err := w.Write(header); err != nil {
		return err
	}
	if _, err := w.Write(jsonChunkHeader); err != nil {
		return err
	}
	if _, err := w.Write(jsonData); err != nil {
		return err
	}
	if _, err := w.Write(binChunkHeader); err != nil {
		return err
	}
	if _, err := w.Write(binBuf.Bytes()); err != nil {
		return err
	}

	return nil
}

// ExportOBJ экспортирует сцену в формат Wavefront OBJ.
func ExportOBJ(scene *Scene, w io.Writer) error {
	if len(scene.Meshes) == 0 {
		return fmt.Errorf("scene has no meshes")
	}

	vertexOffset := 1 // OBJ indices are 1-based

	for _, m := range scene.Meshes {
		fmt.Fprintf(w, "o %s\n", m.Name)

		numVerts := len(m.Vertices) / 3
		for i := 0; i < numVerts; i++ {
			fmt.Fprintf(w, "v %.6f %.6f %.6f\n",
				m.Vertices[i*3], m.Vertices[i*3+1], m.Vertices[i*3+2])
		}

		numNorms := len(m.Normals) / 3
		for i := 0; i < numNorms; i++ {
			fmt.Fprintf(w, "vn %.6f %.6f %.6f\n",
				m.Normals[i*3], m.Normals[i*3+1], m.Normals[i*3+2])
		}

		numTris := len(m.Indices) / 3
		for i := 0; i < numTris; i++ {
			i1 := int(m.Indices[i*3]) + vertexOffset
			i2 := int(m.Indices[i*3+1]) + vertexOffset
			i3 := int(m.Indices[i*3+2]) + vertexOffset
			fmt.Fprintf(w, "f %d//%d %d//%d %d//%d\n",
				i1, i1, i2, i2, i3, i3)
		}

		vertexOffset += numVerts
		fmt.Fprintln(w)
	}

	return nil
}

func buildAccessorsJSON(accessors []accessorInfo) []map[string]interface{} {
	result := make([]map[string]interface{}, len(accessors))
	for i, a := range accessors {
		m := map[string]interface{}{
			"bufferView":    a.BufferView,
			"componentType": a.CompType,
			"count":         a.Count,
			"type":          a.Type,
		}
		if a.Min != nil {
			m["min"] = a.Min
		}
		if a.Max != nil {
			m["max"] = a.Max
		}
		result[i] = m
	}
	return result
}

func buildBufferViewsJSON(views []bufferViewInfo) []map[string]interface{} {
	result := make([]map[string]interface{}, len(views))
	for i, v := range views {
		result[i] = map[string]interface{}{
			"buffer":     0,
			"byteOffset": v.Offset,
			"byteLength": v.Length,
			"target":     v.Target,
		}
	}
	return result
}
