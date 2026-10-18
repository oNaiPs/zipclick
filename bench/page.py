#!/usr/bin/env python3
"""Writes docs/benchmark.html from results-windows.json and results-mac.json."""
import html, json, os, re

here = os.path.dirname(os.path.abspath(__file__))
root = os.path.dirname(here)
win = json.load(open(os.path.join(here, "results-windows.json")))
mac = json.load(open(os.path.join(here, "results-mac.json")))
index = open(os.path.join(root, "docs", "index.html")).read()
style = re.search(r"<style>(.*?)</style>", index, re.S).group(1)

formats = [
    ("a.zip", ".zip"),
    ("aes.zip", ".zip with a password (AES)"),
    ("a.7z", ".7z"),
    ("a.rar", ".rar"),
    ("a.tar.gz", ".tar.gz"),
    ("a.tar.xz", ".tar.xz"),
    ("a.tar.zst", ".tar.zst"),
]
names = {"Explorer": "Windows Extract All", "Expand-Archive": "PowerShell Expand-Archive", "tar.exe": "Windows tar.exe"}


def table(rows):
    rows = sorted(rows, key=lambda r: r[1])
    top = max(s for _, s in rows)
    out = ['<table class="bench">']
    for name, s in rows:
        cls = ' class="me"' if name == "ZipClick" else ""
        w = max(1.0, 100 * s / top)
        out.append(
            f'<tr{cls}><th>{html.escape(name)}</th>'
            f'<td class="bar"><span style="width:{w:.1f}%"></span></td>'
            f'<td class="num">{s:.1f} s</td></tr>'
        )
    out.append("</table>")
    return "\n".join(out)


sections = []
for key, label in formats:
    rows = [(names.get(t, t), v["median"]) for t, v in win[key].items() if v["complete"]]
    sections.append(f"<h3>{html.escape(label)}</h3>\n{table(rows)}")
mac_sections = []
for key, label in formats:
    rows = list(mac[key].items())
    mac_sections.append(f"<h3>{html.escape(label)}</h3>\n{table(rows)}")

page = f"""<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>ZipClick: how fast it is</title>
<meta name="description" content="ZipClick against Windows Extract All, 7-Zip, WinRAR, Bandizip and tar.exe, extracting the same 337 MB in seven archive formats.">
<link rel="icon" href="img/favicon-32.png">
<style>{style}
  h3 {{ font-size: 1rem; margin: 28px 0 8px; }}
  table.bench {{ width: 100%; border-collapse: collapse; font-size: 0.95rem; }}
  table.bench th {{ text-align: left; font-weight: 400; width: 13.5em; padding: 3px 10px 3px 0; white-space: nowrap; }}
  table.bench td.bar {{ padding: 3px 0; }}
  table.bench td.bar span {{ display: block; height: 14px; border-radius: 3px; background: var(--rule); }}
  table.bench td.num {{ text-align: right; width: 4.5em; padding-left: 10px; font-variant-numeric: tabular-nums; white-space: nowrap; }}
  table.bench tr.me th, table.bench tr.me td.num {{ font-weight: 600; }}
  table.bench tr.me td.bar span {{ background: var(--accent); }}
  @media (max-width: 480px) {{ table.bench th {{ width: 9em; white-space: normal; }} }}
  ul {{ padding-left: 1.3em; }}
</style>
</head>
<body>
<main>

<h1><a href="./" style="display:flex;align-items:center;gap:10px;color:inherit;text-decoration:none"><img src="img/icon-64.png" alt="">ZipClick</a></h1>

<p class="lede">How fast ZipClick is</p>

<p>The same 337 MB of files, packed in seven archive formats, extracted by ZipClick and by the tools people already use on Windows. Shorter bars are faster.</p>

<p>ZipClick is faster than Windows' own Extract All for .zip files, which is what most people get, but the dedicated archivers are faster still: 7-Zip, WinRAR, Bandizip and Windows' tar.exe all beat it. For .rar and .tar.xz it is the slowest of the group, and for .7z only WinRAR is slower. The reason is mostly the decoders: ZipClick's are written in Go and work on one file and one core at a time, where the others use optimised C and assembly, often on several cores. For 1 GB of files in a .7z that means about 18 seconds instead of 3.</p>

<h2>On Windows</h2>
{chr(10).join(sections)}

<h2>How it was measured</h2>
<ul>
  <li>The files: 204 files, 337 MB in total. 200 text files of about 1 MB, which compress well, and four 25 MB files of random data, which don't, like photos or video.</li>
  <li>The computer: Windows 11 25H2 in a virtual machine with 8 cores of an Intel Core i5-12600H and 6 GB of memory, with Microsoft Defender on.</li>
  <li>The programs: ZipClick 0.4.1, 7-Zip 26.04, WinRAR 7.23, Bandizip 7.47, and the tar.exe, Extract All and Expand-Archive that come with Windows 11.</li>
  <li>Each program extracted each archive three times and the table shows the middle time, from start until the last file is written. Every run was checked afterwards: all 204 files, all 337,415,044 bytes.</li>
  <li>Windows Extract All was measured through the same Shell copy the menu item uses. WinRAR ran its background extraction, as from its context menu.</li>
  <li>7-Zip decodes .7z and .xz on several cores when the archive was written in blocks, as these were.</li>
  <li>Bandizip only unpacked the .tar from inside the .tar.xz and .tar.zst, so it is left out of those two.</li>
  <li>Measured twice on 18 October 2026, in two sessions. The second is shown: every program was faster in it, some by half, which is how much a virtual machine's disk cache and Defender's scanning can move these numbers. The order of the programs barely changed.</li>
</ul>

<h2>On a Mac</h2>
<p>ZipClick is a Windows program, but its extraction code also runs on a Mac, which makes a cleaner comparison with each format's own command line tool: an Apple M3 Pro, the same archives, five runs each.</p>
{chr(10).join(mac_sections)}

<p>The scripts that make the archives and run the tests, and the raw numbers, are in the <a href="https://github.com/oNaiPs/zipclick/tree/main/bench">bench folder</a> of the source.</p>

<footer>
  <a href="./">Back to ZipClick</a>. The <a href="https://github.com/oNaiPs/zipclick">source is on GitHub</a>.
</footer>

</main>
</body>
</html>
"""
open(os.path.join(root, "docs", "benchmark.html"), "w").write(page)
