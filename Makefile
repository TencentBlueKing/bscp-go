# version
PRO_DIR   = $(shell pwd)
BUILDTIME = $(shell TZ=Asia/Shanghai date +%Y-%m-%dT%T%z)
GITHASH   = $(shell git rev-parse HEAD)
VERSION   = $(shell echo ${ENV_BK_BSCP_VERSION})
DEBUG     = $(shell echo ${ENV_BK_BSCP_ENABLE_DEBUG})
PREFIX   ?= $(shell pwd)

GOBUILD=CGO_ENABLED=0 go build -trimpath

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

export LDVersionFLAG = "-X github.com/TencentBlueking/bk-bcs/bcs-services/bcs-bscp/pkg/version.VERSION=${VERSION} \
    	-X github.com/TencentBlueking/bk-bcs/bcs-services/bcs-bscp/pkg/version.BUILDTIME=${BUILDTIME} \
    	-X github.com/TencentBlueking/bk-bcs/bcs-services/bcs-bscp/pkg/version.GITHASH=${GITHASH} \
    	-X github.com/TencentBlueking/bk-bcs/bcs-services/bcs-bscp/pkg/version.DEBUG=${DEBUG}"

.PHONY: lint
lint:
	@golangci-lint run --issues-exit-code=0

.PHONY: build_initContainer
build_initContainer:
	${GOBUILD} -ldflags ${LDVersionFLAG} -o build/initContainer/bscp cmd/bscp/*.go

.PHONY: build_sidecar
build_sidecar:
	${GOBUILD} -ldflags ${LDVersionFLAG} -o build/sidecar/bscp cmd/bscp/*.go

.PHONY: build_docker
build_docker: build_initContainer build_sidecar
	cd build/initContainer && docker build . -t bscp-init
	cd build/sidecar && docker build . -t bscp-sidecar

.PHONY: build
build:
	${GOBUILD} -ldflags ${LDVersionFLAG} -o ${BIN_NAME} cmd/bscp/*.go

.PHONY: test
test:
	go test ./...

.PYONY: build_nodeman_plugin
build_nodeman_plugin:
# 当前仅支持 plugins_linux_x86_64
	mkdir -p "build/nodeman/bkbscp/plugins_linux_x86_64/bkbscp/etc" "build/nodeman/bkbscp/plugins_linux_x86_64/bkbscp/bin"
	cp build/nodeman/project.yaml build/nodeman/bkbscp/plugins_linux_x86_64/bkbscp/project.yaml
	cp build/nodeman/etc/bkbscp.conf.tpl build/nodeman/bkbscp/plugins_linux_x86_64/bkbscp/etc/bkbscp.conf.tpl
	${GOBUILD} -ldflags ${LDVersionFLAG} -o build/nodeman/bkbscp/plugins_linux_x86_64/bkbscp/bin/bkbscp build/nodeman/build.go
	cd build/nodeman/bkbscp && tar -zcf ../bkbscp.tar.gz .

