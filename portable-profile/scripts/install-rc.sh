#!/bin/sh
# Install the official, pinned rc-cli release to a private user cache.
# Never run an unverified download or silently fall back to source builds.
set -eu

version=v0.6.0
os=$(uname -s | tr '[:upper:]' '[:lower:]')
case $(uname -m) in
  x86_64) arch=amd64 ;;
  arm64|aarch64) arch=arm64 ;;
  *) echo 'rc-cli: unsupported architecture' >&2; exit 1 ;;
esac
case "$os/$arch" in
  linux/amd64) digest=7422d5b5937ab917f499c9e1124927529c5027190f9bcd4c32ef88b228d4fb3b ;;
  linux/arm64) digest=0edff654241ad2fd4c16859c6c6ff2db7f602feaf15a003322399a7712721920 ;;
  darwin/amd64) digest=4a7d6423f726fadc0020d9ec1056c89c31f1268d7ed1158bf46625a3f3e53f10 ;;
  darwin/arm64) digest=efcc64f192e79be514b5bb4fc470ffff18c1c5c21617c9bae7eb9ac5e9c4e1e1 ;;
  *) echo 'rc-cli: unsupported platform' >&2; exit 1 ;;
esac
cache="${XDG_CACHE_HOME:-$HOME/.cache}/rc-demos/rc-cli/$version/$os-$arch"
archive="$cache/rc-cli.tar.gz"
binary="$cache/bin/rc"
mkdir -p "$cache"
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT INT TERM

if [ ! -f "$archive" ]; then
  curl --fail --location --proto '=https' --proto-redir '=https' --max-time 120 \
    "https://github.com/runtimeconditions/rc-cli/releases/download/$version/rc-cli_${version#v}_${os}_${arch}.tar.gz" \
    -o "$work/download.tar.gz"
  mv "$work/download.tar.gz" "$archive"
fi
if command -v sha256sum >/dev/null 2>&1; then
  actual=$(sha256sum "$archive" | awk '{print $1}')
else
  actual=$(shasum -a 256 "$archive" | awk '{print $1}')
fi
[ "$actual" = "$digest" ] || { echo 'rc-cli archive checksum mismatch' >&2; exit 1; }
mkdir -p "$work/extracted"
tar -xzf "$archive" -C "$work/extracted"
[ -f "$work/extracted/bin/rc" ] || { echo 'rc-cli archive missing bin/rc' >&2; exit 1; }
mkdir -p "$cache/bin"
cp "$work/extracted/bin/rc" "$binary"
chmod +x "$binary"
"$binary" --version >&2
printf '%s\n' "$binary"
