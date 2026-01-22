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

package uid

import (
	"net"
	"os"
	"strings"
	"testing"
)

func TestInstanceID(t *testing.T) {
	if InstanceID == "" {
		t.Error("InstanceID is empty")
	}

	t.Logf("InstanceID: %s", InstanceID)

	// InstanceID should be one of: IP address, hostname-random, or timestamp-random
	// Check if it's a valid IP
	if ip := net.ParseIP(InstanceID); ip != nil {
		return // Valid IP address
	}

	// Check if it starts with hostname (hostname-randomHex format)
	if hostname, err := os.Hostname(); err == nil && hostname != "" {
		if strings.HasPrefix(InstanceID, hostname+"-") {
			return // Valid hostname-random format
		}
	}

	// Should be timestamp-random format (numeric-hex)
	parts := strings.Split(InstanceID, "-")
	if len(parts) != 2 {
		t.Errorf("Expected IP, hostname-random, or timestamp-random format, got: %s", InstanceID)
	}
}
