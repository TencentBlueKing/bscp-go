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

package config

import (
	"fmt"

	// for unmarshal yaml config file
	_ "gopkg.in/yaml.v2"

	"github.com/TencentBlueKing/bscp-go/cli/constant"
)

// ClientConfig config for bscp-go when run as daemon
type ClientConfig struct {
	// FeedAddrs bscp feed server addresses
	FeedAddrs []string `json:"feed_addrs" mapstructure:"feed_addrs"`
	// Biz bscp biz id
	Biz uint32 `json:"biz" mapstructure:"biz"`
	// Token bscp sdk token
	Token string `json:"token" mapstructure:"token"`
	// Apps bscp watched apps
	Apps []*AppConfig `json:"apps" mapstructure:"apps"`
	// Labels bscp sdk labels
	Labels map[string]string `json:"labels" mapstructure:"labels"`
	// UID bscp sdk uid
	UID string `json:"uid" mapstructure:"uid"`
	// TempDir config files temporary directory
	TempDir string `json:"temp_dir" mapstructure:"temp_dir"`
}

// Validate validate the watch config
func (c *ClientConfig) Validate() error {
	if len(c.FeedAddrs) == 0 {
		return fmt.Errorf("feedAddrs is empty")
	}
	if c.Biz == 0 {
		return fmt.Errorf("bizID is empty")
	}
	if c.Token == "" {
		return fmt.Errorf("token is empty")
	}
	if len(c.Apps) == 0 {
		return fmt.Errorf("watched apps is empty")
	}
	exists := make(map[string]bool)
	for _, app := range c.Apps {
		if exists[app.Name] {
			return fmt.Errorf("watch repeated for app %s: ", app.Name)
		}
		if err := app.Validate(); err != nil {
			return err
		}
		exists[app.Name] = true
	}
	if c.TempDir == "" {
		c.TempDir = constant.DefaultTempDir
	}
	return nil
}

// AppConfig config for watched app
type AppConfig struct {
	// Name BSCP app name
	Name string `json:"name" mapstructure:"name"`
	// Labels instance labels
	Labels map[string]string `json:"labels" mapstructure:"labels"`
	// UID instance unique uid
	UID string `json:"uid" mapstructure:"uid"`
}

// Validate validate the app watch config
func (c *AppConfig) Validate() error {
	if c.Name == "" {
		return fmt.Errorf("app is empty")
	}
	if len(c.Labels) == 0 {
		return fmt.Errorf("labels is empty")
	}
	return nil
}
