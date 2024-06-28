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

package util

import (
	"context"
	"testing"
	"time"
)

func TestProgressiveTicker(t *testing.T) {

	timeout, _ := context.WithTimeout(context.Background(), 30*time.Second) // nolint:govet

	ticker := NewProgressiveTicker([]time.Duration{
		time.Second,
		time.Second,
		2 * time.Second,
		2 * time.Second,
		// 5 * time.Second,
		// 5 * time.Second,
		// 10 * time.Second,
	})

	for {
		now := time.Now()
		select {
		case c := <-ticker.C:
			t.Logf("tick duration: %s", c.Sub(now).String())
		case <-timeout.Done():
			t.Logf("timeout, test done")
			return
		}
	}
}
