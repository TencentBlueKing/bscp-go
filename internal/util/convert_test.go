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
	"fmt"
	"strings"
	"testing"
)

func TestTruncateString(t *testing.T) {
	// Test with English characters
	englishString := strings.Repeat("a", 1000)
	result := TruncateString(englishString, 800)
	expectedLength := 800 + 3 // 800 characters + "..."
	if len([]rune(result)) != expectedLength {
		t.Errorf("Expected length %d, got %d", expectedLength, len([]rune(result)))
	}

	// Test with Chinese characters
	chineseString := strings.Repeat("中", 1000) // 1000 Chinese characters
	result = TruncateString(chineseString, 800)
	resultRunes := []rune(result)
	expectedLength = 800 + 3 // 800 Chinese characters + "..."
	if len(resultRunes) != expectedLength {
		t.Errorf("Expected Chinese string length %d, got %d", expectedLength, len(resultRunes))
	}

	// Test with mixed characters
	mixedString := "Error: 错误信息：" + strings.Repeat("测试", 400) // Mixed English and Chinese
	result = TruncateString(mixedString, 800)
	resultRunes = []rune(result)
	if len(resultRunes) > 803 { // Should not exceed 800 + 3 ("...")
		t.Errorf("Mixed string truncation failed, length: %d", len(resultRunes))
	}

	// Test string shorter than max length
	shortString := "短字符串"
	result = TruncateString(shortString, 800)
	if result != shortString {
		t.Errorf("Short string should not be truncated")
	}

	fmt.Printf("English test passed\n")
	fmt.Printf("Chinese test passed\n")
	fmt.Printf("Mixed test passed\n")
	fmt.Printf("Short string test passed\n")
}
