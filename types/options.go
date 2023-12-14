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

package types

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
