# version
PRO_DIR   = $(shell pwd)
BUILDTIME = $(shell TZ=Asia/Shanghai date +%Y-%m-%dT%T%z)
GITHASH   = $(shell git rev-parse HEAD)
VERSION   = $(shell echo ${ENV_BK_BSCP_VERSION})
DEBUG     = $(shell echo ${ENV_BK_BSCP_ENABLE_DEBUG})
PREFIX   ?= $(shell pwd)

# 插件 Windows 产物使用 XTLS/go-win7 补丁工具链，兼容 Server 2008 R2 / 2012。
# 官方 Go 1.21+ 在这些系统上会因 ProcessPrng 启动即崩溃。
GO_WIN_TOOLCHAIN_TAG ?= patched-1.26.6
GO_WIN_TOOLCHAIN_DIR ?= $(PRO_DIR)/.toolchain/go-win7-$(GO_WIN_TOOLCHAIN_TAG)

GOBUILD=CGO_ENABLED=0 go build -trimpath
GOBUILD_LINUX_X64=CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath
GOBUILD_WINDOWS_X64=env -u GOROOT GOTOOLCHAIN=local CGO_ENABLED=0 GOOS=windows GOARCH=amd64 "$(GO_WIN_TOOLCHAIN_DIR)/bin/go" build -trimpath

ifeq (${GOOS}, windows)
    BIN_NAME=bscp.exe
else
    BIN_NAME=bscp
endif

ifeq ("$(ENV_BK_BSCP_VERSION)", "")
	VERSION=v1.0.0-devops-unknown
else ifeq ($(shell echo ${ENV_BK_BSCP_VERSION} | egrep "^v1\.[0-9]+\.[0-9]+"),)
	VERSION=v1.0.0-devops-${ENV_BK_BSCP_VERSION}
endif

# 语义化版本, 使用 sed 去掉版本前缀v
SEM_VERSION = $(shell echo $(VERSION) | sed 's/^v//')

export LDVersionFLAG = -X github.com/TencentBlueKing/bk-bscp/pkg/version.VERSION=${VERSION} \
    	-X github.com/TencentBlueKing/bk-bscp/pkg/version.BUILDTIME=${BUILDTIME} \
    	-X github.com/TencentBlueKing/bk-bscp/pkg/version.GITHASH=${GITHASH} \
    	-X github.com/TencentBlueKing/bk-bscp/pkg/version.DEBUG=${DEBUG}


.PHONY: lint
lint:
	@golangci-lint run --fix --issues-exit-code=0

.PHONY: install_go_win_toolchain
install_go_win_toolchain:
	@GO_WIN_TOOLCHAIN_TAG="$(GO_WIN_TOOLCHAIN_TAG)" \
		GO_WIN_TOOLCHAIN_DIR="$(GO_WIN_TOOLCHAIN_DIR)" \
		bash scripts/install-go-win-toolchain.sh

.PHONY: build_initContainer
build_initContainer:
	${GOBUILD} -ldflags "${LDVersionFLAG} \
	-X github.com/TencentBlueKing/bk-bscp/pkg/version.CLIENTTYPE=sidecar" \
	-o build/initContainer/bscp cmd/bscp/*.go

.PHONY: build_sidecar
build_sidecar:
	${GOBUILD} -ldflags "${LDVersionFLAG} \
	-X github.com/TencentBlueKing/bk-bscp/pkg/version.CLIENTTYPE=sidecar" \
	-o build/sidecar/bscp cmd/bscp/*.go

.PHONY: build_docker
build_docker: build_initContainer build_sidecar
	cd build/initContainer && docker build . -t bscp-init
	cd build/sidecar && docker build . -t bscp-sidecar

.PHONY: build
build:
	${GOBUILD} -ldflags "${LDVersionFLAG} \
	-X github.com/TencentBlueKing/bk-bscp/pkg/version.CLIENTTYPE=command" \
	-o bin/${BIN_NAME} cmd/bscp/*.go

.PHONY: build_nodemanPlugin
build_nodemanPlugin: install_go_win_toolchain
	@echo "Building nodemanPlugin version: ${SEM_VERSION}"
	rm -rf build/nodemanPlugin/bkbscp
	# build linux x64
	mkdir -p "build/nodemanPlugin/bkbscp/plugins_linux_x86_64/bkbscp/etc" "build/nodemanPlugin/bkbscp/plugins_linux_x86_64/bkbscp/bin"
	${GOBUILD_LINUX_X64} -ldflags "${LDVersionFLAG} \
		-X github.com/TencentBlueKing/bk-bscp/pkg/version.CLIENTTYPE=agent" \
		-o build/nodemanPlugin/bkbscp/plugins_linux_x86_64/bkbscp/bin/bkbscp build/nodemanPlugin/main.go
	sed -e "s/__VERSION__/$(SEM_VERSION)/g" \
	-e "s/__START_SCRIPT__/.\/start.sh/g" \
	-e "s/__STOP_SCRIPT__/.\/stop.sh/g" \
	-e "s/__RESTART_SCRIPT__/.\/restart.sh/g" \
	-e "s/__RELOAD_SCRIPT__/.\/restart.sh/g" build/nodemanPlugin/project.yaml.tpl > build/nodemanPlugin/bkbscp/plugins_linux_x86_64/bkbscp/project.yaml
	cp build/nodemanPlugin/etc/bkbscp.conf.tpl build/nodemanPlugin/bkbscp/plugins_linux_x86_64/bkbscp/etc/bkbscp.conf.tpl

	# build windows x64
	mkdir -p "build/nodemanPlugin/bkbscp/plugins_windows_x86_64/bkbscp/etc" "build/nodemanPlugin/bkbscp/plugins_windows_x86_64/bkbscp/bin"
	${GOBUILD_WINDOWS_X64} -ldflags "${LDVersionFLAG} \
		-X github.com/TencentBlueKing/bk-bscp/pkg/version.CLIENTTYPE=agent" \
		-o build/nodemanPlugin/bkbscp/plugins_windows_x86_64/bkbscp/bin/bkbscp.exe build/nodemanPlugin/main.go
	sed -e "s/__VERSION__/$(SEM_VERSION)/g" \
	-e "s/__START_SCRIPT__/start.bat/g" \
	-e "s/__STOP_SCRIPT__/stop.bat/g" \
	-e "s/__RESTART_SCRIPT__/restart.bat/g" \
	-e "s/__RELOAD_SCRIPT__/restart.bat/g" build/nodemanPlugin/project.yaml.tpl > build/nodemanPlugin/bkbscp/plugins_windows_x86_64/bkbscp/project.yaml
	cp build/nodemanPlugin/etc/bkbscp.conf.tpl build/nodemanPlugin/bkbscp/plugins_windows_x86_64/bkbscp/etc/bkbscp.conf.tpl

	# tar
	cd build/nodemanPlugin/bkbscp && tar -zcf ../bkbscp.tar.gz .

.PHONY: test
test:
	go test ./...


