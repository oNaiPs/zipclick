#!/bin/sh
# Regenerates the archives the tests read. Needs 7z (p7zip), rar 7, bsdtar or
# GNU tar, gzip, bzip2, xz and zstd. rar is unfree; with Nix:
#   NIXPKGS_ALLOW_UNFREE=1 nix shell --impure nixpkgs#rar -c sh testdata/make.sh
# Every archive holds the same tree, a single top-level folder:
#   album/one.txt  album/sub/two.txt  album/sub/lines.txt
# lines.txt is big enough that every method actually compresses it.
# rar 7 no longer writes RAR 4, so only RAR 5 is here.
set -eu
cd "$(dirname "$0")"
pw='sésamo'
aespw='open sesame' # p7zip refuses non-ASCII passwords for zip
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
mkdir -p "$work/album/sub"
printf 'one\n' >"$work/album/one.txt"
printf 'two\n' >"$work/album/sub/two.txt"
seq 1 5000 | sed 's/$/ green bottles/' >"$work/album/sub/lines.txt"
printf 'just some notes\n' >"$work/notes.txt"
touch -t 202001020304.05 "$work/album/one.txt" "$work/album/sub/two.txt" "$work/album/sub/lines.txt" "$work/notes.txt"
here=$PWD
rm -f album* notes* deflate64*

(cd "$work" && 7z a -bd -t7z "$here/album.7z" album >/dev/null)
(cd "$work" && 7z a -bd -t7z -p"$pw" "$here/album-pw.7z" album >/dev/null)
(cd "$work" && 7z a -bd -t7z -p"$pw" -mhe=on "$here/album-pwhead.7z" album >/dev/null)

(cd "$work" && rar a -idq -ma5 "$here/album.rar" album)
(cd "$work" && rar a -idq -ma5 -s "$here/album-solid.rar" album)
(cd "$work" && rar a -idq -ma5 -p"$pw" "$here/album-pw.rar" album)
(cd "$work" && rar a -idq -ma5 -hp"$pw" "$here/album-pwhead.rar" album)

(cd "$work" && tar -cf "$here/album.tar" album)
gzip -9n <album.tar >album.tar.gz
cp album.tar.gz album.tgz
bzip2 -9 <album.tar >album.tar.bz2
xz -9 <album.tar >album.tar.xz
zstd -q -19 <album.tar >album.tar.zst
gzip -9 <"$work/notes.txt" >notes.txt.gz
xz <"$work/notes.txt" >notes.txt.xz

(cd "$work" && zip -qr -P "$pw" "$here/album-zipcrypto.zip" album)
(cd "$work" && 7z a -bd -tzip -p"$aespw" -mem=AES256 "$here/album-aes.zip" album >/dev/null)
(cd "$work" && 7z a -bd -tzip -p"$aespw" -mem=AES128 -mm=BZip2 "$here/album-aes128-bzip2.zip" album >/dev/null)
(cd "$work" && 7z a -bd -tzip -mm=BZip2 "$here/album-bzip2.zip" album >/dev/null)
(cd "$work" && 7z a -bd -tzip -mm=LZMA "$here/album-lzma.zip" album >/dev/null)
(cd "$work" && 7z a -bd -tzip -mm=Deflate64 "$here/album-deflate64.zip" album >/dev/null)

# Deflate64's own territory: a 40 KB random block twice (distances past
# 32 KiB) and a long run of zeros (lengths past 258).
head -c 40000 /dev/urandom >"$work/r"
cat "$work/r" "$work/r" >"$work/big.bin"
head -c 300000 /dev/zero >>"$work/big.bin"
(cd "$work" && 7z a -bd -tzip -mm=Deflate64 -mx=9 "$here/deflate64-stress.zip" big.bin >/dev/null)
ls -l
