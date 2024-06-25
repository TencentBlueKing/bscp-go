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

package cache

import (
	"context"
	"time"

	"github.com/allegro/bigcache/v3"
)

var mc *bigcache.BigCache

// EnableMemCache define whether to enable in-memory cache
var EnableMemCache bool

// InitMemCache return a bscp sdk in-memory cache instance
func InitMemCache(thresholdMb float64) error {
	EnableMemCache = true

	config := bigcache.Config{
		// number of shards (must be a power of 2)
		Shards: 1024,

		// time after which entry can be evicted
		// we set a value which will not make entry expired, we mainly focus on HardMaxCacheSize
		LifeWindow: 24 * 365 * 10 * time.Hour,

		// max entry size in bytes, used only in initial memory allocation
		MaxEntrySize: 500,

		// cache will not allocate more memory than this limit, value in MB
		// if value is reached then the oldest entries can be overridden for the new ones
		// 0 value means no size limit
		HardMaxCacheSize: int(thresholdMb),
	}

	var err error
	mc, err = bigcache.New(context.Background(), config)
	if err != nil {
		return err
	}
	return nil
}

// GetMemCache return the in-memory cache instance
func GetMemCache() *bigcache.BigCache {
	return mc
}
