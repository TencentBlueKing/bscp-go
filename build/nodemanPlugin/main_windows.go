package main

import (
	"fmt"
	"net"

	"github.com/TencentBlueKing/bscp-go/internal/constant"
)

// netListen windows 2008/2012/2016 不支持 unix_socket, 使用监听 tcp 方式, 只允许一个实例
func netListen() (net.Listener, error) {
	return net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", constant.DefaultHttpPort))
}
