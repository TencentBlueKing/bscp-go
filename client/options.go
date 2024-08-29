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
	// enableP2PDownload
	enableP2PDownload bool
	// bkAgentID bk gse agent id
	bkAgentID string
	// clusterID bcs cluster id
	clusterID string
	// podID id of the pod where the bscp container resides
	podID string
	// containerName bscp container name
	containerName string
	// fileCache file cache option
	fileCache FileCache
	// kvCache kv cache option
	kvCache KvCache
	// EnableMonitorResourceUsage 是否采集/监控资源使用率
	enableMonitorResourceUsage bool
	// textLineBreak is the text file line break character, default as LF
	textLineBreak string
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

// P2PDownload option for p2p download file
type P2PDownload struct {
	// Enabled is whether enable p2p download file
	Enabled bool
	// BkAgentID bk gse agent id
	BkAgentID string
	// ClusterID bcs cluster id
	ClusterID string
	// PodID id of the pod where the bscp container resides
	PodID string
	// ContainerName bscp container name
	ContainerName string
}

// KvCache option for kv cache
type KvCache struct {
	// Enabled is whether enable kv cache
	Enabled bool
	// ThresholdMB is threshold megabyte of kv cache
	ThresholdMB float64
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

// WithP2PDownload enable p2p download file
func WithP2PDownload(enabled bool) Option {
	return func(o *options) error {
		o.enableP2PDownload = enabled
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

// WithBkAgentID set bk gse agent id
func WithBkAgentID(agentID string) Option {
	return func(o *options) error {
		o.bkAgentID = agentID
		return nil
	}
}

// WithClusterID set bcs cluster id
func WithClusterID(clusterID string) Option {
	return func(o *options) error {
		o.clusterID = clusterID
		return nil
	}
}

// WithPodID set pod id where the bscp container resides
func WithPodID(podID string) Option {
	return func(o *options) error {
		o.podID = podID
		return nil
	}
}

// WithContainerName set container name of the bscp container
func WithContainerName(name string) Option {
	return func(o *options) error {
		o.containerName = name
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

// WithTextLineBreak set text file line break character
func WithTextLineBreak(lineBreak string) Option {
	return func(o *options) error {
		o.textLineBreak = lineBreak
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

// WithAppConfigMatch set match condition for app's config items
func WithAppConfigMatch(match []string) AppOption {
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
