# 🏙️ 3D Maps Generator

Go-бэкенд для генерации 3D-моделей участков городов в форматах glTF (.glb) и OBJ.

## Возможности

- 📍 Генерация 3D-модели по координатам или названию города
- 🏢 3D-здания с реальными контурами и высотами из OpenStreetMap
- 🛣️ Дороги с шириной по количеству полос
- ⛰️ Рельеф из Open Elevation API
- 📦 Экспорт в glTF 2.0 (.glb) и Wavefront OBJ

## Быстрый старт

1. Установите зависимости:
```bash
go mod download
```

```bash
# Копирование конфига
cp .env.example .env

# Запуск
make run

# Или напрямую
go run ./cmd/server/
```

Сервер стартует на `http://localhost:8080`.

## API

### Генерация модели

```bash
curl -X POST http://localhost:8080/api/v1/generate \
  -H "Content-Type: application/json" \
  -d '{
    "city": "Moscow",
    "width": 300,
    "height": 300,
    "format": "glb",
    "include_roads": true,
    "include_terrain": false
  }' -o moscow.glb
```

### По координатам

```bash
curl -X POST http://localhost:8080/api/v1/generate \
  -H "Content-Type: application/json" \
  -d '{
    "lat": 55.7558,
    "lon": 37.6173,
    "width": 500,
    "height": 500,
    "format": "obj"
  }' -o center.obj
```

### Геокодирование

```bash
curl "http://localhost:8080/api/v1/geocode?q=Saint+Petersburg"
```

### Параметры генерации

| Параметр | Тип | По умолчанию | Описание |
|----------|-----|-------------|----------|
| `city` | string | — | Название города (геокодируется через Nominatim) |
| `lat` | float | — | Широта центра |
| `lon` | float | — | Долгота центра |
| `width` | float | 500 | Ширина области в метрах (макс. 2000) |
| `height` | float | 500 | Высота области в метрах (макс. 2000) |
| `format` | string | `glb` | Формат: `glb`, `obj` или `stl` |
| `include_terrain` | bool | false | Включить рельег |
| `include_roads` | bool | false | Включить дороги |
| `print_ready` | bool | false | Если `true`, добавляет основу, масштабирует и ставит формат `stl` |
| `scale` | float | 1.0 | Масштаб для 3D-печати. Пример: `0.002` = 1:500 (2мм на 1 метр) |
| `base_thickness` | float | 3.0 | Толщина платформы-основы в миллиметрах |

### Пример генерации для 3D-печати

```bash
curl -X POST http://localhost:8080/api/v1/generate \
  -H "Content-Type: application/json" \
  -d '{
    "city": "Moscow",
    "width": 300,
    "height": 300,
    "print_ready": true,
    "scale": 0.5,
    "base_thickness": 3.0
  }' -o moscow_print.stl
```

## Тесты

```bash
make test
```

## Просмотр моделей

- [glTF Viewer](https://gltf-viewer.donmccurdy.com/) — веб-просмотр .glb
- Blender — импорт .glb и .obj
