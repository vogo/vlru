/*
 * Licensed to the Apache Software Foundation (ASF) under one or more
 * contributor license agreements.  See the NOTICE file distributed with
 * this work for additional information regarding copyright ownership.
 * The ASF licenses this file to You under the Apache License, Version 2.0
 * (the "License"); you may not use this file except in compliance with
 * the License.  You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package vlru

import (
	"context"
	"sync"

	lru "github.com/hashicorp/golang-lru/v2"
	"github.com/vogo/vlru/internal/caller"
	"github.com/vogo/vlru/internal/registry"
	"github.com/vogo/vlru/internal/uid"
	"github.com/vogo/vogo/vlog"
)

// Cache is a distributed LRU cache that wraps hashicorp/golang-lru.
// When keys are evicted or removed, it publishes invalidation events
// to other instances via the configured Broker.
type Cache[K comparable, V any] struct {
	cache      *lru.Cache[K, V]
	cacheName  string
	serializer KeySerializer[K]
	onEvict    func(K, V)

	// suppressKeys tracks keys being remotely invalidated to prevent cascade.
	// The value is a counter of how many concurrent InvalidateKey calls are
	// in progress for each key. This handles the case where multiple goroutines
	// call InvalidateKey for the same key concurrently.
	mu           sync.Mutex
	suppressKeys map[any]int
}

// CacheOption configures a Cache.
type CacheOption[K comparable, V any] func(*Cache[K, V])

// WithSerializer sets a custom key serializer.
func WithSerializer[K comparable, V any](s KeySerializer[K]) CacheOption[K, V] {
	return func(c *Cache[K, V]) {
		c.serializer = s
	}
}

// WithCacheName sets a custom cache name (overrides auto-generated name).
func WithCacheName[K comparable, V any](name string) CacheOption[K, V] {
	return func(c *Cache[K, V]) {
		c.cacheName = name
	}
}

// New creates a distributed LRU cache with the same signature as lru.New.
// The cache name is auto-generated from the call site (file:function:line).
func New[K comparable, V any](size int, opts ...CacheOption[K, V]) (*Cache[K, V], error) {
	return newWithEvict(size, nil, caller.GetCallerName(1), opts...)
}

// NewWithEvict creates a cache with an eviction callback.
// The cache name is auto-generated from the call site (file:function:line).
func NewWithEvict[K comparable, V any](size int, onEvict func(K, V), opts ...CacheOption[K, V]) (*Cache[K, V], error) {
	return newWithEvict(size, onEvict, caller.GetCallerName(1), opts...)
}

// newWithEvict is the internal constructor that accepts a pre-computed cache name.
func newWithEvict[K comparable, V any](size int, onEvict func(K, V), autoName string, opts ...CacheOption[K, V]) (*Cache[K, V], error) {
	c := &Cache[K, V]{
		cacheName:    autoName,
		onEvict:      onEvict,
		suppressKeys: make(map[any]int),
	}

	// Apply options
	for _, opt := range opts {
		opt(c)
	}

	// Create the underlying LRU with our eviction wrapper
	cache, err := lru.NewWithEvict(size, c.handleEviction)
	if err != nil {
		return nil, err
	}
	c.cache = cache

	// Register with global registry
	registry.Register(c)

	return c, nil
}

// handleEviction is called when a key is evicted from the underlying cache.
func (c *Cache[K, V]) handleEviction(key K, value V) {
	// Call user's eviction callback first
	if c.onEvict != nil {
		c.onEvict(key, value)
	}

	// Check if this specific key is being remotely invalidated
	c.mu.Lock()
	suppress := c.suppressKeys[key] > 0
	c.mu.Unlock()

	if suppress {
		return
	}

	// Publish invalidation event
	c.publishInvalidation(key)
}

// publishInvalidation sends an invalidation event for the given key.
func (c *Cache[K, V]) publishInvalidation(key K) {
	broker := GetBroker()
	if broker == nil {
		return
	}

	serializedKey, err := c.serializeKey(key)
	if err != nil {
		return
	}

	event := NewInvalidationEvent(c.cacheName, uid.InstanceID, serializedKey)
	if err := broker.Publish(context.Background(), event); err != nil {
		vlog.Errorf("vlru error publishing event | cache: %s | instance: %s | key: %s | err: %v", event.CacheName, event.InstanceID, event.Key, err)
	}
}

// serializeKey converts a key to its string representation.
func (c *Cache[K, V]) serializeKey(key K) (string, error) {
	if c.serializer != nil {
		return c.serializer.Serialize(key)
	}
	return DefaultSerialize(key)
}

// deserializeKey converts a string back to a key.
func (c *Cache[K, V]) deserializeKey(s string) (K, error) {
	if c.serializer != nil {
		return c.serializer.Deserialize(s)
	}
	return DefaultDeserialize[K](s)
}

// InvalidateKey removes a key without publishing an event (for remote invalidation).
func (c *Cache[K, V]) InvalidateKey(serializedKey string) error {
	key, err := c.deserializeKey(serializedKey)
	if err != nil {
		return err
	}

	c.mu.Lock()
	c.suppressKeys[key]++
	c.mu.Unlock()

	c.cache.Remove(key)

	c.mu.Lock()
	c.suppressKeys[key]--
	if c.suppressKeys[key] == 0 {
		delete(c.suppressKeys, key)
	}
	c.mu.Unlock()

	return nil
}

// InstanceID returns the unique identifier for this cache instance.
func (c *Cache[K, V]) InstanceID() string {
	return uid.InstanceID
}

// CacheName returns the name of this cache.
func (c *Cache[K, V]) CacheName() string {
	return c.cacheName
}

// Close unregisters the cache and releases resources.
func (c *Cache[K, V]) Close() error {
	registry.Unregister(c)
	return nil
}

// Add adds a value to the cache. Returns true if an eviction occurred.
func (c *Cache[K, V]) Add(key K, value V) bool {
	return c.cache.Add(key, value)
}

// Get looks up a key's value from the cache.
func (c *Cache[K, V]) Get(key K) (value V, ok bool) {
	return c.cache.Get(key)
}

// Contains checks if a key is in the cache without updating the recent-ness.
func (c *Cache[K, V]) Contains(key K) bool {
	return c.cache.Contains(key)
}

// Peek returns the key value without updating the recent-ness.
func (c *Cache[K, V]) Peek(key K) (value V, ok bool) {
	return c.cache.Peek(key)
}

// Remove removes the provided key from the cache.
// The eviction callback will publish an invalidation event.
func (c *Cache[K, V]) Remove(key K) bool {
	return c.cache.Remove(key)
}

// RemoveOldest removes the oldest item from the cache.
func (c *Cache[K, V]) RemoveOldest() (key K, value V, ok bool) {
	return c.cache.RemoveOldest()
}

// GetOldest returns the oldest entry.
func (c *Cache[K, V]) GetOldest() (key K, value V, ok bool) {
	return c.cache.GetOldest()
}

// Keys returns a slice of the keys in the cache, from oldest to newest.
func (c *Cache[K, V]) Keys() []K {
	return c.cache.Keys()
}

// Values returns a slice of the values in the cache, from oldest to newest.
func (c *Cache[K, V]) Values() []V {
	return c.cache.Values()
}

// Len returns the number of items in the cache.
func (c *Cache[K, V]) Len() int {
	return c.cache.Len()
}

// Resize changes the cache size.
func (c *Cache[K, V]) Resize(size int) int {
	return c.cache.Resize(size)
}

// Purge clears all cache entries.
func (c *Cache[K, V]) Purge() {
	c.cache.Purge()
}
