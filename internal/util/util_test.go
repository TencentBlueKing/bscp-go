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
	"testing"
)

func TestStrSlicesEqual(t *testing.T) {
	tests := []struct {
		name     string
		slice1   []string
		slice2   []string
		expected bool
	}{
		{
			name:     "Equal slices with same order",
			slice1:   []string{"apple", "banana", "cherry"},
			slice2:   []string{"apple", "banana", "cherry"},
			expected: true,
		},
		{
			name:     "Equal slices with different order",
			slice1:   []string{"apple", "banana", "cherry"},
			slice2:   []string{"banana", "cherry", "apple"},
			expected: true,
		},
		{
			name:     "Slices with different lengths",
			slice1:   []string{"apple", "banana"},
			slice2:   []string{"apple", "banana", "cherry"},
			expected: false,
		},
		{
			name:     "Slices with different elements",
			slice1:   []string{"apple", "banana", "cherry"},
			slice2:   []string{"apple", "banana", "date"},
			expected: false,
		},
		{
			name:     "Slices with different element counts",
			slice1:   []string{"apple", "banana", "banana"},
			slice2:   []string{"apple", "banana"},
			expected: false,
		},
		{
			name:     "Empty slices",
			slice1:   []string{},
			slice2:   []string{},
			expected: true,
		},
		{
			name:     "One empty slice",
			slice1:   []string{"apple", "banana"},
			slice2:   []string{},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := StrSlicesEqual(tt.slice1, tt.slice2)
			if result != tt.expected {
				t.Errorf("StrSlicesEqual(%v, %v) = %v; want %v", tt.slice1, tt.slice2, result, tt.expected)
			}
		})
	}
}
