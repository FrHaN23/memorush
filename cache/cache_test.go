package cache_test

import (
	"bytes"
	"encoding/gob"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/frhan23/memorush/cache"
)

type testStruct struct {
	Name string
	Age  int
}

type UnregisteredType struct {
	Data string
}

func init() {
	gob.Register([]string{})
	gob.Register(map[string]int{})
	gob.Register(testStruct{})
}

// Test nil config uses defaults
func TestCacheDefaultConfig(t *testing.T) {
	c := cache.NewCache(nil)
	c.Set("key", "value")
	val, found := c.Get("key")
	if !found {
		t.Error("value not found with default config")
	}
	if val != "value" {
		t.Errorf("got %v, want 'value'", val)
	}
}

func TestStartPersistence(t *testing.T) {
	// Create a temporary test file
	testFile := "test_cache.gob"
	defer os.Remove(testFile) // Cleanup after test

	// Create cache with persistence
	cache := cache.NewCache(&cache.CacheConfig{PersistTo: testFile})

	// Allow the persistence function to run at least once
	time.Sleep(500 * time.Millisecond)

	// Stop the persistence goroutine
	cache.Close() // ✅ Stops the loop

	// Give some time to ensure persistence stopped
	time.Sleep(100 * time.Millisecond)

	// Check if the file was created
	if _, err := os.Stat(testFile); os.IsNotExist(err) {
		t.Fatalf("expected cache file to be created, but it does not exist")
	}
}

type MockCache struct {
	*cache.Cache
	SaveToDiskCount int
}

func (m *MockCache) SaveToDisk() {
	m.SaveToDiskCount++
	m.Cache.SaveToDisk() // Call actual method if needed
}

func TestCache_StartPersistence(t *testing.T) {
	// Create cache with a mock persist path
	c := cache.NewCache(&cache.CacheConfig{PersistTo: "test_persistence.gob", PersistDuration: 10 * time.Millisecond})

	// Let it run briefly
	time.Sleep(50 * time.Millisecond)

	// Stop persistence
	c.Close()
	time.Sleep(20 * time.Millisecond) // Ensure loop exits
}

func TestSerialize_Error(t *testing.T) {
	// Create a CacheItem with an unregistered type
	item := &cache.CacheItem{
		Key:   "testKey",
		Value: UnregisteredType{Data: "unserializable"}, // Not registered with gob
	}

	// Try to serialize
	_, err := item.Serialize()

	// Expect an error
	if err == nil {
		t.Fatal("expected an error when serializing an unregistered type, but got nil")
	}
}

func TestDeserialize_Error(t *testing.T) {
	// Provide corrupted data
	corruptData := []byte{0x01, 0x02, 0x03}

	// Try to deserialize
	_, err := cache.Deserialize(corruptData)

	// Expect an error
	if err == nil {
		t.Fatal("expected an error when deserializing invalid data, but got nil")
	}
}

func TestCacheDifferentTypes(t *testing.T) {
	c := cache.NewCache(nil)

	// Simple types that can be compared directly
	simpleTests := []struct {
		name  string
		key   string
		value interface{}
	}{
		{"integer", "int", 42},
		{"float", "float", 3.14},
		{"boolean", "bool", true},
		{"string", "string", "hello"},
	}

	for _, tc := range simpleTests {
		t.Run(tc.name, func(t *testing.T) {
			c.Set(tc.key, tc.value)
			val, found := c.Get(tc.key)
			if !found {
				t.Errorf("value not found for type %s", tc.name)
			}
			if val != tc.value {
				t.Errorf("got %v, want %v", val, tc.value)
			}
		})
	}

	// Complex types that need deep comparison
	t.Run("slice", func(t *testing.T) {
		slice := []string{"a", "b"}
		c.Set("slice", slice)
		val, found := c.Get("slice")
		if !found {
			t.Error("slice not found")
		}
		if got, ok := val.([]string); !ok {
			t.Error("couldn't convert value back to []string")
		} else if !reflect.DeepEqual(got, slice) {
			t.Errorf("got %v, want %v", got, slice)
		}
	})

	t.Run("map", func(t *testing.T) {
		m := map[string]int{"a": 1}
		c.Set("map", m)
		val, found := c.Get("map")
		if !found {
			t.Error("map not found")
		}
		if got, ok := val.(map[string]int); !ok {
			t.Error("couldn't convert value back to map[string]int")
		} else if !reflect.DeepEqual(got, m) {
			t.Errorf("got %v, want %v", got, m)
		}
	})

	t.Run("struct", func(t *testing.T) {
		type testStruct struct {
			Name string
			Age  int
		}
		s := testStruct{Name: "test", Age: 25}
		c.Set("struct", s)
		val, found := c.Get("struct")
		if !found {
			t.Error("struct not found")
		}
		if got, ok := val.(testStruct); !ok {
			t.Error("couldn't convert value back to testStruct")
		} else if !reflect.DeepEqual(got, s) {
			t.Errorf("got %v, want %v", got, s)
		}
	})
}

// Test for persistence with complex types
// func TestCachePersistenceComplexTypes(t *testing.T) {
// 	cacheFile := "test_complex_types.gob"
// 	defer func() {
// 		if err := os.Remove(cacheFile); err != nil {
// 			t.Logf("Failed to remove test file: %v", err)
// 		}
// 	}()

// 	config := &cache.CacheConfig{
// 		Capacity:  10,
// 		TTL:       5 * time.Minute,
// 		PersistTo: cacheFile,
// 	}

// 	c := cache.NewCache(config)

// 	// Test slice
// 	slice := []string{"a", "b"}
// 	c.Set("slice", slice, time.Hour)

// 	// Test map
// 	m := map[string]int{"a": 1, "b": 2}
// 	c.Set("map", m, time.Hour)

// 	s := testStruct{Name: "test", Age: 25}
// 	c.Set("struct", s, time.Hour)

// 	c.SaveToDisk()

// 	// Load in new cache
// 	newCache := cache.NewCache(config)
// 	newCache.LoadFromDisk()

// 	// Check slice
// 	if val, found := newCache.Get("slice"); !found {
// 		t.Error("slice not found after load")
// 	} else {
// 		got, ok := val.([]interface{}) // check []interface{}
// 		if !ok {
// 			t.Errorf("expected []interface{}, got %T", val)
// 		} else {
// 			converted := make([]string, len(got))
// 			for i, v := range got {
// 				if str, ok := v.(string); ok {
// 					converted[i] = str
// 				} else {
// 					t.Errorf("unexpected type in slice: %T", v)
// 				}
// 			}
// 			if !reflect.DeepEqual(converted, slice) {
// 				t.Errorf("got %v, want %v", converted, slice)
// 			}
// 		}
// 	}

// 	// Check map
// 	if val, found := newCache.Get("map"); !found {
// 		t.Error("map not found after load")
// 	} else if got, ok := val.(map[string]int); !ok {
// 		t.Error("couldn't convert loaded value back to map[string]int")
// 	} else if !reflect.DeepEqual(got, m) {
// 		t.Errorf("got %v, want %v", got, m)
// 	}

// 	// Check struct
// 	if val, found := newCache.Get("struct"); !found {
// 		t.Error("struct not found after load")
// 	} else if got, ok := val.(testStruct); !ok {
// 		t.Error("couldn't convert loaded value back to testStruct")
// 	} else if !reflect.DeepEqual(got, s) {
// 		t.Errorf("got %v, want %v", got, s)
// 	}
// }

// Test Set and Get operations
func TestCacheSetGet(t *testing.T) {
	cache := cache.NewCache(&cache.CacheConfig{Capacity: 2, TTL: 1 * time.Minute})

	cache.Set("key1", "value1")
	cache.Set("key2", "value2")

	val, found := cache.Get("key1")
	if !found || val.(string) != "value1" {
		t.Errorf("expected value1, got %v", val)
	}
}

// Test that expired items are removed
func TestCacheExpiration(t *testing.T) {
	cache := cache.NewCache(&cache.CacheConfig{Capacity: 2, TTL: 1 * time.Second})

	cache.Set("key1", "value1", 1*time.Second)
	time.Sleep(2 * time.Second)

	_, found := cache.Get("key1")
	if found {
		t.Errorf("expected key1 to be expired and removed")
	}
}

// Test LRU eviction (removes the least recently used item)
func TestCacheLRUEviction(t *testing.T) {
	cache := cache.NewCache(&cache.CacheConfig{Capacity: 2, TTL: 1 * time.Minute})

	cache.Set("key1", "value1")
	cache.Set("key2", "value2")
	cache.Set("key3", "value3") // Should evict key1

	_, found := cache.Get("key1")
	if found {
		t.Errorf("expected key1 to be evicted")
	}
}

// Test deleting an item
func TestCacheDelete(t *testing.T) {
	cache := cache.NewCache(&cache.CacheConfig{Capacity: 2, TTL: 1 * time.Minute})

	cache.Set("key1", "value1")
	cache.Delete("key1")

	_, found := cache.Get("key1")
	if found {
		t.Errorf("expected key1 to be deleted")
	}
}

// Test updating existing items
func TestCacheUpdateExisting(t *testing.T) {
	c := cache.NewCache(nil)
	c.Set("key", "value1")
	c.Set("key", "value2") // Update
	val, found := c.Get("key")
	if !found {
		t.Error("updated value not found")
	}
	if val != "value2" {
		t.Errorf("got %v, want 'value2'", val)
	}
}

// Test persistence (saving & loading from disk)
func TestCachePersistence(t *testing.T) {
	cacheFile := "test_cache.gob"
	defer func() {
		if err := os.Remove(cacheFile); err != nil && !os.IsNotExist(err) {
			t.Logf("Failed to remove test file: %v", err)
		}
	}()
	// Create a cache config
	config := &cache.CacheConfig{
		Capacity:  10,
		TTL:       5 * time.Minute,
		PersistTo: cacheFile,
	}

	// Initialize cache with new config
	cached := cache.NewCache(config)

	// Add test items
	cached.Set("key1", "value1", 10*time.Minute)
	cached.Set("key2", "value2", 10*time.Minute)

	// Manually trigger save
	cached.SaveToDisk()

	// Verify file exists
	if _, err := os.Stat(cacheFile); os.IsNotExist(err) {
		t.Fatalf("Cache file was not created")
	}

	// Load cache from disk
	newCache := cache.NewCache(config)
	newCache.LoadFromDisk()

	// Validate values are restored
	if val, found := newCache.Get("key1"); !found || val != "value1" {
		t.Errorf("Expected key1 to be 'value1', got '%v'", val)
	}
	if val, found := newCache.Get("key2"); !found || val != "value2" {
		t.Errorf("Expected key2 to be 'value2', got '%v'", val)
	}
}

// Test cleanup of expired items
func TestCacheCleanup(t *testing.T) {
	c := cache.NewCache(&cache.CacheConfig{
		Capacity:        5,
		CleanupInterval: 100 * time.Millisecond,
	})

	// Add items with short TTL
	c.Set("key1", "value1", 50*time.Millisecond)
	c.Set("key2", "value2", 200*time.Millisecond)

	// Wait for cleanup
	time.Sleep(150 * time.Millisecond)

	// key1 should be cleaned up, key2 should still exist
	if _, found := c.Get("key1"); found {
		t.Error("key1 should have been cleaned up")
	}
	if _, found := c.Get("key2"); !found {
		t.Error("key2 should still exist")
	}
}

// Test edge cases
func TestCacheEdgeCases(t *testing.T) {
	c := cache.NewCache(nil)

	t.Run("delete non-existent", func(t *testing.T) {
		// Should not panic
		c.Delete("non-existent")
	})

	t.Run("get non-existent", func(t *testing.T) {
		_, found := c.Get("non-existent")
		if found {
			t.Error("found non-existent key")
		}
	})

	t.Run("zero capacity", func(t *testing.T) {
		zc := cache.NewCache(&cache.CacheConfig{Capacity: 0})
		zc.Set("key", "value")
		_, found := zc.Get("key")
		if !found {
			t.Error("zero capacity should use default")
		}
	})
}

// Test concurrency (multiple goroutines)
func TestCacheConcurrency(t *testing.T) {
	cache := cache.NewCache(&cache.CacheConfig{Capacity: 100, TTL: 1 * time.Minute})

	done := make(chan bool)
	for i := 0; i < 100; i++ {
		go func(i int) {
			cache.Set(string(rune(i)), i)
			done <- true
		}(i)
	}

	// Wait for all goroutines to finish
	for i := 0; i < 100; i++ {
		<-done
	}
}

// Test concurrent access patterns
func TestCacheConcurrentAccess(t *testing.T) {
	c := cache.NewCache(&cache.CacheConfig{Capacity: 100})
	done := make(chan bool)

	// Concurrent reads and writes
	for i := 0; i < 100; i++ {
		go func(i int) {
			key := string(rune(i))
			c.Set(key, i)
			time.Sleep(time.Millisecond)
			c.Get(key)
			c.Delete(key)
			done <- true
		}(i)
	}

	// Wait for all operations
	for i := 0; i < 100; i++ {
		<-done
	}
}

// Test persistence with different data types
func TestCachePersistenceTypes(t *testing.T) {
	cacheFile := "test_types_cache.gob"
	defer func() {
		if err := os.Remove(cacheFile); err != nil && !os.IsNotExist(err) {
			t.Logf("Failed to remove test file: %v", err)
		}
	}()

	config := &cache.CacheConfig{
		Capacity:  10,
		TTL:       5 * time.Minute,
		PersistTo: cacheFile,
	}

	c := cache.NewCache(config)

	// Store different types
	testData := map[string]interface{}{
		"string": "test",
		"int":    42,
		"float":  3.14,
		"bool":   true,
		"slice":  []string{"a", "b"},
	}

	for k, v := range testData {
		c.Set(k, v, time.Hour)
	}

	c.SaveToDisk()

	// Load in new cache
	newCache := cache.NewCache(config)
	newCache.LoadFromDisk()

	// Verify all types
	for k, v := range testData {
		val, found := newCache.Get(k)
		if !found {
			t.Errorf("failed to load %s", k)
			continue
		}
		if !reflect.DeepEqual(val, v) { // Gunakan reflect.DeepEqual
			t.Errorf("for %s: got %v, want %v", k, val, v)
		}
	}
}

// Test TTL behavior
func TestCacheTTL(t *testing.T) {
	c := cache.NewCache(nil)

	t.Run("custom TTL", func(t *testing.T) {
		c.Set("key", "value", 50*time.Millisecond)
		time.Sleep(75 * time.Millisecond)
		if _, found := c.Get("key"); found {
			t.Error("item should have expired")
		}
	})

	t.Run("default TTL", func(t *testing.T) {
		c.Set("key2", "value2")
		if _, found := c.Get("key2"); !found {
			t.Error("item should not have expired")
		}
	})
}

func TestCacheLoadNonExistentFile(t *testing.T) {
	cacheFile := "non_existent_cache.gob"
	defer func() {
		if err := os.Remove(cacheFile); err != nil && !os.IsNotExist(err) {
			t.Logf("Failed to remove test file: %v", err)
		}
	}()

	config := &cache.CacheConfig{
		Capacity:  10,
		TTL:       5 * time.Minute,
		PersistTo: cacheFile,
	}

	// Verify file doesn't exist
	if _, err := os.Stat(cacheFile); !os.IsNotExist(err) {
		t.Fatal("Test file should not exist before test")
	}

	// Create cache and load (should create new file)
	c := cache.NewCache(config)
	c.LoadFromDisk()

	// Verify file was created
	if _, err := os.Stat(cacheFile); os.IsNotExist(err) {
		t.Fatal("Cache file should have been created")
	}

	// Verify it's a valid empty cache file
	file, err := os.Open(cacheFile)
	if err != nil {
		t.Fatalf("Failed to open created cache file: %v", err)
	}
	defer file.Close()

	fileInfo, err := file.Stat()
	if err != nil {
		t.Fatalf("Failed to get file info: %v", err)
	}
	if fileInfo.Size() == 0 {
		t.Fatal("Created cache file is empty and not a valid gob file")
	}

	// Attempt to decode the file
	decoder := gob.NewDecoder(file)
	data := make(map[string]*cache.CacheItem)
	if err := decoder.Decode(&data); err != nil {
		t.Fatalf("Created file should be a valid gob file: %v", err)
	}
	if len(data) != 0 {
		t.Error("New cache file should be empty")
	}
}

// TestNewCacheItem checks if NewCacheItem correctly initializes cache items.
func TestNewCacheItem(t *testing.T) {
	ttl := 2 * time.Second
	item := cache.NewCacheItem("testKey", "testValue", ttl)

	if item.Key != "testKey" {
		t.Errorf("expected key 'testKey', got %s", item.Key)
	}

	if item.Value != "testValue" {
		t.Errorf("expected value 'testValue', got %v", item.Value)
	}

	// Check if expiration is set correctly
	if item.Expiration.IsZero() {
		t.Errorf("expected expiration to be set, got zero time")
	}
}

// TestIsExpired checks if expiration logic works correctly.
func TestIsExpired(t *testing.T) {
	item := cache.NewCacheItem("expiringKey", "someValue", 1*time.Millisecond)

	time.Sleep(2 * time.Millisecond) // Ensure item is expired
	if !item.IsExpired() {
		t.Errorf("expected item to be expired, but it is not")
	}

	item2 := cache.NewCacheItem("nonExpiringKey", "someValue", 0)
	if item2.IsExpired() {
		t.Errorf("expected item with zero TTL to never expire, but it did")
	}
}

// TestSerialization checks if CacheItem can be serialized and deserialized.
func TestSerialization(t *testing.T) {
	original := cache.NewCacheItem("serialKey", "serialValue", 1*time.Hour)

	data, err := original.Serialize()
	if err != nil {
		t.Fatalf("failed to serialize cache item: %v", err)
	}

	deserialized, err := cache.Deserialize(data)
	if err != nil {
		t.Fatalf("failed to deserialize cache item: %v", err)
	}

	if deserialized.Key != original.Key {
		t.Errorf("expected key %s, got %s", original.Key, deserialized.Key)
	}

	if deserialized.Value != original.Value {
		t.Errorf("expected value %v, got %v", original.Value, deserialized.Value)
	}

	if !deserialized.Expiration.Equal(original.Expiration) {
		t.Errorf("expected expiration %v, got %v", original.Expiration, deserialized.Expiration)
	}
}

// TestGobEncoding ensures that the gob package can encode and decode CacheItem properly.
func TestGobEncoding(t *testing.T) {
	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)
	dec := gob.NewDecoder(&buf)

	original := cache.NewCacheItem("gobKey", "gobValue", 10*time.Minute)

	err := enc.Encode(original)
	if err != nil {
		t.Fatalf("failed to encode cache item: %v", err)
	}

	var decoded cache.CacheItem
	err = dec.Decode(&decoded)
	if err != nil {
		t.Fatalf("failed to decode cache item: %v", err)
	}

	if decoded.Key != original.Key || decoded.Value != original.Value {
		t.Errorf("gob decoded item does not match original")
	}
}
