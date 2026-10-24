# Real archives (fetch-real.sh) on Windows: each tool three times, the median
# counts. 7-Zip's output for each archive is the reference for "complete".
# Put the archives in C:\bench\real and ZipClick.exe in C:\bench, run as the
# desktop user (Explorer needs a session); results in C:\bench\real\results.csv.
$ErrorActionPreference = 'Continue'
[Threading.Thread]::CurrentThread.CurrentCulture = 'en-US'
$B = 'C:\bench\real'
$out = "$B\results.csv"
$sz = 'C:\Program Files\7-Zip\7z.exe'
$wr = 'C:\Program Files\WinRAR\WinRAR.exe'
$bz = 'C:\Program Files\Bandizip\bz.exe'
# Defender scans every file an unknown program writes, which moves these
# numbers more than anything else: say which way it was.
"# defender realtime: $((Get-MpComputerStatus).RealTimeProtectionEnabled)" | Set-Content $out
"# zipclick: $((Get-Item "C:\bench\ZipClick.exe").VersionInfo.ProductVersion)" | Add-Content $out
"archive,tool,run,seconds,files,bytes" | Add-Content $out

function Measure-Tree($d) {
  $f = Get-ChildItem -Recurse -File -Force $d -ErrorAction SilentlyContinue
  return @($f.Count, [int64]($f | Measure-Object Length -Sum).Sum)
}
function Clear-Dir($d) {
  if (Test-Path $d) { cmd /c "rmdir /s /q `"$d`"" 2>$null }
  New-Item -ItemType Directory $d -Force | Out-Null
}

function Run-Tool($tool, $a, $dir) {
  $arc = "$dir\$a"
  $isTar = $a -like '*.tar.*' -or $a -like '*.tgz'
  $sw = [Diagnostics.Stopwatch]::StartNew()
  switch ($tool) {
    'ZipClick' { & "C:\bench\ZipClick.exe" --quiet $arc | Out-Null }

    '7-Zip' {
      if ($isTar) { cmd /c "`"$sz`" x -so `"$arc`" | `"$sz`" x -si -ttar -y -o`"$dir\out`" >nul 2>&1" }
      else { & $sz x -y "-o$dir\out" $arc | Out-Null }
    }
    'WinRAR' { Start-Process -FilePath $wr -ArgumentList @('x','-ibck','-y','-inul', $arc, "$dir\out\") -Wait }
    'Bandizip' { & $bz x -y "-o:$dir\out" $arc | Out-Null }
    'tar.exe' { New-Item -ItemType Directory "$dir\out" -Force | Out-Null; & tar.exe -xf $arc -C "$dir\out" 2>&1 | Out-Null }
    'Explorer' {
      New-Item -ItemType Directory "$dir\out" -Force | Out-Null
      $sh = New-Object -ComObject Shell.Application
      $src = $sh.Namespace($arc)
      if ($src -eq $null) { return -1 }
      $sh.Namespace("$dir\out").CopyHere($src.Items(), 4 + 16 + 1024)
      $t0 = [Diagnostics.Stopwatch]::StartNew()
      while ($t0.Elapsed.TotalSeconds -lt 300) {
        $m = Measure-Tree "$dir\out"
        if ($m[0] -ge $script:want[0] -and $m[1] -ge $script:want[1]) { break }
        Start-Sleep -Milliseconds 250
      }
      if ($t0.Elapsed.TotalSeconds -ge 300) { return -1 }
    }
    'Expand-Archive' { Expand-Archive -LiteralPath $arc -DestinationPath "$dir\out" -Force }
  }
  $sw.Stop()
  return $sw.Elapsed.TotalSeconds
}

$set = [ordered]@{
  'go-windows.zip'       = 'ZipClick','7-Zip','WinRAR','Bandizip','tar.exe','Explorer','Expand-Archive'
  'node-win-x64.zip'     = 'ZipClick','7-Zip','WinRAR','Bandizip','tar.exe','Explorer','Expand-Archive'
  'vscode-main.zip'      = 'ZipClick','7-Zip','WinRAR','Bandizip','tar.exe','Explorer','Expand-Archive'
  'ffmpeg-essentials.7z' = 'ZipClick','7-Zip','WinRAR','Bandizip','tar.exe','Explorer'
  'python-src.tgz'       = 'ZipClick','7-Zip','WinRAR','Bandizip','tar.exe','Explorer'
  'linux-7.2.11.tar.xz'  = 'ZipClick','7-Zip','WinRAR','Bandizip','tar.exe','Explorer'
}
foreach ($a in $set.Keys) {
  # Reference: what 7-Zip extracts.
  $dir = "$B\run"; Clear-Dir $dir
  New-Item -ItemType HardLink -Path "$dir\$a" -Target "$B\$a" | Out-Null
  $script:want = @(0, 0)
  Run-Tool '7-Zip' $a $dir | Out-Null
  Remove-Item -Force "$dir\$a"
  $script:want = Measure-Tree $dir
  "$a,reference,0,0,$($script:want[0]),$($script:want[1])" | Add-Content $out
  foreach ($tool in $set[$a]) {
    for ($i = 1; $i -le 3; $i++) {
      Clear-Dir $dir
      New-Item -ItemType HardLink -Path "$dir\$a" -Target "$B\$a" | Out-Null
      $s = Run-Tool $tool $a $dir
      Remove-Item -Force "$dir\$a"
      $m = Measure-Tree $dir
      "{0},{1},{2},{3},{4},{5}" -f $a, $tool, $i, $s, $m[0], $m[1] | Add-Content $out
    }
  }
}
Clear-Dir "$B\run"; Remove-Item "$B\run"
"DONE" | Add-Content $out
