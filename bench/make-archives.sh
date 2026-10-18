#!/bin/sh
# Builds the benchmark data set and the same data in every format.
# Needs python3, 7z (p7zip), rar, gzip, xz and zstd. Output goes to ./out.
#   200 text files of about 1 MB (random dictionary words) and four 25 MiB
#   files of random bytes, like photos or video: 204 files, 337 MB.
set -eu
cd "$(dirname "$0")"
mkdir -p out/src
cd out/src
python3 - <<'PY'
import os, random
random.seed(1)
words = open('/usr/share/dict/words').read().split()
os.makedirs('data/text', exist_ok=True)
os.makedirs('data/bin', exist_ok=True)
for i in range(200):
    with open(f'data/text/f{i}.txt', 'w') as f:
        f.write(' '.join(random.choices(words, k=110000)))
for i in range(4):
    with open(f'data/bin/b{i}.bin', 'wb') as f:
        f.write(os.urandom(25 << 20))
PY
7z a -bd -t7z -mx=5 ../a.7z data >/dev/null
7z a -bd -tzip -mx=5 ../a.zip data >/dev/null
7z a -bd -tzip -px -mem=AES256 ../aes.zip data >/dev/null
rar a -idq -ma5 -m3 ../a.rar data
tar -cf - data | gzip -6 >../a.tar.gz
tar -cf - data | xz -6 -T0 >../a.tar.xz
tar -cf - data | zstd -q -6 -T0 >../a.tar.zst
ls -l ..
