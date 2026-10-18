#!/usr/bin/env python3
"""Times ZipClick against each format's own command line tool on a Mac.

Run from bench/out (made by make-archives.sh) with a zipclick binary built
for the Mac next to it:
    go build -o bench/out/zipclick . && cd bench/out && ../mac.py
Needs 7z, rar, gzip, xz and zstd. Five runs each; the median is printed and
written to results-mac-raw.json.
"""
import json, os, statistics, subprocess, time

refs = {
    "a.zip": ("7-Zip", "7z x -o{o} a.zip"),
    "aes.zip": ("7-Zip", "7z x -px -o{o} aes.zip"),
    "a.7z": ("7-Zip", "7z x -o{o} a.7z"),
    "a.rar": ("rar", "rar x -idq a.rar {o}/"),
    "a.tar.gz": ("gzip + tar", "gzip -dc a.tar.gz | tar -xf - -C {o}"),
    "a.tar.xz": ("xz (1 thread) + tar", "xz -dc -T1 a.tar.xz | tar -xf - -C {o}"),
    "a.tar.zst": ("zstd + tar", "zstd -dc a.tar.zst | tar -xf - -C {o}"),
}
counter = 0


def run(cmd):
    start = time.time()
    subprocess.run(cmd, shell=True, check=True, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
    return time.time() - start


def fresh(archive):
    global counter
    counter += 1
    d = f"run{counter}"
    os.mkdir(d)
    os.link(archive, f"{d}/{archive}")
    return d


out = {}
for archive, (name, cmd) in refs.items():
    pw = "--password=x" if archive == "aes.zip" else ""
    zc, ref = [], []
    for _ in range(5):
        d = fresh(archive)
        zc.append(run(f"./zipclick --quiet {pw} {d}/{archive}"))
        d = fresh(archive)
        os.mkdir(f"{d}/out")
        ref.append(run(cmd.format(o=f"{d}/out")))
    out[archive] = {"ZipClick": round(statistics.median(zc), 2), name: round(statistics.median(ref), 2)}
    print(archive, out[archive], flush=True)
    subprocess.run("rm -rf run*", shell=True)
json.dump(out, open("results-mac-raw.json", "w"), indent=1)
