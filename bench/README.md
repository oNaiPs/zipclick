# Benchmark

How the numbers on the [benchmark page](https://onaips.github.io/zipclick/benchmark.html) were made, so they can be made again.

There are two sets of archives.

- **Made up**: `make-archives.sh` writes 204 files (337 MB: 200 text files that compress well, four of random bytes that don't) and packs them in seven formats: .zip, .zip with AES, .7z, .rar, .tar.gz, .tar.xz and .tar.zst. Every format holds exactly the same files, so the formats can be compared with each other.
- **Real**: `fetch-real.sh` downloads things people actually extract on Windows: Go and Node.js for Windows (.zip), the VS Code repository as GitHub serves it (.zip), FFmpeg from gyan.dev (.7z), the Python source (.tgz) and the Linux kernel source (.tar.xz).

## On Windows

The scripts expect 7-Zip, WinRAR and Bandizip installed in their default places (`winget install 7zip.7zip RARLab.WinRAR Bandisoft.Bandizip`); Windows Extract All, tar.exe and Expand-Archive come with Windows.

1. Copy `ZipClick.exe` to `C:\bench`.
2. Made-up archives: put the output of `make-archives.sh` in `C:\bench` and run `windows.ps1`. Real ones: put the output of `fetch-real.sh` in `C:\bench\real` and run `windows-real.ps1`.
3. Run them as the logged-in user, not as an administrator or a service: Explorer's Extract All needs a desktop session.

Each program extracts each archive three times; the median counts. Every run is checked afterwards against the file count and total size the archive should give, so a program that stops early does not look fast. Results land in `results.csv` next to the archives.

## On a Mac

ZipClick's extraction code also builds for macOS, which allows a comparison with each format's own command line tool:

```
./make-archives.sh
go build -o out/zipclick ..
cd out && ../mac.py
```

## The page

`page.py` writes `docs/benchmark.html` from `results-windows.json`, `results-real.json` and `results-mac.json` (medians, in seconds, taken from the runs above).
