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

package caller

import (
	"strings"
	"testing"
)

func TestGetCallerName(t *testing.T) {
	name := GetCallerName(0)
	t.Logf("Caller name: %s", name)

	// Should contain the test file name
	if !strings.Contains(name, "caller_test.go") {
		t.Errorf("Expected name to contain 'caller_test.go', got %s", name)
	}

	// Should contain the test function name
	if !strings.Contains(name, "TestGetCallerName") {
		t.Errorf("Expected name to contain 'TestGetCallerName', got %s", name)
	}

	// Should contain a line number
	if !strings.Contains(name, ":") {
		t.Errorf("Expected name to contain line number separator ':', got %s", name)
	}
}

func TestGetCallerNameWithSkip(t *testing.T) {
	name := helperFunc()

	// Should contain the helper function name since we skip 1 frame from helperFunc
	if !strings.Contains(name, "TestGetCallerNameWithSkip") {
		t.Errorf("Expected name to contain 'TestGetCallerNameWithSkip', got %s", name)
	}
}

func helperFunc() string {
	return GetCallerName(1)
}

func TestGetCallerNameConsistency(t *testing.T) {
	// Same call site should produce same name
	name1 := GetCallerName(0)
	name2 := GetCallerName(0)
	t.Logf("Caller name 1: %s", name1)
	t.Logf("Caller name 2: %s", name2)

	// The names should be different because they're on different lines
	if name1 == name2 {
		t.Error("Expected different names for different lines")
	}

	// But calling from the same line should be the same (test this in a loop)
	names := make([]string, 10)
	for i := range 10 {
		names[i] = GetCallerName(0)
	}

	// All names from the loop should be the same
	for i := 1; i < 10; i++ {
		if names[i] != names[0] {
			t.Errorf("Expected same name for same call site, got %s vs %s", names[0], names[i])
		}
	}
}

func TestGetCallerNameFormat(t *testing.T) {
	name := GetCallerName(0)

	// Should have format: func:file:line
	parts := strings.Split(name, ":")
	if len(parts) != 3 {
		t.Errorf("Expected format func:file:line with 3 parts, got %d parts: %s", len(parts), name)
	}

	// Second part should be the file name
	if !strings.HasSuffix(parts[1], ".go") {
		t.Errorf("Expected second part to be a .go file, got %s", parts[1])
	}

	// Last part should be a number
	if parts[2] == "" {
		t.Error("Expected line number to be non-empty")
	}
}

func TestGetCallerNameInvalidSkip(t *testing.T) {
	// Very large skip value should return empty string
	name := GetCallerName(1000)
	if name != "" {
		t.Errorf("Expected empty string for invalid skip, got %s", name)
	}
}
