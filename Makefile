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

.PHONY: build_gsePlugin
build_gsePlugin: build
	mkdir -p "build/gsePlugin/bkbscp/plugins_linux_x86_64/bkbscp/etc" "build/gsePlugin/bkbscp/plugins_linux_x86_64/bkbscp/bin"
	cp build/gsePlugin/project.yaml build/gsePlugin/bkbscp/plugins_linux_x86_64/bkbscp/project.yaml
	cp build/gsePlugin/etc/bkbscp.conf.tpl build/gsePlugin/bkbscp/plugins_linux_x86_64/bkbscp/etc/bkbscp.conf.tpl
	cd build/gsePlugin/bkbscp && tar -zcf ../bkbscp.tar.gz .

.PHONY: build
build:
	${GOBUILD} -ldflags "${LDVersionFLAG}" -o bin/${BIN_NAME} cmd/bscp/*.go
	${GOBUILD} -ldflags "${LDVersionFLAG} \
		-X github.com/TencentBlueKing/bk-bcs/bcs-services/bcs-bscp/pkg/version.CLIENTTYPE=agent" \
		-o bin/bkbscp build/gsePlugin/main.go

.PHONY: test
test:
	go test ./...


