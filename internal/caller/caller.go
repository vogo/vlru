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
	"fmt"
	"path/filepath"
	"runtime"
)

// GetCallerName returns a consistent identifier based on the caller's location.
// skip is the number of stack frames to skip (0 = caller of GetCallerName).
func GetCallerName(skip int) string {
	pc, file, line, ok := runtime.Caller(skip + 1)
	if !ok {
		return ""
	}

	fn := runtime.FuncForPC(pc)
	funcName := ""
	if fn != nil {
		funcName = filepath.Base(fn.Name())
	}

	file = filepath.Base(file)
	return fmt.Sprintf("%s:%s:%d", file, funcName, line)
}
