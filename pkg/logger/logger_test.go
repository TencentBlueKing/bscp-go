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

// Package logger defines the logger interface.
package logger

import (
	"io"
	"testing"

	"golang.org/x/exp/slog"
)

func BenchmarkAtomicLogger(b *testing.B) {
	textHandler := slog.NewTextHandler(io.Discard, &slog.HandlerOptions{
		AddSource:   true,
		Level:       slog.LevelInfo,
		ReplaceAttr: ReplaceSourceAttr,
	})
	SetHandler(textHandler)

	for i := 0; i < b.N; i++ {
		Debug("msg")
	}
}

func BenchmarkNonAtomicLogger(t *testing.B) {
	textHandler := slog.NewTextHandler(io.Discard, &slog.HandlerOptions{
		AddSource:   true,
		Level:       slog.LevelInfo,
		ReplaceAttr: ReplaceSourceAttr,
	})

	_logger := slog.New(textHandler)

	for i := 0; i < t.N; i++ {
		_logger.Debug("msg")
	}
}

func BenchmarkAtomicLoggerParallel(b *testing.B) {
	textHandler := slog.NewTextHandler(io.Discard, &slog.HandlerOptions{
		AddSource:   true,
		Level:       slog.LevelInfo,
		ReplaceAttr: ReplaceSourceAttr,
	})
	SetHandler(textHandler)

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			Debug("msg")
		}
	})
}

func BenchmarkNonAtomicLoggerParallel(b *testing.B) {
	textHandler := slog.NewTextHandler(io.Discard, &slog.HandlerOptions{
		AddSource:   true,
		Level:       slog.LevelInfo,
		ReplaceAttr: ReplaceSourceAttr,
	})

	_logger := slog.New(textHandler)

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_logger.Debug("msg")
		}
	})
}
