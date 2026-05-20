package cache

import (
	"bytes"
	"os"
	"testing"
	"time"
)

func TestLRUMemory(t *testing.T) {
	c := New(2, "") // без диска

	c.Set("k1", []byte("v1"))
	c.Set("k2", []byte("v2"))

	if val, ok := c.Get("k1"); !ok || !bytes.Equal(val, []byte("v1")) {
		t.Fatalf("expected k1 to be v1, got %s (ok=%v)", val, ok)
	}

	// Добавляем k3, должно вытеснить k2, так как k1 использовался последним
	c.Set("k3", []byte("v3"))

	if _, ok := c.Get("k2"); ok {
		t.Fatal("expected k2 to be evicted")
	}

	if val, ok := c.Get("k1"); !ok || !bytes.Equal(val, []byte("v1")) {
		t.Fatalf("expected k1 to be present and v1, got %s", val)
	}
}

func TestLRUDiskPersistence(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "geo3d-cache-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	c1 := New(5, tempDir)
	c1.Set("key1", []byte("value1"))

	// Ждем немного для асинхронной записи на диск
	time.Sleep(50 * time.Millisecond)

	// Проверяем, что файл создался на диске
	filePath := c1.getFilePath("key1")
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		t.Fatalf("expected cache file to be created on disk at %s", filePath)
	}

	// Инициализируем новый кэш с тем же путем (симулируем перезапуск сервера)
	c2 := New(5, tempDir)

	// Должен быть промах в памяти, но попадание через диск
	val, ok := c2.Get("key1")
	if !ok {
		t.Fatal("expected key1 to be recovered from disk cache")
	}
	if !bytes.Equal(val, []byte("value1")) {
		t.Fatalf("expected recovered value to be 'value1', got '%s'", val)
	}

	// Проверяем, что элемент загрузился в память
	if c2.order.Len() != 1 {
		t.Fatalf("expected 1 element in memory, got %d", c2.order.Len())
	}
}
