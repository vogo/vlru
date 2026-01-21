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

// Package inmemory provides an in-memory broker for testing distributed cache invalidation.
package inmemory

import (
	"context"
	"sync"

	"github.com/vogo/vlru"
)

// Broker is an in-memory broker for testing.
// It routes events directly to the global registry without network transport.
type Broker struct {
	mu     sync.RWMutex
	closed bool
	events []vlru.InvalidationEvent
	ch     chan vlru.InvalidationEvent
}

// New creates a new in-memory broker.
func New() *Broker {
	return &Broker{
		events: make([]vlru.InvalidationEvent, 0),
		ch:     make(chan vlru.InvalidationEvent, 100),
	}
}

// Publish sends an invalidation event to all listeners.
func (b *Broker) Publish(_ context.Context, event vlru.InvalidationEvent) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.closed {
		return nil
	}

	// Store event for inspection
	b.events = append(b.events, event)

	// Route to channel with a fake remote instance ID to simulate distributed invalidation.
	// In real scenarios, events come from different processes with different InstanceIDs.
	remoteEvent := vlru.InvalidationEvent{
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

// Channel returns a channel that receives invalidation events.
func (b *Broker) Channel() <-chan vlru.InvalidationEvent {
	return b.ch
}

// Close releases resources.
func (b *Broker) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if !b.closed {
		b.closed = true
		close(b.ch)
	}
	return nil
}

// Events returns all published events for testing inspection.
func (b *Broker) Events() []vlru.InvalidationEvent {
	b.mu.RLock()
	defer b.mu.RUnlock()
	result := make([]vlru.InvalidationEvent, len(b.events))
	copy(result, b.events)
	return result
}

// ClearEvents clears the event history.
func (b *Broker) ClearEvents() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.events = b.events[:0]
}

// EventCount returns the number of published events.
func (b *Broker) EventCount() int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return len(b.events)
}
