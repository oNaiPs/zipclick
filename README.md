# ZipClick

Double-click a `.zip` on Windows and it is extracted — the way Archive Utility
does it on macOS — instead of being opened as a folder.

- **One item inside** (a folder or a file) → that item appears next to the zip.
- **Several items** → a folder named after the zip, next to it.
- **Never overwrites**: `name 2`, `name 3`… if the name is taken.
- The result is **selected** in the Explorer window you double-clicked in.
- Anything that takes more than a moment gets a **progress window** with the
  size, time left and Cancel — which leaves nothing behind.
- `__MACOSX/` and `.DS_Store` from Mac-made zips are dropped; names are decoded
  from the OEM code page when the zip didn't use UTF-8, and macOS's decomposed
  accents are normalised.
- Extraction goes into a hidden staging folder and is renamed into place, so a
  failure never leaves a half-extracted result. Traversal (`../`), absolute
  paths and names Windows can't hold (`CON`, `a:b`, trailing dots) are made safe.

## Install

Run `zipclick.exe` and say yes. It copies itself to
`%LOCALAPPDATA%\Programs\ZipClick`, registers for `.zip` (per user, no admin),
and opens **Settings → Default apps → ZipClick**: click `.zip` and pick
ZipClick. Windows does not let any program make itself the default, so that one
click is yours.

Until then, every `.zip` already has **Extract here (ZipClick)** in its
right-click menu (under *Show more options* on Windows 11).

To open a zip as a folder afterwards: right-click → *Open with* → *Windows Explorer*.

Uninstall from *Settings → Apps → Installed apps*, or `zipclick uninstall`.

## Command line

```
zipclick FILE.zip...   extract
zipclick install       install for this user
zipclick uninstall     remove
zipclick status        where .zip double-clicks go right now
--quiet                no dialogs; messages on stdout/stderr
```

## Limits

- Zip only. Password-protected zips and Deflate64 (which Explorer uses for very
  large files) aren't supported — ZipClick says so and points to *Open with →
  Windows Explorer*, whose *Extract All…* handles both.

## Tested

Windows 11 25H2 (x64): install from Downloads, Settings deep link, choosing the
default, double-click in Explorer for every fixture shape (several items, one
folder, one file, Mac-made, OEM code-page names, traversal/illegal names,
encrypted), *Open with → Windows Explorer*, upgrade in place, uninstall and
reinstall. 25H2 keeps the default in `UserChoiceLatest\ProgId`, which
`status` and `install` read first.

## Build

```
go test ./...
./build.sh 0.1.0  # dist/zipclick-amd64.exe, dist/zipclick-arm64.exe
```

The icon, manifest and version info live in `winres/` and are embedded through
`rsrc_windows_*.syso`, which `build.sh` regenerates (go-winres).
