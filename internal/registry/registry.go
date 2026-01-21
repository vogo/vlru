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

// Package registry manages cache registration and event routing for distributed invalidation.
package registry

import (
	"sync"
)

// CacheInvalidator is implemented by caches that can receive invalidation events.
type CacheInvalidator interface {
	// InvalidateKey removes a key from the cache without publishing an event.
	// The serializedKey is the string representation of the key.
	InvalidateKey(serializedKey string) error

	// InstanceID returns the unique identifier for this cache instance.
	InstanceID() string

	// CacheName returns the name of this cache.
	CacheName() string
}

// registry manages cache registration and event routing.
type registry struct {
	mu     sync.RWMutex
	caches map[string]CacheInvalidator // cacheName -> cache
}

var globalRegistry = &registry{
	caches: make(map[string]CacheInvalidator),
}

// Register adds a cache to the global registry for event routing.
func Register(cache CacheInvalidator) {
	globalRegistry.register(cache)
}

// Unregister removes a cache from the global registry.
func Unregister(cache CacheInvalidator) {
	globalRegistry.unregister(cache)
}

// HandleEvent routes an invalidation event to the appropriate cache.
// It filters out events from the same instance to prevent cascading.
func HandleEvent(cacheName, instanceID, key string) error {
	return globalRegistry.handleEvent(cacheName, instanceID, key)
}

// register adds a cache to the registry for event routing.
func (r *registry) register(cache CacheInvalidator) {
	r.mu.Lock()
	defer r.mu.Unlock()

	name := cache.CacheName()
	r.caches[name] = cache
}

// unregister removes a cache from the registry.
func (r *registry) unregister(cache CacheInvalidator) {
	r.mu.Lock()
	defer r.mu.Unlock()

	name := cache.CacheName()
	delete(r.caches, name)
}

// handleEvent routes an invalidation event to the appropriate cache.
func (r *registry) handleEvent(cacheName, instanceID, key string) error {
	r.mu.RLock()
	cache, ok := r.caches[cacheName]
	r.mu.RUnlock()

	if !ok {
		return nil
	}

	// Skip if this event came from the same instance (prevent cascade)
	if cache.InstanceID() == instanceID {
		return nil
	}

	// Invalidate the key locally without publishing
	return cache.InvalidateKey(key)
}
