/*
 * TencentBlueKing is pleased to support the open source community by making
 * 蓝鲸智云-权限中心Go SDK(iam-go-sdk) available.
 * Copyright (C) 2017-2021 THL A29 Limited, a Tencent company. All rights reserved.
 * Licensed under the MIT License (the "License"); you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at http://opensource.org/licenses/MIT
 * Unless required by applicable law or agreed to in writing, software distributed under the License is distributed on
 * an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the License for the
 * specific language governing permissions and limitations under the License.
 *
 */

package logger

import (
	"os"

	"github.com/sirupsen/logrus"
)

// Logger represent common interface for logging function
type Logger interface {
	Errorf(format string, args ...interface{})
	Error(args ...interface{})
	Fatalf(format string, args ...interface{})
	Fatal(args ...interface{})
	Infof(format string, args ...interface{})
	Info(args ...interface{})
	Warnf(format string, args ...interface{})
	Warn(args ...interface{})
	Debugf(format string, args ...interface{})
	Debug(args ...interface{})
}

var log Logger

func init() {
	// default is logrus
	log = &logrus.Logger{
		Out:          os.Stderr,
		Formatter:    new(logrus.TextFormatter),
		Hooks:        make(logrus.LevelHooks),
		Level:        logrus.InfoLevel,
		ExitFunc:     os.Exit,
		ReportCaller: false,
	}
}

// SetLogger will set an logger implements for the sdk
func SetLogger(l Logger) {
	log = l
}

// Errorf log error
func Errorf(format string, args ...interface{}) {
	log.Errorf(format, args...)
}

// Error log error
func Error(args ...interface{}) {
	log.Error(args...)
}

// Fatalf log fatal
func Fatalf(format string, args ...interface{}) {
	log.Fatalf(format, args...)
}

// Fatal log fatal
func Fatal(args ...interface{}) {
	log.Fatal(args...)
}

// Infof log info
func Infof(format string, args ...interface{}) {
	log.Infof(format, args...)
}

// Info log info
func Info(args ...interface{}) {
	log.Info(args...)
}

// Warnf log warn
func Warnf(format string, args ...interface{}) {
	log.Warnf(format, args...)
}

// Warn log warn
func Warn(args ...interface{}) {
	log.Warn(args...)
}

// Debugf log debug
func Debugf(format string, args ...interface{}) {
	log.Debugf(format, args...)
}

// Debug log debug
func Debug(args ...interface{}) {
	log.Debug(args...)
}
