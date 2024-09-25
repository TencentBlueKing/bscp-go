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

// Package env defines the environment variables.
package env

// !important: promise of compatibility
const (
	// HookAppTempDir is environment variable for bk_bscp_app_temp_dir
	HookAppTempDir = "bk_bscp_app_temp_dir"
	// HookTempDir is environment variable for bk_bscp_temp_dir
	HookTempDir = "bk_bscp_temp_dir"
	// HookBiz is environment variable for bk_bscp_biz
	HookBiz = "bk_bscp_biz"
	// HookApp is environment variable for bk_bscp_app
	HookApp = "bk_bscp_app"
	// HookRelName is environment variable for bk_bscp_current_version_name
	HookRelName = "bk_bscp_current_version_name"

	// BscpConfig is environment variable for BSCP_CONFIG, keep uppercase for compatibility
	BscpConfig = "BSCP_CONFIG"
	// LogLevel is environment variable for log_level
	LogLevel = "log_level"
	// Biz is environment variable for biz
	Biz = "biz"
	// App is environment variable for app
	App = "app"
	// Labels is environment variable for labels
	Labels = "labels"
	// LabelsFile is environment variable for labels_file
	LabelsFile = "labels_file"
	// FeedAddrs is environment variable for feed_addrs
	FeedAddrs = "feed_addrs"
	// Token is environment variable for token
	Token = "token"
	// TempDir is environment variable for temp_dir
	TempDir = "temp_dir"
	// ConfigMatches is environment variable for config_matches
	ConfigMatches = "config_matches"
	// EnableP2PDownload is environment variable for enable_p2p_download
	EnableP2PDownload = "enable_p2p_download"
	// BkAgentID is environment variable for bk_agent_id
	BkAgentID = "bk_agent_id"
	// ClusterID is environment variable for cluster_id
	ClusterID = "cluster_id"
	// PodID is environment variable for pod_id
	PodID = "pod_id"
	// PodName is environment variable for pod_name
	PodName = "pod_name"
	// ContainerName is environment variable for container_name
	ContainerName = "container_name"
	// NodeName is environment variable for node_name
	NodeName = "node_name"
	// Namespace is environment variable for name_space
	Namespace = "namespace"
	// IP is environment variable for ip
	IP = "ip"
	// Port is environment variable for port
	Port = "port"
)
