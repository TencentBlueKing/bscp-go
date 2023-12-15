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
	// UseFileCache use file cache
	useFileCache bool
	// FileCacheDir file cache directory
	fileCacheDir string
	// DialTimeoutMS dial upstream timeout in millisecond
	dialTimeoutMS int64
	// Token sdk token
	token string
}

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

// AppOptions options for app pull and watch
type AppOptions struct {
	// Key watch config item key
	Key string
	// Labels instance labels
	Labels map[string]string
	// UID instance unique uid
	UID string
}

// AppOption setter for app options
type AppOption func(*AppOptions)

// WithAppKey set watch config item key
func WithAppKey(key string) AppOption {
	return func(o *AppOptions) {
		o.Key = key
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
