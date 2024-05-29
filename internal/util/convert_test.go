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
	// 使用 strings.Builder 高效地构建长字符串
	var builder strings.Builder

	// 设置生成字符串的长度
	length := 1024

	// 使用循环生成一个包含 1500 个字符的字符串
	for i := 0; i < length; i++ {
		builder.WriteString("a") // 每次写入一个字符 'a'
	}

	// 获取生成的字符串
	longString := builder.String()

	// 打印字符串的长度
	fmt.Println("字符串长度:", longString)
	fmt.Println("字符串长度:", len(longString))

	// 设置最大长度
	maxLength := 1024

	// 调用截断函数
	result := TruncateString(longString, maxLength)

	fmt.Println(result)
}
