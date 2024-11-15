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

// Package eventmeta defines the callback event metadata for release change event.
package eventmeta

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	sfs "github.com/TencentBlueKing/bk-bscp/pkg/sf-share"
	"golang.org/x/exp/slog"

	"github.com/TencentBlueKing/bscp-go/pkg/logger"
)

// EventMeta defines release event meta that write to metadata.json
type EventMeta struct {
	// ReleaseID release id
	ReleaseID uint32 `json:"releaseID"`
	// Files config item files
	Status EventStatus `json:"status"`
	// Message event message
	Message string `json:"message"`
	// ConfigMatches app config item's match conditions
	ConfigMatches []string `json:"configMatches"`
	// EventTime event time
	EventTime string `json:"eventTime"`
}

// EventStatus defines event status
type EventStatus string

const (
	// EventStatusSuccess event success
	EventStatusSuccess EventStatus = "SUCCESS"
	// EventStatusFailed event failed
	EventStatusFailed EventStatus = "FAILED"
)

// AppendMetadataToFile append metadata to file.
func AppendMetadataToFile(tempDir string, metadata *EventMeta) error {
	if tempDir == "" {
		return sfs.WrapPrimaryError(sfs.UpdateMetadataFailed,
			sfs.SecondaryError{SpecificFailedReason: sfs.FilePathNotFound,
				Err: errors.New("metadata file path can not be empty")})
	}
	if metadata == nil {
		return sfs.WrapPrimaryError(sfs.UpdateMetadataFailed,
			sfs.SecondaryError{SpecificFailedReason: sfs.DataEmpty,
				Err: errors.New("metadata is nil")})
	}
	// prepare temp dir, make sure it exists
	if err := os.MkdirAll(tempDir, os.ModePerm); err != nil {
		return sfs.WrapPrimaryError(sfs.UpdateMetadataFailed,
			sfs.SecondaryError{SpecificFailedReason: sfs.NewFolderFailed, Err: err})
	}

	metaFilePath := filepath.Join(tempDir, "metadata.json")

	metaFile, err := os.OpenFile(metaFilePath, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0644)
	if err != nil {
		return sfs.WrapPrimaryError(sfs.UpdateMetadataFailed,
			sfs.SecondaryError{SpecificFailedReason: sfs.OpenFileFailed,
				Err: fmt.Errorf("open metadata.json failed, err: %s", err.Error())})
	}
	defer metaFile.Close()
	b, err := json.Marshal(metadata)
	if err != nil {
		return sfs.WrapPrimaryError(sfs.UpdateMetadataFailed,
			sfs.SecondaryError{SpecificFailedReason: sfs.SerializationFailed,
				Err: fmt.Errorf("marshal metadata failed, err: %s", err.Error())})
	}
	compress := bytes.NewBuffer([]byte{})
	if err := json.Compact(compress, b); err != nil {
		return sfs.WrapPrimaryError(sfs.UpdateMetadataFailed,
			sfs.SecondaryError{SpecificFailedReason: sfs.FormattingFailed,
				Err: fmt.Errorf("compress metadata failed, err: %s", err.Error())})
	}
	if _, err := metaFile.WriteString(compress.String() + "\n"); err != nil {
		return sfs.WrapPrimaryError(sfs.UpdateMetadataFailed,
			sfs.SecondaryError{SpecificFailedReason: sfs.WriteFileFailed,
				Err: fmt.Errorf("append metadata to metadata.json failed, err: %s", err.Error())})
	}
	logger.Info("append event metadata to metadata.json success", slog.String("event", compress.String()))

	return nil
}

// GetLatestMetadataFromFile get latest metadata from file.
// Return metadata, exists, error
func GetLatestMetadataFromFile(tempDir string) (*EventMeta, bool, error) {
	if tempDir == "" {
		return nil, false, sfs.WrapPrimaryError(sfs.UpdateMetadataFailed,
			sfs.SecondaryError{SpecificFailedReason: sfs.DataEmpty,
				Err: errors.New("metadata file path can not be empty")})
	}

	metaFilePath := filepath.Join(tempDir, "metadata.json")

	metaFile, err := os.Open(metaFilePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, false, nil
		}
		return nil, false, sfs.WrapPrimaryError(sfs.UpdateMetadataFailed,
			sfs.SecondaryError{SpecificFailedReason: sfs.OpenFileFailed,
				Err: err})
	}
	defer metaFile.Close()
	var lastLine string
	scanner := bufio.NewScanner(metaFile)
	for scanner.Scan() {
		lastLine = scanner.Text()
	}
	if err := scanner.Err(); err != nil {
		return nil, false, sfs.WrapPrimaryError(sfs.UpdateMetadataFailed,
			sfs.SecondaryError{SpecificFailedReason: sfs.ReadFileFailed,
				Err: err})
	}

	metadata := &EventMeta{}
	if err := json.Unmarshal([]byte(lastLine), metadata); err != nil {
		return nil, false, sfs.WrapPrimaryError(sfs.UpdateMetadataFailed,
			sfs.SecondaryError{SpecificFailedReason: sfs.SerializationFailed,
				Err: fmt.Errorf("unmarshal metadata failed, err: %s", err.Error())})
	}

	return metadata, true, nil
}

// ChangeEvent 记录变更事件的结构体
type ChangeEvent struct {
	// ReleaseID release id
	ReleaseID uint32 `json:"releaseID"`
	// Status event status
	Status EventStatus `json:"status"`
}

// RecordChangeEvent 记录变更的事件
func RecordChangeEvent(tempDir string, eventData *ChangeEvent) error {
	if tempDir == "" {
		return sfs.WrapPrimaryError(sfs.UnknownFailed,
			sfs.SecondaryError{SpecificFailedReason: sfs.FilePathNotFound,
				Err: errors.New("the file path for record change event is empty")})
	}

	// prepare temp dir, make sure it exists
	if err := os.MkdirAll(tempDir, os.ModePerm); err != nil {
		return sfs.WrapPrimaryError(sfs.UnknownFailed,
			sfs.SecondaryError{SpecificFailedReason: sfs.NewFolderFailed, Err: err})
	}

	metaFilePath := filepath.Join(tempDir, "changeevent.json")

	metaFile, err := os.OpenFile(metaFilePath, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0644)
	if err != nil {
		return sfs.WrapPrimaryError(sfs.UnknownFailed,
			sfs.SecondaryError{SpecificFailedReason: sfs.OpenFileFailed,
				Err: fmt.Errorf("open record change event file failed, err: %s", err.Error())})
	}
	defer metaFile.Close()

	b, err := json.Marshal(eventData)
	if err != nil {
		return sfs.WrapPrimaryError(sfs.UpdateMetadataFailed,
			sfs.SecondaryError{SpecificFailedReason: sfs.SerializationFailed,
				Err: fmt.Errorf("marshal metadata failed, err: %s", err.Error())})
	}
	compress := bytes.NewBuffer([]byte{})
	if err := json.Compact(compress, b); err != nil {
		return sfs.WrapPrimaryError(sfs.UpdateMetadataFailed,
			sfs.SecondaryError{SpecificFailedReason: sfs.FormattingFailed,
				Err: fmt.Errorf("compress metadata failed, err: %s", err.Error())})
	}
	if _, err := metaFile.WriteString(compress.String() + "\n"); err != nil {
		return sfs.WrapPrimaryError(sfs.UpdateMetadataFailed,
			sfs.SecondaryError{SpecificFailedReason: sfs.WriteFileFailed,
				Err: fmt.Errorf("record change event failed, err: %s", err.Error())})
	}

	logger.Info("record change event success", slog.String("event", compress.String()))

	return nil
}

// GetLatestChangeEventFromFile get the last data from the file that change event
func GetLatestChangeEventFromFile(tempDir string) (*ChangeEvent, error) {
	if tempDir == "" {
		return nil, sfs.WrapPrimaryError(sfs.UnknownFailed,
			sfs.SecondaryError{SpecificFailedReason: sfs.DataEmpty,
				Err: errors.New("the file path for record change event is empty")})
	}

	metaFilePath := filepath.Join(tempDir, "changeevent.json")

	metaFile, err := os.Open(metaFilePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, sfs.WrapPrimaryError(sfs.UpdateMetadataFailed,
			sfs.SecondaryError{SpecificFailedReason: sfs.OpenFileFailed,
				Err: err})
	}
	defer metaFile.Close()
	var lastLine string
	scanner := bufio.NewScanner(metaFile)
	for scanner.Scan() {
		lastLine = scanner.Text()
	}
	if err := scanner.Err(); err != nil {
		return nil, sfs.WrapPrimaryError(sfs.UpdateMetadataFailed,
			sfs.SecondaryError{SpecificFailedReason: sfs.ReadFileFailed,
				Err: err})
	}

	metadata := &ChangeEvent{}
	if err := json.Unmarshal([]byte(lastLine), metadata); err != nil {
		return nil, sfs.WrapPrimaryError(sfs.UpdateMetadataFailed,
			sfs.SecondaryError{SpecificFailedReason: sfs.SerializationFailed,
				Err: fmt.Errorf("unmarshal metadata failed, err: %s", err.Error())})
	}

	return metadata, nil
}
