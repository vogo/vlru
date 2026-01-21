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
	"fmt"
	"os"
	"time"
)

// InstanceID is the unique identifier for this process instance.
// All caches in the same process share this ID.
var InstanceID = generate()

// generate creates a unique instance ID combining hostname, timestamp, and random bytes.
func generate() string {
	hostname, _ := os.Hostname()
	if hostname == "" {
		hostname = "unknown"
	}

	timestamp := time.Now().UnixNano()

	randomBytes := make([]byte, 4)
	_, _ = rand.Read(randomBytes)

	return fmt.Sprintf("%s-%d-%s", hostname, timestamp, hex.EncodeToString(randomBytes))
}
