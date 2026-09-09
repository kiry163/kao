#!/bin/sh
# Kao 一键安装脚本:从 GitHub Releases 下载最新 k 并安装到 ~/.local/bin。
# 用法: curl -fsSL https://raw.githubusercontent.com/kiry163/kao/main/install.sh | sh
#
# 环境变量:
#   INSTALL_DIR  安装目录(默认 ~/.local/bin)
#   KAO_VERSION  指定版本(默认 latest release,如 v0.1.0)
set -eu

REPO="kiry163/kao"
INSTALL_DIR="${INSTALL_DIR:-$HOME/.local/bin}"

# --- 检测平台 ---
os=$(uname -s | tr '[:upper:]' '[:lower:]')
arch=$(uname -m)
case "$arch" in
  x86_64|amd64) asset_arch="amd64" ;;
  aarch64|arm64) asset_arch="arm64" ;;
  *) echo "不支持的架构: $arch" >&2; exit 1 ;;
esac
case "$os" in
  linux|darwin) ;;
  *) echo "不支持的操作系统: $os" >&2; exit 1 ;;
esac

# --- sha256 校验命令:Linux sha256sum,macOS shasum -a 256 ---
if command -v sha256sum >/dev/null 2>&1; then
  SHA256() { sha256sum -c; }
elif command -v shasum >/dev/null 2>&1; then
  SHA256() { shasum -a 256 -c; }
else
  SHA256() { return 0; }
fi

# --- 解析版本与资源 URL ---
if [ -n "${KAO_VERSION:-}" ]; then
  version="$KAO_VERSION"
else
  echo "获取最新版本..."
  version=$(curl -fsSL "https://api.github.com/repos/$REPO/releases/latest" \
    | sed -n 's/.*"tag_name": *"\([^"]*\)".*/\1/p' | head -n1)
  [ -n "$version" ] || { echo "获取最新版本失败" >&2; exit 1; }
fi

base="https://github.com/$REPO/releases/download/$version"
asset="k-${os}-${asset_arch}"
echo "下载 $base/$asset (版本 $version)"

mkdir -p "$INSTALL_DIR"
tmpdir=$(mktemp -d)
trap 'rm -rf "$tmpdir"' EXIT

curl -fsSL -o "$tmpdir/$asset" "$base/$asset"
chmod +x "$tmpdir/$asset"

# --- 校验 sha256(存在 checksums.txt 时) ---
if curl -fsSL -o "$tmpdir/checksums.txt" "$base/checksums.txt" 2>/dev/null; then
  (cd "$tmpdir" && SHA256 < checksums.txt) \
    || { echo "sha256 校验失败,已中止" >&2; exit 1; }
  echo "sha256 校验通过"
fi

mv "$tmpdir/$asset" "$INSTALL_DIR/k"
echo "已安装到 $INSTALL_DIR/k"

case ":$PATH:" in
  *":$INSTALL_DIR:"*) ;;
  *) echo "警告: $INSTALL_DIR 不在 PATH 中,请将其加入 PATH 后使用 k" >&2 ;;
esac
echo "在 herdr pane 内直接敲 k 即可使用。"
