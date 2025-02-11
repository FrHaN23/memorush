package cache

import (
	"container/list"
	"encoding/gob"
	"log"
	"os"
	"sync"
	"time"
)

// Default values
const (
	defaultCapacity        = 100
	defaultCleanupInterval = 10 * time.Second
	defaultTTL             = 5 * time.Minute
	defaultPersistFile     = "cache_data.gob"
	defaultPersistDuration = 30 * time.Second
)

type CacheConfig struct {
	Capacity        int
	TTL             time.Duration
	CleanupInterval time.Duration
	PersistTo       string
	PersistDuration time.Duration
}

type Cache struct {
	capacity  int
	mutex     sync.Mutex
	items     map[string]*list.Element
	eviction  *list.List
	stopped   chan struct{}
	persistTo string
}

// NewCache creates a new LRU cache with a given capacity and TTL
func NewCache(cfg *CacheConfig) *Cache {
	if cfg == nil {
		cfg = &CacheConfig{}
	}
	if cfg.Capacity == 0 {
		cfg.Capacity = defaultCapacity
	}
	if cfg.CleanupInterval == 0 {
		cfg.CleanupInterval = defaultCleanupInterval
	}
	persistPath := cfg.PersistTo
	if persistPath == "" {
		persistPath = defaultPersistFile
	}
	if cfg.PersistDuration == 0 {
		cfg.PersistDuration = defaultPersistDuration
	}

	cache := &Cache{
		capacity:  cfg.Capacity,
		items:     make(map[string]*list.Element),
		eviction:  list.New(),
		persistTo: persistPath,
		stopped:   make(chan struct{}),
	}
	cache.LoadFromDisk()

	go cache.cleanup(cfg.CleanupInterval)
	go cache.startPersistence(cfg.PersistDuration)

	return cache
}

// Save cache to disk periodically
func (c *Cache) startPersistence(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			c.SaveToDisk()
		case <-c.stopped:
			return
		}
	}
}

// Save cache to disk
func (c *Cache) SaveToDisk() {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	file, err := os.Create(c.persistTo)
	if err != nil {
		log.Printf("Failed to create cache file: %v", err)
		return
	}
	defer file.Close()

	encoder := gob.NewEncoder(file)
	data := make(map[string]*CacheItem)

	// Collect non-expired items
	for key, elem := range c.items {
		item, ok := elem.Value.(*CacheItem)
		if !ok {
			log.Printf("Failed to type assert item for key %s", key)
			continue
		}
		if !item.IsExpired() {
			data[key] = item
		}
	}

	if err := encoder.Encode(data); err != nil {
		log.Printf("Failed to encode cache items: %v", err)
		return
	}
	log.Printf("Cache successfully saved to %s", c.persistTo)
}

func (c *Cache) LoadFromDisk() {
	c.mutex.Lock()
	file, err := os.Open(c.persistTo)
	c.mutex.Unlock()

	if os.IsNotExist(err) {
		c.SaveToDisk()
		return
	} else if err != nil {
		log.Printf("Failed to open cache file %s: %v", c.persistTo, err)
		return
	}
	defer file.Close()

	decoder := gob.NewDecoder(file)
	data := make(map[string]*CacheItem)

	if err := decoder.Decode(&data); err != nil {
		log.Printf("Failed to decode cache items: %v", err)
		return
	}

	// Clear existing cache
	c.items = make(map[string]*list.Element)
	c.eviction.Init()

	// Restore non-expired items
	count := 0
	for key, item := range data {
		if item.IsExpired() {
			continue
		}
		elem := c.eviction.PushFront(item)
		c.items[key] = elem
		count++
	}
	log.Printf("Cache successfully loaded from %s", c.persistTo)
}

// Get retrieves a value from the cache
func (c *Cache) Get(key string) (interface{}, bool) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	elem, found := c.items[key]
	if !found {
		return nil, false
	}

	item := elem.Value.(*CacheItem)
	if time.Now().After(item.Expiration) {
		c.removeElement(elem)
		return nil, false
	}

	// Update LastAccessed timestamp
	item.LastAccessed = time.Now()
	if item.SlidingTTL > 0 {
		newTTL := item.SlidingTTL * 2 // Double the TTL
		maxTTL := item.TTL * 10       // Limit max TTL to 10x initial TTL

		if newTTL > maxTTL {
			newTTL = maxTTL // Don't let it grow indefinitely
		}

		item.SlidingTTL = newTTL
		item.Expiration = time.Now().Add(newTTL)
	}

	c.eviction.MoveToFront(elem)
	return item.Value, true
}

// Get retrieves a value from the cache
func (c *Cache) InspectItem(key string) (*CacheItem, bool) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	element, exists := c.items[key]
	if !exists {
		return nil, false
	}

	return element.Value.(*CacheItem), true
}

// Set adds a key-value pair to the cache
func (c *Cache) Set(key string, value interface{}, ttl ...time.Duration) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	expiration := time.Now().Add(defaultTTL)
	if len(ttl) > 0 {
		expiration = time.Now().Add(ttl[0]) // Use provided TTL if given
	}

	var slidingTTL time.Duration
	if len(ttl) > 0 {
		expiration = time.Now().Add(ttl[0])
		slidingTTL = ttl[0] // Store for sliding TTL logic
	} else {
		slidingTTL = defaultTTL // Default sliding TTL if none provided
	}

	// If key already exists, update and move to front
	if elem, found := c.items[key]; found {
		item, ok := elem.Value.(*CacheItem)
		if !ok {
			log.Printf("Failed to type assert item for key %s, got type %T", key, elem.Value)
			return
		}
		item.Value = value
		item.Expiration = expiration
		item.LastAccessed = time.Now()
		item.SlidingTTL = slidingTTL
		c.eviction.MoveToFront(elem)
		return
	}

	// Evict if at capacity
	if len(c.items) >= c.capacity {
		c.evict()
	}

	// Add new item
	item := &CacheItem{
		Key:        key,
		Value:      value,
		Expiration: expiration,
		SlidingTTL: slidingTTL,
		TTL: slidingTTL,
	}
	elem := c.eviction.PushFront(item)
	c.items[key] = elem
}

// Delete removes a key from the cache
func (c *Cache) Delete(key string) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if elem, found := c.items[key]; found {
		c.removeElement(elem)
	}
}

// evict removes the least recently used item
func (c *Cache) evict() {
	elem := c.eviction.Back()
	if elem != nil {
		c.removeElement(elem)
	}
}

// removeElement removes an element from the cache
func (c *Cache) removeElement(elem *list.Element) {
	c.eviction.Remove(elem)
	item := elem.Value.(*CacheItem)
	delete(c.items, item.Key)
}

// cleanup runs a background goroutine to remove expired items
func (c *Cache) cleanup(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			c.cleanupExpiredItems()
		case <-c.stopped:
			return
		}
	}
}

// cleanupExpiredItems removes expired items from the cache
func (c *Cache) cleanupExpiredItems() {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	for elem := c.eviction.Back(); elem != nil; {
		prev := elem.Prev()
		item := elem.Value.(*CacheItem)
		if time.Now().After(item.Expiration) {
			c.removeElement(elem)
		}
		elem = prev
	}
}

// StopCleanup stops the background cleanup process
func (c *Cache) stopCleanup() {
	close(c.stopped)
}

func (c *Cache) Close() {
	c.stopCleanup()
	c.SaveToDisk()
}
