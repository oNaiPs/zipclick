#!/bin/sh
# Formatting, linting, known vulnerabilities and tests: what CI runs on every
# push. Usage: ./check.sh
# The vendored code (third_party/, internal/deflate64) is formatted but not
# linted: it is upstream's, kept as close to it as possible.
set -e
cd "$(dirname "$0")"

echo "gofmt"
unformatted=$(gofmt -l $(git ls-files '*.go' | grep -v '^third_party/'))
if [ -n "$unformatted" ]; then
	echo "not gofmt'ed:"
	echo "$unformatted"
	exit 1
fi

echo "go mod verify"
go mod verify

# The tools are built for this machine once, then pointed at each target.
tools=$(mktemp -d)
trap 'rm -rf "$tools"' EXIT
GOBIN=$tools GOOS= GOARCH= go install honnef.co/go/tools/cmd/staticcheck@v0.8.2
GOBIN=$tools GOOS= GOARCH= go install golang.org/x/vuln/cmd/govulncheck@v1.9.0

pkgs=$(go list ./... | grep -v /internal/deflate64)
for goos in windows linux; do
	echo "go vet, staticcheck ($goos)"
	GOOS=$goos go vet ./...
	GOOS=$goos "$tools/staticcheck" $pkgs
done

echo "govulncheck"
"$tools/govulncheck" ./...

echo "go test"
go test ./...
