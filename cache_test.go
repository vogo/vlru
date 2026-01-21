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
	"sync/atomic"
	"testing"
	"time"

	"github.com/vogo/vlru/internal/registry"
	"github.com/vogo/vogo/vsync/vrun"
)

// testBroker is a simple broker for testing that records events.
type testBroker struct {
	mu     sync.Mutex
	events []InvalidationEvent
	ch     chan *InvalidationEvent
	closed bool
}

func newTestBroker() *testBroker {
	return &testBroker{
		events: make([]InvalidationEvent, 0),
		ch:     make(chan *InvalidationEvent, 100),
	}
}

func (b *testBroker) Publish(_ context.Context, event InvalidationEvent) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.events = append(b.events, event)
	// Route to registry with a fake remote instance ID to simulate distributed invalidation.
	// In real scenarios, events come from different processes with different InstanceIDs.
	return registry.HandleEvent(event.CacheName, "remote-instance", event.Key)
}

func (b *testBroker) Channel() <-chan *InvalidationEvent {
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

func TestCacheBasicOperations(t *testing.T) {
	cache, err := New[string, int](10)
	if err != nil {
		t.Fatalf("Failed to create cache: %v", err)
	}
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

	// Keys
	keys := cache.Keys()
	if len(keys) != 2 {
		t.Errorf("Expected 2 keys, got %d", len(keys))
	}

	// Values
	values := cache.Values()
	if len(values) != 2 {
		t.Errorf("Expected 2 values, got %d", len(values))
	}

	// Remove
	present := cache.Remove("key1")
	if !present {
		t.Error("Expected Remove to return true for existing key")
	}
	if cache.Contains("key1") {
		t.Error("Expected key1 to be removed")
	}

	// Purge
	cache.Purge()
	if cache.Len() != 0 {
		t.Errorf("Expected empty cache after Purge, got length %d", cache.Len())
	}
}

func TestCacheEviction(t *testing.T) {
	var evictedKey string
	var evictedVal int

	cache, err := NewWithEvict[string, int](2, func(k string, v int) {
		evictedKey = k
		evictedVal = v
	})
	if err != nil {
		t.Fatalf("Failed to create cache: %v", err)
	}
	defer cache.Close()

	cache.Add("key1", 1)
	cache.Add("key2", 2)
	cache.Add("key3", 3) // This should evict key1

	if evictedKey != "key1" || evictedVal != 1 {
		t.Errorf("Expected eviction of key1=1, got %s=%d", evictedKey, evictedVal)
	}
}

func TestCacheEventPublishing(t *testing.T) {
	broker := newTestBroker()
	runner := vrun.New()
	defer runner.Stop()
	StartEventBroker(runner, broker)

	cache, err := New[string, int](10)
	if err != nil {
		t.Fatalf("Failed to create cache: %v", err)
	}
	defer cache.Close()

	// Add should not publish
	cache.Add("key1", 1)
	if broker.EventCount() != 0 {
		t.Errorf("Expected 0 events after Add, got %d", broker.EventCount())
	}

	// Remove should publish
	cache.Remove("key1")
	if broker.EventCount() != 1 {
		t.Errorf("Expected 1 event after Remove, got %d", broker.EventCount())
	}
}

func TestCacheDistributedInvalidation(t *testing.T) {
	broker := newTestBroker()
	runner := vrun.New()
	defer runner.Stop()
	StartEventBroker(runner, broker)

	// Create a cache instance
	cache, err := New[string, int](10, WithCacheName[string, int]("dist-test"))
	if err != nil {
		t.Fatalf("Failed to create cache: %v", err)
	}
	defer cache.Close()

	// Add key to cache
	cache.Add("shared-key", 100)

	// Verify cache has the key
	if _, ok := cache.Get("shared-key"); !ok {
		t.Error("Expected cache to have shared-key")
	}

	broker.ClearEvents()

	// Remove from cache - should publish event
	cache.Remove("shared-key")

	// Wait a bit for event processing
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

func TestCacheCapacityEvictionPublishes(t *testing.T) {
	broker := newTestBroker()
	runner := vrun.New()
	defer runner.Stop()
	StartEventBroker(runner, broker)

	cache, err := New[string, int](2)
	if err != nil {
		t.Fatalf("Failed to create cache: %v", err)
	}
	defer cache.Close()

	cache.Add("key1", 1)
	cache.Add("key2", 2)

	if broker.EventCount() != 0 {
		t.Errorf("Expected 0 events before capacity eviction, got %d", broker.EventCount())
	}

	// This should evict key1 and publish event
	cache.Add("key3", 3)

	if broker.EventCount() != 1 {
		t.Errorf("Expected 1 event after capacity eviction, got %d", broker.EventCount())
	}
}

func TestCacheWithCustomSerializer(t *testing.T) {
	broker := newTestBroker()
	runner := vrun.New()
	defer runner.Stop()
	StartEventBroker(runner, broker)

	cache, err := New[int, string](10, WithSerializer[int, string](JSONKeySerializer[int]{}))
	if err != nil {
		t.Fatalf("Failed to create cache: %v", err)
	}
	defer cache.Close()

	cache.Add(42, "value")
	cache.Remove(42)

	if broker.EventCount() != 1 {
		t.Errorf("Expected 1 event, got %d", broker.EventCount())
	}
}

func TestCacheWithCustomCacheName(t *testing.T) {
	// Test that WithCacheName overrides auto-generated name
	cache1, _ := New[string, int](10)
	cache2, _ := New[string, int](10, WithCacheName[string, int]("custom-name"))
	defer cache1.Close()
	defer cache2.Close()

	// cache1 should have auto-generated name (file:func:line)
	if cache1.CacheName() == "" {
		t.Error("Expected non-empty auto-generated cache name")
	}
	// cache2 should have custom name
	if cache2.CacheName() != "custom-name" {
		t.Errorf("Expected custom-name, got %s", cache2.CacheName())
	}
}

func TestCacheAutoNaming(t *testing.T) {
	// Two caches created at different lines should have different names
	cache1, _ := New[string, int](10)
	cache2, _ := New[string, int](10)
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

func TestCacheAutoNamingSameLineLoop(t *testing.T) {
	// Caches created at the same line (in a loop) should have the same name
	caches := make([]*Cache[string, int], 3)
	for i := range 3 {
		caches[i], _ = New[string, int](10)
	}
	defer func() {
		for _, c := range caches {
			c.Close()
		}
	}()

	// All caches from the loop should have the same name
	for i := 1; i < 3; i++ {
		if caches[i].CacheName() != caches[0].CacheName() {
			t.Errorf("Expected same name for same call site, got %s vs %s", caches[0].CacheName(), caches[i].CacheName())
		}
	}
}

func TestCacheResize(t *testing.T) {
	cache, err := New[string, int](10)
	if err != nil {
		t.Fatalf("Failed to create cache: %v", err)
	}
	defer cache.Close()

	for i := 0; i < 10; i++ {
		cache.Add(string(rune('a'+i)), i)
	}

	if cache.Len() != 10 {
		t.Errorf("Expected length 10, got %d", cache.Len())
	}

	evicted := cache.Resize(5)
	if evicted != 5 {
		t.Errorf("Expected 5 evicted, got %d", evicted)
	}
	if cache.Len() != 5 {
		t.Errorf("Expected length 5 after resize, got %d", cache.Len())
	}
}

func TestCacheConcurrentAccess(t *testing.T) {
	cache, err := New[int, int](1000)
	if err != nil {
		t.Fatalf("Failed to create cache: %v", err)
	}
	defer cache.Close()

	var wg sync.WaitGroup
	var ops int64

	// Concurrent writes
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(base int) {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				cache.Add(base*100+j, j)
				atomic.AddInt64(&ops, 1)
			}
		}(i)
	}

	// Concurrent reads
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(base int) {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				cache.Get(base*100 + j)
				atomic.AddInt64(&ops, 1)
			}
		}(i)
	}

	wg.Wait()

	if atomic.LoadInt64(&ops) != 2000 {
		t.Errorf("Expected 2000 ops, got %d", ops)
	}
}

// TestCacheConcurrentInvalidateKey verifies that concurrent InvalidateKey calls
// don't cause race conditions with the per-key suppression map.
// This test ensures:
// 1. Remote invalidations (InvalidateKey) don't publish events
// 2. Local removes (Remove) still publish events correctly
// 3. Concurrent operations on different keys don't interfere with each other
func TestCacheConcurrentInvalidateKey(t *testing.T) {
	broker := newTestBroker()
	runner := vrun.New()
	defer runner.Stop()
	StartEventBroker(runner, broker)

	cache, err := New[string, int](1000)
	if err != nil {
		t.Fatalf("Failed to create cache: %v", err)
	}
	defer cache.Close()

	const numKeys = 100
	const numGoroutines = 10

	// Add keys that will be remotely invalidated
	for i := 0; i < numKeys; i++ {
		cache.Add("remote-"+string(rune('a'+i)), i)
	}
	// Add keys that will be locally removed
	for i := 0; i < numKeys; i++ {
		cache.Add("local-"+string(rune('a'+i)), i)
	}

	broker.ClearEvents()

	var wg sync.WaitGroup

	// Launch goroutines that call InvalidateKey concurrently (simulating remote invalidation)
	// These should NOT publish any events
	for g := 0; g < numGoroutines; g++ {
		wg.Add(1)
		go func(goroutineID int) {
			defer wg.Done()
			for i := goroutineID; i < numKeys; i += numGoroutines {
				key := "remote-" + string(rune('a'+i))
				if err := cache.InvalidateKey(key); err != nil {
					t.Errorf("InvalidateKey failed: %v", err)
				}
			}
		}(g)
	}

	// Launch goroutines that call Remove concurrently (local removal)
	// These SHOULD publish events
	for g := 0; g < numGoroutines; g++ {
		wg.Add(1)
		go func(goroutineID int) {
			defer wg.Done()
			for i := goroutineID; i < numKeys; i += numGoroutines {
				key := "local-" + string(rune('a'+i))
				cache.Remove(key)
			}
		}(g)
	}

	wg.Wait()

	// Wait a bit for any async event processing
	time.Sleep(10 * time.Millisecond)

	// Only local removes should have published events (numKeys events)
	// Remote invalidations should NOT have published any events
	eventCount := broker.EventCount()
	if eventCount != numKeys {
		t.Errorf("Expected exactly %d events (only from local removes), got %d", numKeys, eventCount)
	}

	// Verify all keys were removed
	for i := 0; i < numKeys; i++ {
		if cache.Contains("remote-" + string(rune('a'+i))) {
			t.Errorf("Expected remote key %d to be removed", i)
		}
		if cache.Contains("local-" + string(rune('a'+i))) {
			t.Errorf("Expected local key %d to be removed", i)
		}
	}
}

// TestCacheConcurrentInvalidateKeySameKey verifies that multiple goroutines
// calling InvalidateKey for the same key concurrently don't cause issues.
func TestCacheConcurrentInvalidateKeySameKey(t *testing.T) {
	broker := newTestBroker()
	runner := vrun.New()
	defer runner.Stop()
	StartEventBroker(runner, broker)

	cache, err := New[string, int](10)
	if err != nil {
		t.Fatalf("Failed to create cache: %v", err)
	}
	defer cache.Close()

	const numGoroutines = 50
	const iterations = 100

	for iter := 0; iter < iterations; iter++ {
		// Add the key back
		cache.Add("contested-key", iter)
		broker.ClearEvents()

		var wg sync.WaitGroup

		// Multiple goroutines try to invalidate the same key concurrently
		for g := 0; g < numGoroutines; g++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_ = cache.InvalidateKey("contested-key")
			}()
		}

		wg.Wait()

		// No events should be published (all were remote invalidations)
		if broker.EventCount() != 0 {
			t.Errorf("Iteration %d: Expected 0 events from InvalidateKey, got %d", iter, broker.EventCount())
		}
	}
}

// TestCacheMixedConcurrentOperations tests a mix of Add, Remove, and InvalidateKey
// operations happening concurrently to ensure the suppression map handles all cases.
func TestCacheMixedConcurrentOperations(t *testing.T) {
	broker := newTestBroker()
	runner := vrun.New()
	defer runner.Stop()
	StartEventBroker(runner, broker)

	cache, err := New[int, int](1000)
	if err != nil {
		t.Fatalf("Failed to create cache: %v", err)
	}
	defer cache.Close()

	const numOps = 1000
	var wg sync.WaitGroup
	var localRemoves int64

	// Writer goroutine - continuously adds keys
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < numOps; i++ {
			cache.Add(i%100, i)
		}
	}()

	// Local remover goroutine - removes keys (should publish)
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < numOps; i++ {
			if cache.Remove(i % 100) {
				atomic.AddInt64(&localRemoves, 1)
			}
		}
	}()

	// Remote invalidator goroutines - call InvalidateKey (should NOT publish)
	for g := 0; g < 5; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < numOps/5; i++ {
				_ = cache.InvalidateKey(string(rune('0' + i%10)))
			}
		}()
	}

	wg.Wait()

	// Wait for any async processing
	time.Sleep(10 * time.Millisecond)

	// The number of events should be close to the number of successful local removes
	// (InvalidateKey calls should not contribute to event count)
	// Due to concurrent racing between Add and Remove on the same keys, there may be
	// small discrepancies, so we allow some tolerance.
	eventCount := broker.EventCount()
	removes := atomic.LoadInt64(&localRemoves)

	// Events should be close to local removes (allow 5% tolerance for timing issues)
	tolerance := int(removes) / 20
	if tolerance < 10 {
		tolerance = 10
	}
	diff := eventCount - int(removes)
	if diff < 0 {
		diff = -diff
	}
	if diff > tolerance {
		t.Errorf("Event count (%d) should be close to local removes (%d), diff=%d exceeds tolerance=%d", eventCount, removes, diff, tolerance)
	}
}
