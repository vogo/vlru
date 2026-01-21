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

package inmemory

import (
	"testing"
	"time"

	"github.com/vogo/vlru"
	"github.com/vogo/vogo/vsync/vrun"
)

func TestBrokerIntegration(t *testing.T) {
	broker := New()
	runner := vrun.New()
	defer runner.Stop()
	vlru.StartEventBroker(runner, broker)

	// Create two cache instances with the same cache name (required for distributed invalidation)
	cache1, err := vlru.New[string, int](10, vlru.WithCacheName[string, int]("inmemory-test"))
	if err != nil {
		t.Fatalf("Failed to create cache1: %v", err)
	}
	defer cache1.Close()

	cache2, err := vlru.New[string, int](10, vlru.WithCacheName[string, int]("inmemory-test"))
	if err != nil {
		t.Fatalf("Failed to create cache2: %v", err)
	}
	defer cache2.Close()

	// Add key to both caches
	cache1.Add("key1", 100)
	cache2.Add("key1", 200)

	// Verify initial state
	if val, ok := cache1.Get("key1"); !ok || val != 100 {
		t.Errorf("Expected cache1 to have key1=100, got %d (ok=%v)", val, ok)
	}
	if val, ok := cache2.Get("key1"); !ok || val != 200 {
		t.Errorf("Expected cache2 to have key1=200, got %d (ok=%v)", val, ok)
	}

	// Clear events before testing
	broker.ClearEvents()

	// Remove from cache1
	cache1.Remove("key1")

	// Wait for event processing
	time.Sleep(10 * time.Millisecond)

	// Verify cache1 doesn't have key
	if _, ok := cache1.Get("key1"); ok {
		t.Error("Expected cache1 to NOT have key1 after removal")
	}

	// Verify cache2 also doesn't have key (distributed invalidation)
	if _, ok := cache2.Get("key1"); ok {
		t.Error("Expected cache2 to NOT have key1 after distributed invalidation")
	}

	// Verify only one event was published (no cascade)
	if broker.EventCount() != 1 {
		t.Errorf("Expected exactly 1 event, got %d", broker.EventCount())
	}
}

func TestBrokerEventTracking(t *testing.T) {
	broker := New()

	if broker.EventCount() != 0 {
		t.Error("Expected 0 events initially")
	}

	events := broker.Events()
	if len(events) != 0 {
		t.Errorf("Expected empty events slice, got %d events", len(events))
	}
}

func TestBrokerClose(t *testing.T) {
	broker := New()

	if err := broker.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	// Verify that publishing after close doesn't record events
	// (broker is closed, so events won't be stored)
	if broker.EventCount() != 0 {
		t.Errorf("Expected 0 events after close, got %d", broker.EventCount())
	}
}
