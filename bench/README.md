# Benchmark

How the numbers on the [benchmark page](https://onaips.github.io/zipclick/benchmark.html) were made, so they can be made again.

There are two sets of archives.

- **Made up**: `make-archives.sh` writes 204 files (337 MB: 200 text files that compress well, four of random bytes that don't) and packs them in seven formats: .zip, .zip with AES, .7z, .rar, .tar.gz, .tar.xz and .tar.zst. Every format holds exactly the same files, so the formats can be compared with each other.
- **Real**: `fetch-real.sh` downloads things people actually extract on Windows: Go and Node.js for Windows (.zip), the VS Code repository as GitHub serves it (.zip), FFmpeg from gyan.dev (.7z), the Python source (.tgz) and the Linux kernel source (.tar.xz).

## On Windows

The scripts expect 7-Zip, WinRAR and Bandizip installed in their default places (`winget install 7zip.7zip RARLab.WinRAR Bandisoft.Bandizip`); Windows Extract All, tar.exe and Expand-Archive come with Windows.

1. Copy `ZipClick.exe` (from `../build.sh`) to `C:\bench`.
2. Made-up archives: put the output of `make-archives.sh` in `C:\bench` and run `windows.ps1`. Real ones: put the output of `fetch-real.sh` in `C:\bench\real` and run `windows-real.ps1`.
3. Run them as the logged-in user, not as an administrator or a service: Explorer's Extract All needs a desktop session.

Each program extracts each archive three times; the median counts. Results land in `results.csv` next to the archives. Its first lines say which ZipClick was measured and whether Defender's real-time protection was on.

**Microsoft Defender changes these numbers more than anything else.** It checks every file a program writes, and it checks ZipClick's much more than 7-Zip's, WinRAR's or tar.exe's, so an archive of thousands of small files can take several times longer with it on. The page shows both: run everything once with real-time protection on, as Windows comes, and once with it off, which leaves only the programs' own speed. To see where Defender's time goes, wrap a run in `New-MpPerformanceRecording` and read it with `Get-MpPerformanceReport -TopProcesses 5`.

A run counts only if it wrote what 7-Zip extracts from the same archive, to within 1% of the files and 0.1% of the bytes. The tolerance is for symbolic links, which each program writes differently (the kernel has about a hundred); a program that stops early, like Bandizip unpacking only the .tar from a .tar.xz, is off by far more. Explorer's Extract All is given five minutes.

## On a Mac

ZipClick's extraction code also builds for macOS, which allows a comparison with each format's own command line tool, without an antivirus in the way:

```
./make-archives.sh
go build -o out/zipclick ..
cd out && ../mac.py
```

`mac.py` writes `results-mac-raw.json`; copy it to `results-mac.json`.

## The page

`page.py` writes `docs/benchmark.html` from the raw Windows results, copied unchanged into `results/` (`windows-defender-on.csv`, `windows-defender-off.csv`, `windows-real-defender-on.csv`, `windows-real-defender-off.csv`), and from `results-mac.json`. It works out the medians and which runs were complete itself.
