# ZipClick

ZipClick makes double-clicking a .zip file on Windows extract it, the way it works on a Mac.

![Double-clicking three zip files in Explorer. Each one is extracted next to itself and the result is selected. The large one shows a progress window first.](docs/demo.gif)

On Windows, double-clicking a zip opens it like a folder. To get the files out you right-click, choose Extract All, confirm where they should go, and then deal with the second window that opens. With ZipClick the double-click puts the files next to the zip and you carry on.

Website: https://onaips.github.io/zipclick/

## Install

Download [ZipClick.exe](https://github.com/oNaiPs/zipclick/releases/latest/download/ZipClick.exe) and run it ([ARM64 build](https://github.com/oNaiPs/zipclick/releases/latest/download/ZipClick-arm64.exe) for Snapdragon PCs). It installs for your user only, without admin rights, and then opens the Default apps page in Settings, where you pick ZipClick for .zip files. Windows doesn't let a program make itself the default, so that one click has to be yours.

The exe isn't code-signed yet, so SmartScreen warns the first time you run it. Click More info, then Run anyway.

To remove it, go to Settings > Apps > Installed apps, find ZipClick and choose Uninstall.

## What happens when you double-click

If the zip holds a single folder or file, that item appears next to the zip. If it holds several things, they go into a new folder named after the zip. Nothing is ever overwritten: if the name is taken you get "Photos 2", then "Photos 3".

The new item is selected in the Explorer window you were using. A big archive shows a small progress window with a Cancel button. Cancelling leaves nothing behind, because ZipClick extracts into a hidden folder and only renames it into place once everything is written.

Zips made on a Mac come without the `__MACOSX` folder and `.DS_Store` files. Zips from older Windows versions, which store names in the old DOS code page, come out with their accents intact.

The dialogs follow your Windows display language. ZipClick speaks English, Portuguese (Portugal and Brazil), Spanish, French, German and Italian.

## Limits

It only handles .zip files, not .rar, .7z or .tar.gz.

Password-protected zips, and zips compressed with Deflate64 (which Windows itself uses for very large files), can't be extracted. ZipClick says so and suggests Extract All, which handles both.

You can still open a zip as a folder: right-click it, then Open with > File Explorer.

## Command line

```
zipclick FILE.zip...   extract
zipclick install       install for this user
zipclick uninstall     remove
zipclick status        show where .zip double-clicks go right now
--quiet                no dialogs, messages go to stdout and stderr
```

Set `ZIPCLICK_LANG` (for example `pt-PT`) to force a language.

## Building

You need Go. It cross-compiles from any OS.

```
go test ./...
./build.sh 0.3.1    # writes dist/zipclick-amd64.exe and dist/zipclick-arm64.exe
```

The icon, manifest and version info live in `winres/`. `build.sh` regenerates `rsrc_windows_*.syso` from them with go-winres, and `go build` embeds those.

I test releases on Windows 11 25H2 (x64), clicking through the install, the Settings step, every kind of zip in the test suite, upgrade and uninstall.
