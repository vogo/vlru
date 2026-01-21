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
	"runtime/debug"
	"sync"

	"github.com/vogo/vlru/internal/registry"
	"github.com/vogo/vogo/vlog"
	"github.com/vogo/vogo/vsync/vrun"
)

// Broker handles both publishing and receiving invalidation events.
// Implementations should handle the network transport (e.g., Redis pub/sub, NATS, etc.)
type Broker interface {
	// Publish sends an invalidation event to other instances.
	// This is called when a local cache evicts or removes a key.
	Publish(ctx context.Context, event InvalidationEvent) error

	// Channel returns a channel that receives invalidation events from other instances.
	Channel() <-chan *InvalidationEvent

	// Close releases resources held by the broker.
	Close() error
}

// Global configuration
var (
	globalBroker     Broker
	globalBrokerLock sync.RWMutex
)

// GetBroker returns the global broker.
func GetBroker() Broker {
	globalBrokerLock.RLock()
	defer globalBrokerLock.RUnlock()
	return globalBroker
}

// StartEventBroker sets the global broker for all caches.
func StartEventBroker(runner *vrun.Runner, broker Broker) {
	globalBrokerLock.Lock()
	if globalBroker != nil {
		globalBroker.Close()
	}
	globalBroker = broker
	globalBrokerLock.Unlock()

	runner.Defer(func() {
		if err := broker.Close(); err != nil {
			vlog.Errorf("vlru error closing broker | err: %v", err)
		}
	})

	ch := broker.Channel()

	runner.Loop(func() {
		defer func() {
			if _err := recover(); _err != nil {
				vlog.Errorf("vlru event broker loop panic: %v | stack: %s", _err, debug.Stack())
			}
		}()

		select {
		case event, ok := <-ch:
			if !ok {
				vlog.Infof("vlru event broker channel closed")
				return
			}
			if event == nil {
				return
			}
			vlog.Debugf("vlru invalidation event | cache: %s | instance: %s | key: %s", event.CacheName, event.InstanceID, event.Key)
			if err := registry.HandleEvent(event.CacheName, event.InstanceID, event.Key); err != nil {
				vlog.Errorf("vlru error handling event | err: %v", err)
			}
		case <-runner.C:
			vlog.Infof("vlru event broker context done")
			return
		}
	})
}
