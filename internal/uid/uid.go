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

// Package uid provides unique identifier generation for cache instances.
package uid

import (
	"crypto/rand"
	"encoding/hex"
	"net"
	"os"
	"strconv"
	"time"
)

// InstanceID is the unique identifier for this process instance.
// All caches in the same process share this ID.
var InstanceID = generate()

// generate creates a unique instance ID.
// Priority: IP address > hostname > timestamp+randomBytes.
func generate() string {
	// Try to get local IP address first
	if ip := getLocalIP(); ip != "" {
		return ip
	}

	// Fall back to hostname + random hex
	if hostname, err := os.Hostname(); err == nil && hostname != "" {
		return hostname + "-" + randomHex()
	}

	// Last resort: timestamp + random hex
	return strconv.FormatInt(time.Now().UnixNano(), 10) + "-" + randomHex()
}

// getLocalIP returns the non-loopback local IP address, or empty string if not found.
func getLocalIP() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return ""
	}

	for _, addr := range addrs {
		if ipNet, ok := addr.(*net.IPNet); ok && !ipNet.IP.IsLoopback() {
			if ipNet.IP.To4() != nil {
				return ipNet.IP.String()
			}
		}
	}

	return ""
}

// randomHex generates a random hex string of length 8.
func randomHex() string {
	randomBytes := make([]byte, 4)
	_, _ = rand.Read(randomBytes)
	return hex.EncodeToString(randomBytes)
}
