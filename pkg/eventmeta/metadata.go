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

package eventmeta

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"

	"bscp.io/pkg/logs"
)

// EventMeta defines release event meta that write to metadata.json
type EventMeta struct {
	// ReleaseID release id
	ReleaseID uint32 `json:"releaseID"`
	// Files config item files
	Status EventStatus `json:"status"`
	// Message event message
	Message string `json:"message"`
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
		return errors.New("metadata file path can not be empty")
	}
	if metadata == nil {
		return errors.New("metadata is nil")
	}

	metaFilePath := path.Join(tempDir, "metadata.json")

	metaFile, err := os.OpenFile(metaFilePath, os.O_WRONLY|os.O_CREATE|os.O_APPEND, os.ModePerm)
	if err != nil {
		return fmt.Errorf("open metadata.json failed, err: %s", err.Error())
	}
	defer metaFile.Close()
	b, err := json.Marshal(metadata)
	if err != nil {
		return fmt.Errorf("marshal metadata failed, err: %s", err.Error())
	}
	compress := bytes.NewBuffer([]byte{})
	if err := json.Compact(compress, b); err != nil {
		return fmt.Errorf("compress metadata failed, err: %s", err.Error())
	}
	if _, err := metaFile.WriteString(compress.String() + "\n"); err != nil {
		return fmt.Errorf("append metadata to metadata.json failed, err: %s", err.Error())
	}
	logs.Infof("append event metadata to metadata.json success, event: %s", compress.String())

	return nil
}

// GetLatestMetadataFromFile get latest metadata from file.
func GetLatestMetadataFromFile(tempDir string) (*EventMeta, error) {
	if tempDir == "" {
		return nil, errors.New("metadata file path can not be empty")
	}

	metaFilePath := path.Join(tempDir, "metadata.json")

	metaFile, err := os.Open(metaFilePath)
	if err != nil {
		return nil, fmt.Errorf("open metadata.json failed, err: %s", err.Error())
	}
	defer metaFile.Close()
	var lastLine string
	scanner := bufio.NewScanner(metaFile)
	for scanner.Scan() {
		lastLine = scanner.Text()
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	metadata := &EventMeta{}
	if err := json.Unmarshal([]byte(lastLine), metadata); err != nil {
		return nil, fmt.Errorf("unmarshal metadata failed, err: %s", err.Error())
	}

	return metadata, nil
}
