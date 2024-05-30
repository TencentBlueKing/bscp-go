/*
 * Tencent is pleased to support the open source community by making Blueking Container Service available.
 * Copyright (C) 2019 THL A29 Limited, a Tencent company. All rights reserved.
 * Licensed under the MIT License (the "License"); you may not use this file except
 * in compliance with the License. You may obtain a copy of the License at
 * http://opensource.org/licenses/MIT
 * Unless required by applicable law or agreed to in writing, software distributed under
 * the License is distributed on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND,
 * either express or implied. See the License for the specific language governing permissions and
 * limitations under the License.
 */

// Package constant defines the constants for cli.
package constant

const (
	// DefaultTempDir is the bscp cli default temp dir.
	// !important: promise of compatibility
	DefaultTempDir = "/data/bscp"

	// DefaultFileCacheEnabled is the bscp cli default file cache switch.
	// !important: promise of compatibility
	DefaultFileCacheEnabled = true
	// DefaultFileCacheDir is the bscp cli default file cache dir.
	// !important: promise of compatibility
	DefaultFileCacheDir = "/data/bscp/cache"
	// DefaultCleanupIntervalSeconds is the bscp cli default file cache cleanup interval.
	// !important: promise of compatibility
	DefaultCleanupIntervalSeconds = 300
	// DefaultCacheThresholdGB is the bscp cli default file cache threshold, which is 2GB
	// !important: promise of compatibility
	DefaultCacheThresholdGB = 2
	// DefaultCacheRetentionRate is the bscp cli default file cache retention rate, which is 90%
	// !important: promise of compatibility
	DefaultCacheRetentionRate = 0.9

	// DefaultKvCacheEnabled is the bscp cli default kv cache switch.
	// !important: promise of compatibility
	DefaultKvCacheEnabled = true
	// DefaultKvCacheThresholdMB is the bscp cli default file cache threshold, which is 500MB
	// !important: promise of compatibility
	DefaultKvCacheThresholdMB = 500

	// DefaultHttpPort is the bscp sidecar default http port.
	// !important: promise of compatibility
	DefaultHttpPort = 9616
)
