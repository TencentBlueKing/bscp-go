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

// Package metrics defines the metrics for bscp-go.
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
)

const (
	namespace = "bscp_go"
)

var (
	// ReleaseChangeCallbackCounter is the counter of release change event callback
	ReleaseChangeCallbackCounter = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "total_release_change_callback_count",
		Help:      "the total release change count to callback the release change event",
	}, []string{"app", "status", "release"})

	// ReleaseChangeCallbackHandingSecond is the histogram of release change event callback handing time(seconds)
	ReleaseChangeCallbackHandingSecond = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: namespace,
		Name:      "release_change_callback_handing_second",
		Help:      "the handing time(seconds) of release change callback",
		Buckets:   []float64{1, 2, 5, 10, 30, 60, 120, 300, 600, 1800, 3600},
	}, []string{"app", "status", "release"})
)

// RegisterMetrics will register the mtrics
func RegisterMetrics() {
	prometheus.MustRegister(ReleaseChangeCallbackCounter)
	prometheus.MustRegister(ReleaseChangeCallbackHandingSecond)
}
