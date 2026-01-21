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

import "time"

// InvalidationEvent represents a cache key invalidation that should be
// propagated to other cache instances.
type InvalidationEvent struct {
	// CacheName identifies which cache this event belongs to
	CacheName string `json:"cache_name"`

	// InstanceID identifies the cache instance that originated this event
	InstanceID string `json:"instance_id"`

	// Key is the serialized cache key that was invalidated
	Key string `json:"key"`

	// Timestamp when the invalidation occurred
	Timestamp time.Time `json:"timestamp"`
}

// NewInvalidationEvent creates a new InvalidationEvent with the current timestamp.
func NewInvalidationEvent(cacheName, instanceID, key string) InvalidationEvent {
	return InvalidationEvent{
		CacheName:  cacheName,
		InstanceID: instanceID,
		Key:        key,
		Timestamp:  time.Now(),
	}
}
