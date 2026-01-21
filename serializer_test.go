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
	"testing"
)

func TestStringKeySerializer(t *testing.T) {
	s := StringKeySerializer{}

	t.Run("Serialize", func(t *testing.T) {
		key := "test-key"
		serialized, err := s.Serialize(key)
		if err != nil {
			t.Fatalf("Serialize failed: %v", err)
		}
		if serialized != key {
			t.Errorf("Expected %q, got %q", key, serialized)
		}
	})

	t.Run("Deserialize", func(t *testing.T) {
		str := "test-key"
		key, err := s.Deserialize(str)
		if err != nil {
			t.Fatalf("Deserialize failed: %v", err)
		}
		if key != str {
			t.Errorf("Expected %q, got %q", str, key)
		}
	})
}

func TestJSONKeySerializer(t *testing.T) {
	t.Run("IntKey", func(t *testing.T) {
		s := JSONKeySerializer[int]{}

		serialized, err := s.Serialize(42)
		if err != nil {
			t.Fatalf("Serialize failed: %v", err)
		}
		if serialized != "42" {
			t.Errorf("Expected %q, got %q", "42", serialized)
		}

		key, err := s.Deserialize(serialized)
		if err != nil {
			t.Fatalf("Deserialize failed: %v", err)
		}
		if key != 42 {
			t.Errorf("Expected %d, got %d", 42, key)
		}
	})

	t.Run("StructKey", func(t *testing.T) {
		type UserKey struct {
			ID   int    `json:"id"`
			Name string `json:"name"`
		}

		s := JSONKeySerializer[UserKey]{}

		original := UserKey{ID: 1, Name: "test"}
		serialized, err := s.Serialize(original)
		if err != nil {
			t.Fatalf("Serialize failed: %v", err)
		}

		key, err := s.Deserialize(serialized)
		if err != nil {
			t.Fatalf("Deserialize failed: %v", err)
		}
		if key.ID != original.ID || key.Name != original.Name {
			t.Errorf("Expected %+v, got %+v", original, key)
		}
	})
}

func TestDefaultSerialize(t *testing.T) {
	t.Run("StringKey", func(t *testing.T) {
		key := "test"
		serialized, err := DefaultSerialize(key)
		if err != nil {
			t.Fatalf("DefaultSerialize failed: %v", err)
		}
		if serialized != key {
			t.Errorf("Expected %q, got %q", key, serialized)
		}
	})

	t.Run("IntKey", func(t *testing.T) {
		key := 42
		serialized, err := DefaultSerialize(key)
		if err != nil {
			t.Fatalf("DefaultSerialize failed: %v", err)
		}
		if serialized != "42" {
			t.Errorf("Expected %q, got %q", "42", serialized)
		}
	})

	t.Run("Int64Key", func(t *testing.T) {
		key := int64(9223372036854775807)
		serialized, err := DefaultSerialize(key)
		if err != nil {
			t.Fatalf("DefaultSerialize failed: %v", err)
		}
		if serialized != "9223372036854775807" {
			t.Errorf("Expected %q, got %q", "9223372036854775807", serialized)
		}
	})
}

func TestDefaultDeserialize(t *testing.T) {
	t.Run("StringKey", func(t *testing.T) {
		serialized := "test"
		key, err := DefaultDeserialize[string](serialized)
		if err != nil {
			t.Fatalf("DefaultDeserialize failed: %v", err)
		}
		if key != serialized {
			t.Errorf("Expected %q, got %q", serialized, key)
		}
	})

	t.Run("IntKey", func(t *testing.T) {
		serialized := "42"
		key, err := DefaultDeserialize[int](serialized)
		if err != nil {
			t.Fatalf("DefaultDeserialize failed: %v", err)
		}
		if key != 42 {
			t.Errorf("Expected %d, got %d", 42, key)
		}
	})

	t.Run("Int64Key", func(t *testing.T) {
		serialized := "9223372036854775807"
		key, err := DefaultDeserialize[int64](serialized)
		if err != nil {
			t.Fatalf("DefaultDeserialize failed: %v", err)
		}
		if key != 9223372036854775807 {
			t.Errorf("Expected %d, got %d", int64(9223372036854775807), key)
		}
	})
}
