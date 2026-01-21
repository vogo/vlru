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
	"encoding/json"
	"fmt"
	"strconv"
)

// KeySerializer handles serialization and deserialization of cache keys.
type KeySerializer[K comparable] interface {
	// Serialize converts a key to its string representation
	Serialize(key K) (string, error)

	// Deserialize converts a string back to a key
	Deserialize(s string) (K, error)
}

// StringKeySerializer is a KeySerializer for string keys.
type StringKeySerializer struct{}

// Serialize returns the string as-is.
func (StringKeySerializer) Serialize(key string) (string, error) {
	return key, nil
}

// Deserialize returns the string as-is.
func (StringKeySerializer) Deserialize(s string) (string, error) {
	return s, nil
}

// JSONKeySerializer uses JSON encoding for arbitrary key types.
type JSONKeySerializer[K comparable] struct{}

// Serialize marshals the key to JSON.
func (JSONKeySerializer[K]) Serialize(key K) (string, error) {
	data, err := json.Marshal(key)
	if err != nil {
		return "", fmt.Errorf("failed to serialize key: %w", err)
	}
	return string(data), nil
}

// Deserialize unmarshals the JSON string back to a key.
func (JSONKeySerializer[K]) Deserialize(s string) (K, error) {
	var key K
	if err := json.Unmarshal([]byte(s), &key); err != nil {
		return key, fmt.Errorf("failed to deserialize key: %w", err)
	}
	return key, nil
}

// DefaultSerialize is used when no serializer is configured.
// It supports string, int, and int64 keys directly, otherwise falls back to JSON.
func DefaultSerialize[K comparable](key K) (string, error) {
	switch v := any(key).(type) {
	case string:
		return v, nil
	case int:
		return strconv.Itoa(v), nil
	case int64:
		return strconv.FormatInt(v, 10), nil
	default:
		return JSONKeySerializer[K]{}.Serialize(key)
	}
}

// DefaultDeserialize is used when no serializer is configured.
// It supports string, int, and int64 keys directly, otherwise falls back to JSON.
func DefaultDeserialize[K comparable](s string) (K, error) {
	var zero K
	switch any(zero).(type) {
	case string:
		return any(s).(K), nil
	case int:
		v, err := strconv.Atoi(s)
		if err != nil {
			return zero, fmt.Errorf("failed to deserialize int key: %w", err)
		}
		return any(v).(K), nil
	case int64:
		v, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			return zero, fmt.Errorf("failed to deserialize int64 key: %w", err)
		}
		return any(v).(K), nil
	default:
		return JSONKeySerializer[K]{}.Deserialize(s)
	}
}
