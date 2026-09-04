#!/usr/bin/env bash
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
cd "$root"

version="${1:?usage: build_release.sh VERSION [OUT]}"
out="${2:-dist}"
rm -rf "$out"
mkdir -p "$out"

ldflags="-s -w -X github.com/thomas-huang/caosi/internal/app.Version=${version}"

build_one() {
  local goos="$1" goarch="$2" name="$3"
  echo "building $name"
  GOOS="$goos" GOARCH="$goarch" CGO_ENABLED=0 go build -trimpath -ldflags "$ldflags" -o "$out/$name" ./cmd/caosi
}

build_one darwin arm64 caosi-darwin-arm64
build_one darwin amd64 caosi-darwin-amd64
build_one linux arm64 caosi-linux-arm64
build_one linux amd64 caosi-linux-amd64
build_one windows amd64 caosi-windows-amd64.exe

assets=(
  caosi-darwin-arm64
  caosi-darwin-amd64
  caosi-linux-arm64
  caosi-linux-amd64
  caosi-windows-amd64.exe
)

if command -v sha256sum >/dev/null 2>&1; then
  (cd "$out" && sha256sum "${assets[@]}" > checksums.txt)
else
  (cd "$out" && shasum -a 256 "${assets[@]}" > checksums.txt)
fi

echo "wrote $out:"
ls -l "$out"
