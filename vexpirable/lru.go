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

// Package vexpirable provides an LRU cache with TTL expiration and distributed invalidation.
package vexpirable

import (
	"context"
	"sync"
	"time"

	"github.com/hashicorp/golang-lru/v2/expirable"
	"github.com/vogo/vlru"
	"github.com/vogo/vlru/internal/caller"
	"github.com/vogo/vlru/internal/registry"
	"github.com/vogo/vlru/internal/uid"
	"github.com/vogo/vogo/vlog"
)

// LRU is a distributed expirable LRU cache.
// It wraps hashicorp/golang-lru/v2/expirable and adds distributed invalidation.
type LRU[K comparable, V any] struct {
	cache      *expirable.LRU[K, V]
	cacheName  string
	serializer vlru.KeySerializer[K]
	onEvict    func(K, V)

	// suppressKeys tracks keys being remotely invalidated to prevent cascade.
	// The value is a counter of how many concurrent InvalidateKey calls are
	// in progress for each key. This handles the case where multiple goroutines
	// call InvalidateKey for the same key concurrently.
	mu           sync.Mutex
	suppressKeys map[any]int
}

// LRUOption configures an LRU cache.
type LRUOption[K comparable, V any] func(*LRU[K, V])

// WithSerializer sets a custom key serializer.
func WithSerializer[K comparable, V any](s vlru.KeySerializer[K]) LRUOption[K, V] {
	return func(l *LRU[K, V]) {
		l.serializer = s
	}
}

// WithCacheName sets a custom cache name (overrides auto-generated name).
func WithCacheName[K comparable, V any](name string) LRUOption[K, V] {
	return func(l *LRU[K, V]) {
		l.cacheName = name
	}
}

// NewLRU creates a distributed expirable LRU with the same signature as expirable.NewLRU.
// The cache name is auto-generated from the call site (file:function:line).
func NewLRU[K comparable, V any](size int, onEvict func(K, V), ttl time.Duration, opts ...LRUOption[K, V]) *LRU[K, V] {
	l := &LRU[K, V]{
		cacheName:    caller.GetCallerName(1),
		onEvict:      onEvict,
		suppressKeys: make(map[any]int),
	}

	// Apply options
	for _, opt := range opts {
		opt(l)
	}

	// Create the underlying expirable LRU with our eviction wrapper
	l.cache = expirable.NewLRU[K, V](size, l.handleEviction, ttl)

	// Register with global registry
	registry.Register(l)

	return l
}

// handleEviction is called when a key is evicted from the underlying cache.
func (l *LRU[K, V]) handleEviction(key K, value V) {
	// Call user's eviction callback first
	if l.onEvict != nil {
		l.onEvict(key, value)
	}

	// Check if this specific key is being remotely invalidated
	l.mu.Lock()
	suppress := l.suppressKeys[key] > 0
	l.mu.Unlock()

	if suppress {
		return
	}

	// Publish invalidation event
	l.publishInvalidation(key)
}

// publishInvalidation sends an invalidation event for the given key.
func (l *LRU[K, V]) publishInvalidation(key K) {
	broker := vlru.GetBroker()
	if broker == nil {
		return
	}

	serializedKey, err := l.serializeKey(key)
	if err != nil {
		vlog.Errorf("error serializing | cache: %s | key %v | err: %v", l.cacheName, key, err)
		return
	}

	event := vlru.NewInvalidationEvent(l.cacheName, uid.Instance, serializedKey)
	if err := broker.Publish(context.Background(), event); err != nil {
		vlog.Errorf("vlru error publishing event | cache: %s | instance: %s | key: %s | err: %v", event.CacheName, event.Instance, event.Key, err)
	}
}

// serializeKey converts a key to its string representation.
func (l *LRU[K, V]) serializeKey(key K) (string, error) {
	if l.serializer != nil {
		return l.serializer.Serialize(key)
	}
	return vlru.DefaultSerialize(key)
}

// deserializeKey converts a string back to a key.
func (l *LRU[K, V]) deserializeKey(s string) (K, error) {
	if l.serializer != nil {
		return l.serializer.Deserialize(s)
	}
	return vlru.DefaultDeserialize[K](s)
}

// InvalidateKey removes a key without publishing an event (for remote invalidation).
func (l *LRU[K, V]) InvalidateKey(serializedKey string) error {
	key, err := l.deserializeKey(serializedKey)
	if err != nil {
		return err
	}

	l.mu.Lock()
	l.suppressKeys[key]++
	l.mu.Unlock()

	l.cache.Remove(key)

	l.mu.Lock()
	l.suppressKeys[key]--
	if l.suppressKeys[key] == 0 {
		delete(l.suppressKeys, key)
	}
	l.mu.Unlock()

	return nil
}

// CacheName returns the name of this cache.
func (l *LRU[K, V]) CacheName() string {
	return l.cacheName
}

// Close unregisters the cache and releases resources.
func (l *LRU[K, V]) Close() error {
	registry.Unregister(l)
	return nil
}

// Add adds a value to the cache. Returns true if an eviction occurred.
func (l *LRU[K, V]) Add(key K, value V) bool {
	return l.cache.Add(key, value)
}

// Get looks up a key's value from the cache.
func (l *LRU[K, V]) Get(key K) (value V, ok bool) {
	return l.cache.Get(key)
}

// Contains checks if a key is in the cache without updating the recent-ness.
func (l *LRU[K, V]) Contains(key K) bool {
	return l.cache.Contains(key)
}

// Peek returns the key value without updating the recent-ness.
func (l *LRU[K, V]) Peek(key K) (value V, ok bool) {
	return l.cache.Peek(key)
}

// Remove removes the provided key from the cache.
// The eviction callback will publish an invalidation event.
func (l *LRU[K, V]) Remove(key K) bool {
	return l.cache.Remove(key)
}

// RemoveOldest removes the oldest item from the cache.
func (l *LRU[K, V]) RemoveOldest() (key K, value V, ok bool) {
	return l.cache.RemoveOldest()
}

// GetOldest returns the oldest entry.
func (l *LRU[K, V]) GetOldest() (key K, value V, ok bool) {
	return l.cache.GetOldest()
}

// Keys returns a slice of the keys in the cache, from oldest to newest.
func (l *LRU[K, V]) Keys() []K {
	return l.cache.Keys()
}

// Values returns a slice of the values in the cache, from oldest to newest.
func (l *LRU[K, V]) Values() []V {
	return l.cache.Values()
}

// Len returns the number of items in the cache.
func (l *LRU[K, V]) Len() int {
	return l.cache.Len()
}

// Resize changes the cache size.
func (l *LRU[K, V]) Resize(size int) int {
	return l.cache.Resize(size)
}

// Purge clears all cache entries.
func (l *LRU[K, V]) Purge() {
	l.cache.Purge()
}
