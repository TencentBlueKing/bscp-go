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

# 语义化版本, 使用 sed 去掉版本前缀v
SEM_VERSION = $(shell echo $(VERSION) | sed 's/^v//')

export LDVersionFLAG = -X github.com/TencentBlueKing/bk-bcs/bcs-services/bcs-bscp/pkg/version.VERSION=${VERSION} \
    	-X github.com/TencentBlueKing/bk-bcs/bcs-services/bcs-bscp/pkg/version.BUILDTIME=${BUILDTIME} \
    	-X github.com/TencentBlueKing/bk-bcs/bcs-services/bcs-bscp/pkg/version.GITHASH=${GITHASH} \
    	-X github.com/TencentBlueKing/bk-bcs/bcs-services/bcs-bscp/pkg/version.DEBUG=${DEBUG}


.PHONY: lint
lint:
	@golangci-lint run --issues-exit-code=0

.PHONY: build_initContainer
build_initContainer:
	${GOBUILD} -ldflags "${LDVersionFLAG} \
	-X github.com/TencentBlueKing/bk-bcs/bcs-services/bcs-bscp/pkg/version.CLIENTTYPE=sidecar" \
	-o build/initContainer/bscp cmd/bscp/*.go

.PHONY: build_sidecar
build_sidecar:
	${GOBUILD} -ldflags "${LDVersionFLAG} \
	-X github.com/TencentBlueKing/bk-bcs/bcs-services/bcs-bscp/pkg/version.CLIENTTYPE=sidecar" \
	-o build/sidecar/bscp cmd/bscp/*.go

.PHONY: build_docker
build_docker: build_initContainer build_sidecar
	cd build/initContainer && docker build . -t bscp-init
	cd build/sidecar && docker build . -t bscp-sidecar

.PHONY: build_nodemanPlugin
build_nodemanPlugin: build
	@echo "Building nodemanPlugin version: ${SEM_VERSION}"
	rm -rf build/nodemanPlugin/bkbscp/plugins_linux_x86_64
	mkdir -p "build/nodemanPlugin/bkbscp/plugins_linux_x86_64/bkbscp/etc" "build/nodemanPlugin/bkbscp/plugins_linux_x86_64/bkbscp/bin"
	cp bin/bkbscp build/nodemanPlugin/bkbscp/plugins_linux_x86_64/bkbscp/bin
	sed 's/__VERSION__/$(SEM_VERSION)/' build/nodemanPlugin/project.yaml > build/nodemanPlugin/bkbscp/plugins_linux_x86_64/bkbscp/project.yaml
	cp build/nodemanPlugin/etc/bkbscp.conf.tpl build/nodemanPlugin/bkbscp/plugins_linux_x86_64/bkbscp/etc/bkbscp.conf.tpl
	cd build/nodemanPlugin/bkbscp && tar -zcf ../bkbscp.tar.gz .

.PHONY: build
build:
	${GOBUILD} -ldflags "${LDVersionFLAG}" -o bin/${BIN_NAME} cmd/bscp/*.go
	${GOBUILD} -ldflags "${LDVersionFLAG} \
		-X github.com/TencentBlueKing/bk-bcs/bcs-services/bcs-bscp/pkg/version.CLIENTTYPE=agent" \
		-o bin/bkbscp build/nodemanPlugin/main.go

.PHONY: test
test:
	go test ./...


