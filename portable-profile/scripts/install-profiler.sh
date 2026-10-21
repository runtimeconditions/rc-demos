#!/bin/sh
# Official release binaries include the approved core schema; plain go install does not.
set -eu
version=v0.6.0
os=$(uname -s | tr '[:upper:]' '[:lower:]')
case $(uname -m) in x86_64) arch=amd64 ;; arm64|aarch64) arch=arm64 ;; *) echo 'unsupported architecture' >&2; exit 1 ;; esac
case "$os/$arch" in
  linux/amd64) digest=e35ef96832608445d0651118551e563c6ce2d19a8c239d172c301bd759bd5b75 ;;
  linux/arm64) digest=7707f08046f6d0c95f19f6b308e3adfd1a48c94ae11059ce29374a68a3be7dac ;;
  darwin/amd64) digest=1b1dcd4a3c441dba32996eab91a7414f715c11789c18f739c7054e1fa5f331c3 ;;
  darwin/arm64) digest=d82a66ef901e39695d4e563cee9cb61bb43a7917e6c7b94eacc35424dbff1e78 ;;
  *) echo 'unsupported platform; see profiler release downloads' >&2; exit 1 ;;
esac
cache="${XDG_CACHE_HOME:-$HOME/.cache}/rc-demos/profiler/$version/$os-$arch"
mkdir -p "$cache"
# Recheck the pinned archive on every invocation, including cache hits.
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT INT TERM
archive="$cache/release.tar.gz"
if [ ! -f "$archive" ]; then
  curl --fail --location --proto '=https' --proto-redir '=https' --max-time 120 \
    "https://github.com/runtimeconditions/go-rc-profiler/releases/download/$version/go-rc-profiler_${version}_${os}_${arch}.tar.gz" -o "$work/release.tar.gz"
  cp "$work/release.tar.gz" "$archive"
fi
if command -v sha256sum >/dev/null 2>&1; then
  actual=$(sha256sum "$archive" | awk '{print $1}')
else
  actual=$(shasum -a 256 "$archive" | awk '{print $1}')
fi
[ "$actual" = "$digest" ] || { echo 'profiler release archive checksum mismatch' >&2; exit 1; }
tar --no-same-owner -xzf "$archive" -C "$work"
cp "$work/go-rc-profiler" "$cache/go-rc-profiler"
chmod +x "$cache/go-rc-profiler"
"$cache/go-rc-profiler" --version >&2
printf '%s\n' "$cache/go-rc-profiler"
