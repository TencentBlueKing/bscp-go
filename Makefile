# version
PRO_DIR   = $(shell pwd)
BUILDTIME = $(shell date +%Y-%m-%dT%T%z)
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

ifeq ("$(VERSION)", "")
	export OUTPUT_DIR = ${PRO_DIR}/build/bk-bscp
	export LDVersionFLAG = "-X bscp.io/pkg/version.BUILDTIME=${BUILDTIME} \
    	-X bscp.io/pkg/version.GITHASH=${GITHASH} \
		-X bscp.io/pkg/version.DEBUG=${DEBUG}"
else
	export OUTPUT_DIR = ${PRO_DIR}/build/bk-bscp-${VERSION}
	export LDVersionFLAG = "-X bscp.io/pkg/version.VERSION=${VERSION} \
    	-X bscp.io/pkg/version.BUILDTIME=${BUILDTIME} \
    	-X bscp.io/pkg/version.GITHASH=${GITHASH} \
    	-X bscp.io/pkg/version.DEBUG=${DEBUG}"
endif

.PHONY: build_initContainer
build_initContainer:
	${GOBUILD} -ldflags ${LDVersionFLAG} -o build/initContainer/bscp cli/main.go
	cd build/initContainer && docker build . -t bscp-init

.PHONY: build_sidecar
build_sidecar:
	${GOBUILD} -ldflags ${LDVersionFLAG} -o build/sidecar/bscp cli/main.go
	cd build/sidecar && docker build . -t bscp-sidecar

.PHONY: build
build:
	${GOBUILD} -ldflags ${LDVersionFLAG} -o ${BIN_NAME} cli/main.go

.PHONY: test
test:
	go test ./...

