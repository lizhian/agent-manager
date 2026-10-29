#!/bin/bash
set -euo pipefail
cd "$(dirname "$0")/.."
if [[ "$(uname -s)" != Darwin || "$(uname -m)" != arm64 ]]; then
  echo '本轮打包仅支持 macOS ARM64。' >&2
  exit 1
fi
if [[ "$(go env GOVERSION)" != go1.27.3 ]]; then
  echo '请使用 .go-version 指定的 Go 1.27.3。' >&2
  exit 1
fi
plist=assets/macos/Info.plist
version=$(/usr/libexec/PlistBuddy -c 'Print :CFBundleShortVersionString' "$plist")
work=$(mktemp -d "${TMPDIR:-/tmp}/agent-manager-package.XXXXXX")
trap 'rm -rf "$work"' EXIT
bundle="$work/Agent Manager.app"
mkdir -p "$bundle/Contents/MacOS" "$bundle/Contents/Resources" dist
cp "$plist" "$bundle/Contents/Info.plist"
/usr/libexec/PlistBuddy -c "Add :CFBundleVersion string $version" "$bundle/Contents/Info.plist"
cp assets/macos/icon.icns "$bundle/Contents/Resources/icon.icns"
CGO_ENABLED=1 GOOS=darwin GOARCH=arm64 go build -trimpath -ldflags='-s -w' -o "$bundle/Contents/MacOS/agent-manager" ./cmd/agent-manager
codesign --force --sign - "$bundle"
codesign --verify --deep --strict "$bundle"
rm -rf 'dist/Agent Manager.app'
ditto "$bundle" 'dist/Agent Manager.app'
archive="dist/agent-manager-$version-macos-arm64.zip"
rm -f "$archive"
ditto -c -k --keepParent "$bundle" "$archive"
shasum -a 256 "$archive" > "$archive.sha256"
echo "已生成 dist/Agent Manager.app 和 ${archive}（ad hoc 签名，未公证）。"
