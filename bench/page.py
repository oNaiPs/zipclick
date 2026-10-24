#!/usr/bin/env python3
"""Writes docs/benchmark.html from the raw results in results/ and
results-mac.json.

results/windows-<set>-defender-<on|off>.csv are what windows.ps1 (set
"windows") and windows-real.ps1 (set "windows-real") write, copied as they
are. The medians and the completeness check are worked out here.
"""
import csv, html, json, os, re, statistics

here = os.path.dirname(os.path.abspath(__file__))
root = os.path.dirname(here)
index = open(os.path.join(root, "docs", "index.html")).read()
style = re.search(r"<style>(.*?)</style>", index, re.S).group(1)

# What every program must have written for the synthetic archives.
SYNTHETIC = (204, 337415044)


def load(name):
    """{archive: {tool: (median seconds or None, complete)}} and the header."""
    path = os.path.join(here, "results", name)
    if not os.path.exists(path):
        return {}, {}
    meta, rows = {}, []
    for line in open(path):
        line = line.strip()
        if line.startswith("#"):
            k, _, v = line[1:].partition(":")
            meta[k.strip()] = v.strip()
        elif line and line != "DONE":
            rows.append(line)
    runs, want = {}, {}
    for r in csv.DictReader(rows):
        a = r.get("archive") or r["format"]
        files, size = int(r["files"]), int(r["bytes"] or 0)
        if r["tool"] == "reference":
            want[a] = (files, size)
            continue
        runs.setdefault(a, {}).setdefault(r["tool"], []).append((float(r["seconds"]), files, size))
    out = {}
    for a, tools in runs.items():
        wf, wb = want.get(a, SYNTHETIC)
        for tool, rs in tools.items():
            # Close to what 7-Zip writes: the programs disagree only on how to
            # write symbolic links (the kernel has about a hundred, a few bytes
            # each), while one that stops early is off by far more.
            done = all(s >= 0 and abs(f - wf) <= wf / 100 and abs(b - wb) <= wb / 1000 for s, f, b in rs)
            timed = [s for s, _, _ in rs if s >= 0]
            out.setdefault(a, {})[tool] = (statistics.median(timed) if timed else None, done)
    return out, meta


names = {"Explorer": "Windows Extract All", "Expand-Archive": "PowerShell Expand-Archive", "tar.exe": "Windows tar.exe"}


def table(on, off):
    """Two bars per program: Defender on above, off below."""
    tools = list(dict.fromkeys(list(on) + list(off)))

    def key(t):
        s, ok = on.get(t, off.get(t, (None, False)))
        return (not ok, s if s is not None else 1e9)

    tools.sort(key=key)
    top = max([s for d in (on, off) for s, ok in d.values() if ok and s is not None] or [1])

    def cell(d, t):
        s, ok = d.get(t, (None, False))
        if t not in d:
            return "", ""
        if not ok:
            label = "over 5 min" if s is None else "didn't finish"
            return "", f'<span class="miss">{label}</span>'
        w = max(0.6, 100 * s / top)
        return f'<span class="bar" style="width:{w:.1f}%"></span>', f"{s:.1f} s"

    out = ['<table class="bench">']
    for t in tools:
        cls = ' class="me"' if t == "ZipClick" else ""
        bon, non = cell(on, t)
        boff, noff = cell(off, t)
        out.append(
            f'<tr{cls}><th>{html.escape(names.get(t, t))}</th>'
            f'<td class="bars"><div class="on">{bon}</div><div class="off">{boff}</div></td>'
            f'<td class="num"><div>{non}</div><div class="dim">{noff}</div></td></tr>'
        )
    out.append("</table>")
    return "\n".join(out)


real_on, meta_on = load("windows-real-defender-on.csv")
real_off, meta_off = load("windows-real-defender-off.csv")
syn_on, _ = load("windows-defender-on.csv")
syn_off, _ = load("windows-defender-off.csv")
mac = json.load(open(os.path.join(here, "results-mac.json")))

real = [
    ("go-windows.zip", "Go 1.27.2 for Windows", ".zip, 15,657 files, 247 MB"),
    ("node-win-x64.zip", "Node.js 24.22.0 for Windows", ".zip, 1,994 files, 107 MB"),
    ("vscode-main.zip", "The VS Code source, as GitHub serves it", ".zip, 20,792 files, 310 MB"),
    ("python-src.tgz", "The Python 3.15.0 source", ".tgz, 5,434 files, 138 MB"),
    ("ffmpeg-essentials.7z", "FFmpeg for Windows, from gyan.dev", ".7z, 45 files, 329 MB"),
    ("linux-7.2.11.tar.xz", "The Linux 7.2.11 source", ".tar.xz, 94,861 files, 1.6 GB"),
]
formats = [
    ("a.zip", ".zip"),
    ("aes.zip", ".zip with a password (AES)"),
    ("a.7z", ".7z"),
    ("a.rar", ".rar"),
    ("a.tar.gz", ".tar.gz"),
    ("a.tar.xz", ".tar.xz"),
    ("a.tar.zst", ".tar.zst"),
]

real_sections = [
    f'<h3>{html.escape(title)} <span class="dim">{html.escape(what)}</span></h3>\n{table(real_on.get(k, {}), real_off.get(k, {}))}'
    for k, title, what in real
]
syn_sections = [f"<h3>{html.escape(label)}</h3>\n{table(syn_on.get(k, {}), syn_off.get(k, {}))}" for k, label in formats]


def mac_table(rows):
    rows = sorted(rows, key=lambda r: r[1])
    top = max(s for _, s in rows)
    out = ['<table class="bench">']
    for name, s in rows:
        cls = ' class="me"' if name == "ZipClick" else ""
        out.append(
            f'<tr{cls}><th>{html.escape(name)}</th>'
            f'<td class="bars"><div class="on"><span class="bar" style="width:{max(0.6, 100 * s / top):.1f}%"></span></div></td>'
            f'<td class="num"><div>{s:.1f} s</div></td></tr>'
        )
    out.append("</table>")
    return "\n".join(out)


mac_sections = [f"<h3>{html.escape(label)}</h3>\n{mac_table(list(mac[k].items()))}" for k, label in formats]
version = meta_on.get("zipclick") or meta_off.get("zipclick", "")

page = f"""<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>ZipClick: how fast it is</title>
<meta name="description" content="ZipClick against Windows Extract All, 7-Zip, WinRAR, Bandizip and tar.exe, on archives people download and on the same files in seven formats, with Microsoft Defender on and off.">
<link rel="icon" href="img/favicon-32.png">
<style>{style}
  h3 {{ font-size: 1rem; margin: 28px 0 8px; }}
  h3 .dim {{ font-weight: 400; }}
  .dim {{ color: var(--muted, #777); }}
  table.bench {{ width: 100%; border-collapse: collapse; font-size: 0.95rem; }}
  table.bench th {{ text-align: left; font-weight: 400; width: 13.5em; padding: 4px 10px 4px 0; white-space: nowrap; vertical-align: middle; }}
  table.bench td.bars {{ padding: 4px 0; }}
  table.bench td.bars div {{ height: 9px; margin: 2px 0; }}
  table.bench .bar {{ display: block; height: 100%; border-radius: 2px; background: var(--rule); }}
  table.bench .off .bar {{ opacity: 0.45; }}
  table.bench td.num {{ text-align: right; width: 6.5em; padding-left: 10px; font-variant-numeric: tabular-nums; white-space: nowrap; font-size: 0.85rem; line-height: 1.15; }}
  table.bench tr.me th, table.bench tr.me td.num div:first-child {{ font-weight: 600; }}
  table.bench tr.me .bar {{ background: var(--accent); }}
  table.bench .miss {{ font-style: italic; }}
  .key {{ display: flex; gap: 18px; flex-wrap: wrap; font-size: 0.9rem; margin: 10px 0 0; }}
  .key span::before {{ content: ""; display: inline-block; width: 22px; height: 9px; border-radius: 2px; background: var(--accent); margin-right: 6px; vertical-align: middle; }}
  .key .k-off::before {{ opacity: 0.45; }}
  @media (max-width: 480px) {{ table.bench th {{ width: 8.5em; white-space: normal; }} }}
  ul {{ padding-left: 1.3em; }}
</style>
</head>
<body>
<main>

<h1><a href="./" style="display:flex;align-items:center;gap:10px;color:inherit;text-decoration:none"><img src="img/icon-64.png" alt="">ZipClick</a></h1>

<p class="lede">How fast ZipClick is</p>

<p>Two things decide how long an archive takes to extract on Windows: how fast the program decodes it, and how long Microsoft Defender spends checking each file the program writes. The second one often matters more, and it is not the same for every program.</p>

<p>On its own, ZipClick is the fastest of the programs here at .zip, the format most people get, and keeps up with them on .tar.gz and .tar.zst. It is slower at .7z, .tar.xz and .rar, whose decoders in ZipClick are written in Go, where 7-Zip's are optimised C and use several cores.</p>

<p>With Defender on, as Windows comes, an archive with thousands of small files takes several times longer with ZipClick than with 7-Zip, WinRAR or tar.exe: the Linux source, 95,000 files, takes five minutes instead of one. Defender checks every file ZipClick writes much more thoroughly than the files those programs write. Archives with a few big files, like the FFmpeg one, barely notice. <a href="#defender">More about that below.</a></p>

<div class="key"><span class="k-on">Defender on (how Windows comes)</span><span class="k-off">Defender off</span></div>

<h2>Archives people download</h2>
{chr(10).join(real_sections)}

<h2>The same files in seven formats</h2>
<p>204 files, 337 MB, packed in each format, so the formats can be compared with each other.</p>
{chr(10).join(syn_sections)}

<h2 id="defender">Microsoft Defender</h2>
<p>Defender's real-time protection checks files as programs write them. Windows' own performance recorder, run while each program extracted the Go archive, showed the difference: Defender spent 77 to 81 seconds checking the 15,000 files ZipClick wrote, 4.5 seconds on 7-Zip's, and checked only 54 of tar.exe's. Those seconds are spread over several cores, which is why they can add up to more than the time the extraction took.</p>
<p>Defender doesn't say why it treats programs differently. Being well known seems to count, being signed doesn't seem to be enough on its own: Bandizip is signed, and Defender slows it down about as much as ZipClick. ZipClick isn't <a href="code-signing.html">code-signed</a> yet, and how much a signature would change this hasn't been measured.</p>
<p>If you extract big source trees or SDKs often and know what that means for your security, a Defender exclusion for the ZipClick process gives you the "Defender off" times above. The files are still checked when they are opened or run. Turning real-time protection off is not something to do for this.</p>

<h2>How it was measured</h2>
<ul>
  <li>The computer: Windows 11 25H2 in a virtual machine with 8 cores of an Intel Core i5-12600H and 6 GB of memory.</li>
  <li>The programs: ZipClick {html.escape(version)}, the code as it is after 0.4.1, 7-Zip 26.04, WinRAR 7.23, Bandizip 7.47, and the tar.exe, Extract All and Expand-Archive that come with Windows 11.</li>
  <li>Each program extracted each archive three times and the tables show the middle time, from start until the last file is written. Every run was checked afterwards against what 7-Zip extracts from the same archive. A run that wrote less is shown as "didn't finish": Bandizip only unpacked the .tar from inside the .tar.xz and .tar.zst. Windows Extract All was stopped after 5 minutes.</li>
  <li>Windows Extract All was measured through the same Shell copy the menu item uses. WinRAR ran its background extraction, as from its context menu.</li>
  <li>The archives were not marked as downloaded from the internet. When one is, ZipClick and Explorer also copy that mark to every file they extract.</li>
  <li>Measured on 18 and 19 October 2026, first with Defender's real-time protection off, then on. A virtual machine's disk cache and Defender move these numbers from one session to the next, sometimes by half, but the order of the programs stays about the same.</li>
</ul>

<h2>On a Mac</h2>
<p>ZipClick is a Windows program, but its extraction code also runs on a Mac, with no antivirus in the way, which makes a clean comparison with each format's own command line tool: an Apple M3 Pro, the same seven archives, five runs each.</p>
{chr(10).join(mac_sections)}

<p>The scripts that make and fetch the archives and run the tests, and the raw numbers, are in the <a href="https://github.com/oNaiPs/zipclick/tree/main/bench">bench folder</a> of the source.</p>

<footer>
  <a href="./">Back to ZipClick</a>. The <a href="https://github.com/oNaiPs/zipclick">source is on GitHub</a>.
</footer>

</main>
</body>
</html>
"""
open(os.path.join(root, "docs", "benchmark.html"), "w").write(page)
