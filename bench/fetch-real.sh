#!/bin/sh
# Downloads the real-world archives for windows-real.ps1 into ./real: what
# people actually download and extract on Windows. Versions as measured on
# 18 October 2026; newer ones work the same way.
set -eu
cd "$(dirname "$0")"
mkdir -p real
cd real
curl -fL -o go-windows.zip https://dl.google.com/go/go1.27.2.windows-amd64.zip
curl -fL -o node-win-x64.zip https://nodejs.org/dist/v24.22.0/node-v24.22.0-win-x64.zip
curl -fL -o vscode-main.zip https://github.com/microsoft/vscode/archive/refs/heads/main.zip
curl -fL -o ffmpeg-essentials.7z https://www.gyan.dev/ffmpeg/builds/ffmpeg-release-essentials.7z
curl -fL -o python-src.tgz https://www.python.org/ftp/python/3.15.0/Python-3.15.0.tgz
curl -fL -o linux-7.2.11.tar.xz https://cdn.kernel.org/pub/linux/kernel/v7.x/linux-7.2.11.tar.xz
ls -l
