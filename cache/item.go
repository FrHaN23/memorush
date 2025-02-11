package cache

import (
	"bytes"
	"encoding/gob"
	"time"
)

// CacheItem represents an individual cache entry
type CacheItem struct {
	Key          string
	Value        interface{}
	Expiration   time.Time
	LastAccessed time.Time // New field for metadata
	TTL          time.Duration
	SlidingTTL   time.Duration
}

// Register CacheItem for gob encoding
func init() {
	gob.Register(CacheItem{})
	gob.Register(map[string]int{})
	gob.Register([]int{})
}

// NewCacheItem creates a new cache item with a given TTL
func NewCacheItem(key string, value interface{}, ttl time.Duration) *CacheItem {
	var expiration time.Time
	if ttl > 0 {
		expiration = time.Now().Add(ttl)
	}

	return &CacheItem{
		Key:          key,
		Value:        value,
		Expiration:   expiration,
		LastAccessed: time.Now(),
		TTL:          ttl,
		SlidingTTL:   ttl,
	}
}

// IsExpired checks if the cache item is expired
func (item *CacheItem) IsExpired() bool {
	return !item.Expiration.IsZero() && time.Now().After(item.Expiration)
}

// Serialize encodes a CacheItem into a byte slice using Gob
func (item *CacheItem) Serialize() ([]byte, error) {
	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)
	err := enc.Encode(item)
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// Deserialize decodes a byte slice into a CacheItem
func Deserialize(data []byte) (*CacheItem, error) {
	buf := bytes.NewBuffer(data)
	dec := gob.NewDecoder(buf)
	var item CacheItem
	err := dec.Decode(&item)
	if err != nil {
		return nil, err
	}
	return &item, nil
}
