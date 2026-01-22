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
	"github.com/vogo/vogo/vsync/vrun"
)

// Broker is an in-memory broker for testing.
// It routes events directly to the global registry without network transport.
type Broker struct {
	mu     sync.RWMutex
	closed bool
	events []vlru.InvalidationEvent

	receiveChan   chan *vlru.InvalidationEvent
	receiveRunner *vrun.Runner
}

// New creates a new in-memory broker.
func New() *Broker {
	return &Broker{
		events:      make([]vlru.InvalidationEvent, 0),
		receiveChan: make(chan *vlru.InvalidationEvent, 100),
	}
}

// Publish sends an invalidation event to all listeners.
func (b *Broker) Publish(_ context.Context, event *vlru.InvalidationEvent) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.closed {
		return nil
	}

	// Store event for inspection
	b.events = append(b.events, *event)

	// Route to channel with a fake remote instance ID to simulate distributed invalidation.
	// In real scenarios, events come from different processes with different Instances.
	remoteEvent := &vlru.InvalidationEvent{
		CacheName: event.CacheName,
		Instance:  "remote-instance",
		Key:       event.Key,
		Timestamp: event.Timestamp,
	}

	if b.receiveRunner != nil {
		select {
		case <-b.receiveRunner.C:
			return nil
		case b.receiveChan <- remoteEvent:
		default:
		}
	}

	return nil
}

// StartReceive starts the event receiver and returns the channel.
func (b *Broker) StartReceive(runner *vrun.Runner) <-chan *vlru.InvalidationEvent {
	b.receiveRunner = runner

	return b.receiveChan
}

// Close releases resources.
func (b *Broker) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if !b.closed {
		b.closed = true
		close(b.receiveChan)
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
