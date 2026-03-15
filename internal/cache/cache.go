package cache

import (
	"container/list"
	"sync"
)

// LRU — потокобезопасный LRU кэш.
type LRU struct {
	mu       sync.Mutex
	capacity int
	items    map[string]*list.Element
	order    *list.List
}

type entry struct {
	key   string
	value []byte
}

// New создаёт LRU кэш с заданной ёмкостью.
func New(capacity int) *LRU {
	return &LRU{
		capacity: capacity,
		items:    make(map[string]*list.Element),
		order:    list.New(),
	}
}

// Get возвращает значение по ключу.
func (c *LRU) Get(key string) ([]byte, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if el, ok := c.items[key]; ok {
		c.order.MoveToFront(el)
		return el.Value.(*entry).value, true
	}
	return nil, false
}

// Set сохраняет значение в кэш.
func (c *LRU) Set(key string, value []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if el, ok := c.items[key]; ok {
		c.order.MoveToFront(el)
		el.Value.(*entry).value = value
		return
	}

	if c.order.Len() >= c.capacity {
		// Удаляем самый старый
		oldest := c.order.Back()
		if oldest != nil {
			c.order.Remove(oldest)
			delete(c.items, oldest.Value.(*entry).key)
		}
	}

	el := c.order.PushFront(&entry{key: key, value: value})
	c.items[key] = el
}

// Len возвращает количество элементов в кэше.
func (c *LRU) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.order.Len()
}
