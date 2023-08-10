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
 *
 */

package upstream

// Options options for watch bscp config items
type Options struct {
	// BizID BSCP business id
	BizID uint32
	// App BSCP app name
	App string
	// Key watch config item key
	Key string
	// Labels instance labels
	Labels map[string]string
	// UID instance unique uid
	UID string
	// FeedAddrs bscp feed server addresses
	FeedAddrs []string
	// DialTimeoutMS dial timeout milliseconds
	DialTimeoutMS int64
}

// Option setter for bscp watch options
type Option func(*Options)

// WithApp set watch app name
func WithApp(app string) Option {
	return func(o *Options) {
		o.App = app
	}
}

// WithKey set watch config item key
func WithKey(key string) Option {
	return func(o *Options) {
		o.Key = key
	}
}

// WitchLabels set watch labels
func WitchLabels(labels map[string]string) Option {
	return func(o *Options) {
		o.Labels = labels
	}
}

// WithUID set watch uid
func WithUID(uid string) Option {
	return func(o *Options) {
		o.UID = uid
	}
}

// WithFeedAddrs set bscp feed server addresses
func WithFeedAddrs(addrs []string) Option {
	return func(o *Options) {
		o.FeedAddrs = addrs
	}
}

// WithDialTimeoutMS set dial timeout milliseconds
func WithDialTimeoutMS(timeout int64) Option {
	return func(o *Options) {
		o.DialTimeoutMS = timeout
	}
}

// WithBizID set bscp business id
func WithBizID(id uint32) Option {
	return func(o *Options) {
		o.BizID = id
	}
}
