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

// TruncateString It accepts a string s and an integer maxLength as parameters,
// If the length of string s is greater than maxLength, the first maxLength characters are
// truncated and '...' is appended to the end.
// If the length of string s does not exceed maxLength, the string s is returned directly.
// This function properly handles Chinese characters by counting runes instead of bytes.
func TruncateString(s string, maxLength int) string {
	runes := []rune(s)
	if len(runes) > maxLength {
		return string(runes[:maxLength]) + "..."
	}
	return s
}
