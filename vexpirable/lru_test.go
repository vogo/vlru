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

package vexpirable

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/vogo/vlru"
	"github.com/vogo/vogo/vsync/vrun"
)

// testBroker is a simple broker for testing that records events.
type testBroker struct {
	mu     sync.Mutex
	events []vlru.InvalidationEvent
	ch     chan *vlru.InvalidationEvent
	closed bool
}

func newTestBroker() *testBroker {
	return &testBroker{
		events: make([]vlru.InvalidationEvent, 0),
		ch:     make(chan *vlru.InvalidationEvent, 100),
	}
}

func (b *testBroker) Publish(_ context.Context, event *vlru.InvalidationEvent) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.closed {
		return nil
	}

	b.events = append(b.events, *event)

	// Route to channel with a fake remote instance ID to simulate distributed invalidation.
	// In real scenarios, events come from different processes with different InstanceIDs.
	remoteEvent := &vlru.InvalidationEvent{
		CacheName:  event.CacheName,
		InstanceID: "remote-instance",
		Key:        event.Key,
		Timestamp:  event.Timestamp,
	}
	select {
	case b.ch <- remoteEvent:
	default:
	}

	return nil
}

func (b *testBroker) Channel() <-chan *vlru.InvalidationEvent {
	return b.ch
}

func (b *testBroker) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if !b.closed {
		b.closed = true
		close(b.ch)
	}
	return nil
}

func (b *testBroker) EventCount() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.events)
}

func (b *testBroker) ClearEvents() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.events = b.events[:0]
}

func TestLRUBasicOperations(t *testing.T) {
	cache := NewLRU[string, int](10, nil, time.Hour)
	defer cache.Close()

	// Add
	cache.Add("key1", 1)
	cache.Add("key2", 2)

	// Get
	val, ok := cache.Get("key1")
	if !ok || val != 1 {
		t.Errorf("Expected value 1, got %d (ok=%v)", val, ok)
	}

	// Contains
	if !cache.Contains("key2") {
		t.Error("Expected cache to contain key2")
	}

	// Peek
	val, ok = cache.Peek("key2")
	if !ok || val != 2 {
		t.Errorf("Expected value 2, got %d (ok=%v)", val, ok)
	}

	// Len
	if cache.Len() != 2 {
		t.Errorf("Expected length 2, got %d", cache.Len())
	}

	// Remove
	present := cache.Remove("key1")
	if !present {
		t.Error("Expected Remove to return true for existing key")
	}
	if cache.Contains("key1") {
		t.Error("Expected key1 to be removed")
	}
}

func TestLRUExpiration(t *testing.T) {
	cache := NewLRU[string, int](10, nil, 50*time.Millisecond)
	defer cache.Close()

	cache.Add("key1", 1)

	// Key should exist initially
	if _, ok := cache.Get("key1"); !ok {
		t.Error("Expected key to exist initially")
	}

	// Wait for expiration
	time.Sleep(100 * time.Millisecond)

	// Key should be expired
	if _, ok := cache.Get("key1"); ok {
		t.Error("Expected key to be expired")
	}
}

func TestLRUEvictionCallback(t *testing.T) {
	var evictedKey string
	var evictedVal int

	cache := NewLRU[string, int](2, func(k string, v int) {
		evictedKey = k
		evictedVal = v
	}, time.Hour)
	defer cache.Close()

	cache.Add("key1", 1)
	cache.Add("key2", 2)
	cache.Add("key3", 3) // This should evict key1

	if evictedKey != "key1" || evictedVal != 1 {
		t.Errorf("Expected eviction of key1=1, got %s=%d", evictedKey, evictedVal)
	}
}

func TestLRUDistributedInvalidation(t *testing.T) {
	broker := newTestBroker()
	runner := vrun.New()
	defer runner.Stop()
	vlru.StartEventBroker(runner, broker)

	// Create a cache instance
	cache := NewLRU[string, int](10, nil, time.Hour, WithCacheName[string, int]("expirable-dist-test"))
	defer cache.Close()

	// Add key to cache
	cache.Add("shared-key", 100)

	broker.ClearEvents()

	// Remove from cache - should publish event
	cache.Remove("shared-key")

	// Wait for event processing
	time.Sleep(10 * time.Millisecond)

	// cache should not have the key
	if _, ok := cache.Get("shared-key"); ok {
		t.Error("Expected cache to NOT have shared-key after removal")
	}

	// 1 event should have been published
	if broker.EventCount() != 1 {
		t.Errorf("Expected exactly 1 event, got %d", broker.EventCount())
	}
}

func TestLRUTTLExpirationPublishesEvent(t *testing.T) {
	broker := newTestBroker()
	runner := vrun.New()
	defer runner.Stop()
	vlru.StartEventBroker(runner, broker)

	cache := NewLRU[string, int](10, nil, 50*time.Millisecond)
	defer cache.Close()

	cache.Add("expiring-key", 1)

	// Wait for expiration
	time.Sleep(100 * time.Millisecond)

	// Access to trigger cleanup
	cache.Get("expiring-key")

	// An event should have been published for TTL expiration
	if broker.EventCount() < 1 {
		t.Errorf("Expected at least 1 event for TTL expiration, got %d", broker.EventCount())
	}
}

func TestLRUWithCustomCacheName(t *testing.T) {
	// Test that WithCacheName overrides auto-generated name
	cache1 := NewLRU[string, int](10, nil, time.Hour)
	cache2 := NewLRU[string, int](10, nil, time.Hour, WithCacheName[string, int]("custom-expirable"))
	defer cache1.Close()
	defer cache2.Close()

	// cache1 should have auto-generated name (file:func:line)
	if cache1.CacheName() == "" {
		t.Error("Expected non-empty auto-generated cache name")
	}
	// cache2 should have custom name
	if cache2.CacheName() != "custom-expirable" {
		t.Errorf("Expected custom-expirable, got %s", cache2.CacheName())
	}
}

func TestLRUAutoNaming(t *testing.T) {
	// Two caches created at different lines should have different names
	cache1 := NewLRU[string, int](10, nil, time.Hour)
	cache2 := NewLRU[string, int](10, nil, time.Hour)
	defer cache1.Close()
	defer cache2.Close()

	// They should have different names since they're on different lines
	if cache1.CacheName() == cache2.CacheName() {
		t.Errorf("Expected different auto-generated names, both got %s", cache1.CacheName())
	}

	// Names should contain file and function info
	name := cache1.CacheName()
	if name == "" {
		t.Error("Expected non-empty cache name")
	}
}

func TestLRUPurge(t *testing.T) {
	cache := NewLRU[string, int](10, nil, time.Hour)
	defer cache.Close()

	cache.Add("key1", 1)
	cache.Add("key2", 2)

	if cache.Len() != 2 {
		t.Errorf("Expected length 2, got %d", cache.Len())
	}

	cache.Purge()

	if cache.Len() != 0 {
		t.Errorf("Expected length 0 after purge, got %d", cache.Len())
	}
}
