bscp sdk examples
============

## 示例代码
* [pull-file](./pull-file) - 拉取 file 型配置
* [watch-file](./watch-file) - 拉取 file 型配置并监听配置变更
* [pull-kv](./pull-kv) - 拉取 kv 型配置
* [watch-kv](./watch-kv) - 拉取 kv 型配置并监听配置变更

运行测试

添加环境变量
```bash
#  FEED 地址
export BSCP_FEED_ADDRS="bscp-feed.example.com:9510"
# 服务密钥 Token, 记得需要关联配置文件
export BSCP_TOKEN="xxx"
# 当前业务
export BSCP_BIZ="2"
# 当前服务名称
export BSCP_APP="app-test"
```

运行示例
```bash
cd examples/pull-file
go run main.go
```

示例都能运行成功，可参考修改


## 编译&引入
go get github.com/TencentBlueKing/bscp-go

```go
import (
	"github.com/TencentBlueKing/bscp-go"
)
```

## 初始化


