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

package option

import "bscp.io/pkg/logs"

// ClientOptions options for bscp sdk client
type ClientOptions struct {
	// FeedAddr BSCP feed_server address
	FeedAddrs []string
	// BizID BSCP business id
	BizID uint32
	// Labels instance labels
	Labels map[string]string
	// Version SDK version
	Version string
	// Fingerprint sdk fingerprint
	Fingerprint string
	// UID sdk uid
	UID string
	// UseFileCache use file cache
	UseFileCache bool
	// FileCacheDir file cache directory
	FileCacheDir string
	// LogVerbosity log verbosity
	LogVerbosity uint
	// DialTimeoutMS dial upstream timeout in millisecond
	DialTimeoutMS int64
	// Token sdk token
	Token string
	// LogOption log option
	LogOption LogOption
}

// LogOption defines log's related configuration
type LogOption struct {
	LogDir           string `yaml:"logDir"`
	MaxPerFileSizeMB uint32 `yaml:"maxPerFileSizeMB"`
	MaxPerLineSizeKB uint32 `yaml:"maxPerLineSizeKB"`
	MaxFileNum       uint   `yaml:"maxFileNum"`
	LogAppend        bool   `yaml:"logAppend"`
	// log the log to std err only, it can not be used with AlsoToStdErr
	// at the same time.
	ToStdErr bool `yaml:"toStdErr"`
	// log the log to file and also to std err. it can not be used with ToStdErr
	// at the same time.
	AlsoToStdErr bool `yaml:"alsoToStdErr"`
	Verbosity    uint `yaml:"verbosity"`
}

// ClientOption setter for bscp sdk options
type ClientOption func(*ClientOptions) error

// FeedAddrs set feed_server addresses
func FeedAddrs(addrs []string) ClientOption {
	// TODO: validate Address
	return func(o *ClientOptions) error {
		o.FeedAddrs = addrs
		return nil
	}
}

// BizID set bscp business id
func BizID(id uint32) ClientOption {
	return func(o *ClientOptions) error {
		o.BizID = id
		return nil
	}
}

// Labels set instance labels
func Labels(labels map[string]string) ClientOption {
	return func(o *ClientOptions) error {
		o.Labels = labels
		return nil
	}
}

// UID set sdk uid
func UID(uid string) ClientOption {
	return func(o *ClientOptions) error {
		o.UID = uid
		return nil
	}
}

// UseFileCache cache file to local file system
func UseFileCache(useFileCache bool) ClientOption {
	return func(o *ClientOptions) error {
		o.UseFileCache = useFileCache
		return nil
	}
}

// FileCacheDir file local cache directory
func FileCacheDir(dir string) ClientOption {
	return func(o *ClientOptions) error {
		o.FileCacheDir = dir
		return nil
	}
}

// WithDialTimeoutMS set dial timeout in millisecond
func WithDialTimeoutMS(timeout int64) ClientOption {
	return func(o *ClientOptions) error {
		o.DialTimeoutMS = timeout
		return nil
	}
}

// Token set sdk token
func Token(token string) ClientOption {
	return func(o *ClientOptions) error {
		o.Token = token
		return nil
	}
}

// LogVerbosity set log verbosity
func LogVerbosity(verbosity uint) ClientOption {
	return func(o *ClientOptions) error {
		o.LogVerbosity = verbosity
		return nil
	}
}

// SetLogOption set log option
func SetLogOption(l *LogOption) ClientOption {
	return func(o *ClientOptions) error {
		o.LogOption = *l
		return nil
	}

}

// TrySetDefault set the log's default value if user not configured.
func (log *LogOption) TrySetDefault() {
	if len(log.LogDir) == 0 {
		log.LogDir = "./log"
	}

	if log.MaxPerFileSizeMB == 0 {
		log.MaxPerFileSizeMB = 500
	}

	if log.MaxPerLineSizeKB == 0 {
		log.MaxPerLineSizeKB = 5
	}

	if log.MaxFileNum == 0 {
		log.MaxFileNum = 5
	}

}

// Logs convert it to logs.LogConfig.
func (log LogOption) Logs() logs.LogConfig {
	l := logs.LogConfig{
		LogDir:             log.LogDir,
		LogMaxSize:         log.MaxPerFileSizeMB,
		LogLineMaxSize:     log.MaxPerLineSizeKB,
		LogMaxNum:          log.MaxFileNum,
		RestartNoScrolling: log.LogAppend,
		ToStdErr:           log.ToStdErr,
		AlsoToStdErr:       log.AlsoToStdErr,
		Verbosity:          log.Verbosity,
	}

	return l
}
