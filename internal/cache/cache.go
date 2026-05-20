package cache

import (
	"container/list"
	"crypto/md5"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
)

// LRU — потокобезопасный LRU кэш с поддержкой сохранения на диск.
type LRU struct {
	mu       sync.Mutex
	capacity int
	cacheDir string
	items    map[string]*list.Element
	order    *list.List
}

type entry struct {
	key   string
	value []byte
}

// New создаёт LRU кэш с заданной ёмкостью и директорией для дискового кэша.
func New(capacity int, cacheDir string) *LRU {
	if cacheDir != "" {
		if err := os.MkdirAll(cacheDir, 0755); err != nil {
			slog.Warn("Failed to create cache directory", "dir", cacheDir, "error", err)
		}
	}
	return &LRU{
		capacity: capacity,
		cacheDir: cacheDir,
		items:    make(map[string]*list.Element),
		order:    list.New(),
	}
}

// Get возвращает значение по ключу. Сначала проверяет память, затем диск.
func (c *LRU) Get(key string) ([]byte, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// 1. Проверяем в памяти
	if el, ok := c.items[key]; ok {
		c.order.MoveToFront(el)
		return el.Value.(*entry).value, true
	}

	// 2. Если в памяти нет, но настроен диск — ищем файл
	if c.cacheDir != "" {
		filePath := c.getFilePath(key)
		if data, err := os.ReadFile(filePath); err == nil {
			// Восстанавливаем в память
			if c.order.Len() >= c.capacity {
				c.evictOldest()
			}
			el := c.order.PushFront(&entry{key: key, value: data})
			c.items[key] = el
			return data, true
		}
	}

	return nil, false
}

// Set сохраняет значение в кэш (в память и на диск).
func (c *LRU) Set(key string, value []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Если уже есть в памяти — обновляем
	if el, ok := c.items[key]; ok {
		c.order.MoveToFront(el)
		el.Value.(*entry).value = value
		if c.cacheDir != "" {
			c.saveToDiskAsync(key, value)
		}
		return
	}

	// Вытесняем старый элемент при превышении лимита
	if c.order.Len() >= c.capacity {
		c.evictOldest()
	}

	// Сохраняем в память
	el := c.order.PushFront(&entry{key: key, value: value})
	c.items[key] = el

	// Сохраняем на диск в фоне
	if c.cacheDir != "" {
		c.saveToDiskAsync(key, value)
	}
}

// evictOldest удаляет самый старый элемент из памяти.
func (c *LRU) evictOldest() {
	oldest := c.order.Back()
	if oldest != nil {
		c.order.Remove(oldest)
		delete(c.items, oldest.Value.(*entry).key)
	}
}

// getFilePath возвращает путь к файлу для конкретного ключа.
func (c *LRU) getFilePath(key string) string {
	h := md5.New()
	io.WriteString(h, key)
	fileName := fmt.Sprintf("%x.cache", h.Sum(nil))
	return filepath.Join(c.cacheDir, fileName)
}

// saveToDiskAsync асинхронно сохраняет значение на диск.
func (c *LRU) saveToDiskAsync(key string, value []byte) {
	filePath := c.getFilePath(key)
	go func() {
		err := os.WriteFile(filePath, value, 0644)
		if err != nil {
			slog.Warn("Failed to write cache to disk", "path", filePath, "error", err)
		}
	}()
}

// Len возвращает количество элементов в кэше.
func (c *LRU) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.order.Len()
}
