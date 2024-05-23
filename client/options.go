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

package client

// options options for bscp sdk client
type options struct {
	// FeedAddr BSCP feed_server address
	feedAddrs []string
	// BizID BSCP business id
	bizID uint32
	// Labels instance labels
	labels map[string]string
	// Fingerprint sdk fingerprint
	fingerprint string
	// UID sdk uid
	uid string
	// DialTimeoutMS dial upstream timeout in millisecond
	dialTimeoutMS int64
	// Token sdk token
	token string
	// fileCache file cache option
	fileCache FileCache
	// kvCache kv cache option
	kvCache KvCache
	// EnableMonitorResourceUsage 是否采集/监控资源使用率
	enableMonitorResourceUsage bool
}

// FileCache option for file cache
type FileCache struct {
	// Enabled is whether enable file cache
	Enabled bool
	// CacheDir is file cache dir
	CacheDir string
	// ThresholdGB is threshold gigabyte of cleanup
	ThresholdGB float64
	// CleanupIntervalSeconds is interval seconds of cleanup, not exposed for configuration now, use default value
	// CleanupIntervalSeconds int64
	// RetentionRate is retention rate of cleanup, not exposed for configuration now, use default value
	// RetentionRate float64
}

// KvCache option for kv cache
type KvCache struct {
	// Enabled is whether enable kv cache
	Enabled bool
	// ThresholdCount is threshold count of kv cache
	ThresholdCount int
}

const (
	// DefaultCleanupIntervalSeconds is the bscp cli default file cache cleanup interval.
	DefaultCleanupIntervalSeconds = 300
	// DefaultCacheRetentionRate is the bscp cli default file cache retention rate, which is 90%
	DefaultCacheRetentionRate = 0.9
)

// Option setter for bscp sdk options
type Option func(*options) error

// WithFeedAddrs set feed_server addresses
func WithFeedAddrs(addrs []string) Option {
	// TODO: validate Address
	return func(o *options) error {
		o.feedAddrs = addrs
		return nil
	}
}

// WithFeedAddr set feed_server addresse
func WithFeedAddr(addr string) Option {
	// TODO: validate Address
	return func(o *options) error {
		o.feedAddrs = []string{addr}
		return nil
	}
}

// WithBizID set bscp business id
func WithBizID(id uint32) Option {
	return func(o *options) error {
		o.bizID = id
		return nil
	}
}

// WithLabels set instance labels
func WithLabels(labels map[string]string) Option {
	return func(o *options) error {
		o.labels = labels
		return nil
	}
}

// WithUID set sdk uid
func WithUID(uid string) Option {
	return func(o *options) error {
		o.uid = uid
		return nil
	}
}

// WithToken set sdk token
func WithToken(token string) Option {
	return func(o *options) error {
		o.token = token
		return nil
	}
}

// WithFileCache set file cache
func WithFileCache(c FileCache) Option {
	return func(o *options) error {
		o.fileCache = c
		return nil
	}
}

// WithKvCache set kv cache
func WithKvCache(c KvCache) Option {
	return func(o *options) error {
		o.kvCache = c
		return nil
	}
}

// WithEnableMonitorResourceUsage 是否采集/监控资源使用率
func WithEnableMonitorResourceUsage(enable bool) Option {
	return func(o *options) error {
		o.enableMonitorResourceUsage = enable
		return nil
	}
}

// AppOptions options for app pull and watch
type AppOptions struct {
	// Match matches config items
	Match []string
	// Labels instance labels
	Labels map[string]string
	// UID instance unique uid
	UID string
}

// AppOption setter for app options
type AppOption func(*AppOptions)

// WithAppMatch set match condition for config items
func WithAppMatch(match []string) AppOption {
	return func(o *AppOptions) {
		o.Match = match
	}
}

// WithAppLabels set watch labels
func WithAppLabels(labels map[string]string) AppOption {
	return func(o *AppOptions) {
		o.Labels = labels
	}
}

// WithAppUID set watch uid
func WithAppUID(uid string) AppOption {
	return func(o *AppOptions) {
		o.UID = uid
	}
}
