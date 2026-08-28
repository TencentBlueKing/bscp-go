#!/usr/bin/env bash
#
# 安装用于构建 Windows 产物的 Go 工具链。
#
# 官方 Go 1.21 起把 Windows 最低要求提到 Windows 10 / Server 2016，runtime 初始化时会调用
# bcryptprimitives.dll 的 ProcessPrng（Windows 8 / Server 2012 才引入）。在 Windows 7 /
# Server 2008 R2 / Server 2012 上该符号解析为 0 地址，进程启动即崩溃，表现为
# Exception 0xc0000005、PC=0x0、栈顶停在 runtime.asmstdcall。
#
# XTLS/go-win7 是官方 SDK 的补丁版本，把上述调用换回 Vista 起就存在的
# bcrypt.dll!BCryptGenRandom，同时修掉 RemoveAll 与旧版控制台句柄的兼容问题。
# 节点管理插件的 Windows 产物使用它，Linux 产物与 CLI / sidecar 仍使用官方工具链。
#
# 用法（通常由 Makefile 调用，不需要手动执行）：
#   GO_WIN_TOOLCHAIN_TAG=patched-1.26.6 GO_WIN_TOOLCHAIN_DIR=/path/to/dir \
#     bash scripts/install-go-win-toolchain.sh
#
set -euo pipefail

TAG="${GO_WIN_TOOLCHAIN_TAG:?GO_WIN_TOOLCHAIN_TAG is required}"
DIR="${GO_WIN_TOOLCHAIN_DIR:?GO_WIN_TOOLCHAIN_DIR is required}"
BASE_URL="${GO_WIN_TOOLCHAIN_BASE_URL:-https://github.com/XTLS/go-win7/releases/download}"

# 已知发行包的 sha256。key 为 "<tag>/<host>"，升级 TAG 时必须同步补充，
# 否则脚本会拒绝安装而不是静默跳过校验。
checksum_for() {
    case "$1" in
    "patched-1.26.6/linux-amd64")
        echo "2631cccd3500ebb89258252cfcc2b615b3150e5d8b9e056840f8bf3e9deefc38"
        ;;
    *)
        echo ""
        ;;
    esac
}

detect_host() {
    local os arch
    case "$(uname -s)" in
    Linux) os="linux" ;;
    Darwin) os="darwin" ;;
    *)
        echo "unsupported build host os: $(uname -s)" >&2
        return 1
        ;;
    esac
    case "$(uname -m)" in
    x86_64 | amd64) arch="amd64" ;;
    arm64 | aarch64) arch="arm64" ;;
    *)
        echo "unsupported build host arch: $(uname -m)" >&2
        return 1
        ;;
    esac
    echo "${os}-${arch}"
}

sha256_of() {
    if command -v sha256sum >/dev/null 2>&1; then
        sha256sum "$1" | awk '{print $1}'
    else
        shasum -a 256 "$1" | awk '{print $1}'
    fi
}

# 清掉 GOROOT，否则 gvm/asdf 等导出的 GOROOT 会让这里的 go 命令去用另一套工具链的编译器。
go_version() {
    env -u GOROOT GOTOOLCHAIN=local "$1" version
}

HOST="$(detect_host)"
STAMP="${DIR}/.installed"

if [ -x "${DIR}/bin/go" ] && [ -f "${STAMP}" ] && [ "$(cat "${STAMP}")" = "${TAG}/${HOST}" ]; then
    echo "go-win7 toolchain already installed: $(go_version "${DIR}/bin/go") (${TAG})"
    exit 0
fi

WANT_SHA256="${GO_WIN_TOOLCHAIN_SHA256:-$(checksum_for "${TAG}/${HOST}")}"
if [ -z "${WANT_SHA256}" ]; then
    cat >&2 <<EOF
error: 缺少 ${TAG}/${HOST} 的 sha256 校验值。

请在 ${BASE_URL%/releases/download}/releases/tag/${TAG} 下载
go-for-win7-${HOST}.zip 并核对哈希后，二选一：
  1. 把哈希补进本脚本的 checksum_for()（推荐，可复现）；
  2. 临时通过 GO_WIN_TOOLCHAIN_SHA256=<sha256> 传入。
EOF
    exit 1
fi

ARCHIVE="go-for-win7-${HOST}.zip"
URL="${BASE_URL}/${TAG}/${ARCHIVE}"
TMP_DIR="$(mktemp -d)"
trap 'rm -rf "${TMP_DIR}"' EXIT

echo "downloading ${URL}"
curl -fsSL --retry 3 --retry-delay 2 -o "${TMP_DIR}/${ARCHIVE}" "${URL}"

GOT_SHA256="$(sha256_of "${TMP_DIR}/${ARCHIVE}")"
if [ "${GOT_SHA256}" != "${WANT_SHA256}" ]; then
    echo "error: sha256 mismatch for ${ARCHIVE}" >&2
    echo "  want: ${WANT_SHA256}" >&2
    echo "  got:  ${GOT_SHA256}" >&2
    exit 1
fi

# 发行包内是扁平的 GOROOT 结构（bin/、src/、pkg/ ...），没有额外的顶层目录。
# 先解压到临时目录再整体移动，避免中断后留下不完整的工具链。
unzip -q "${TMP_DIR}/${ARCHIVE}" -d "${TMP_DIR}/extracted"
if [ ! -x "${TMP_DIR}/extracted/bin/go" ]; then
    echo "error: unexpected archive layout, ${ARCHIVE} does not contain bin/go" >&2
    exit 1
fi

rm -rf "${DIR}"
mkdir -p "$(dirname "${DIR}")"
mv "${TMP_DIR}/extracted" "${DIR}"
echo "${TAG}/${HOST}" >"${STAMP}"

echo "installed go-win7 toolchain: $(go_version "${DIR}/bin/go") (${TAG})"
