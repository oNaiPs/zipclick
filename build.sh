#!/bin/sh
# Cross-compile zipclick.exe from any OS. Usage: ./build.sh [version]
set -e
cd "$(dirname "$0")"
v=${1:-$(git describe --tags --always --dirty 2>/dev/null || echo dev)}
# Icon, manifest and version info, embedded via rsrc_windows_*.syso.
go run github.com/tc-hib/go-winres@v0.3.3 make --in winres/winres.json --arch amd64,arm64 \
	--file-version "${v#v}" --product-version "${v#v}"
for arch in amd64 arm64; do
	GOOS=windows GOARCH=$arch go build -trimpath \
		-ldflags "-H windowsgui -s -w -X main.version=$v" \
		-o "dist/zipclick-$arch.exe" .
done
ls -l dist
