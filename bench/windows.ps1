# ZipClick benchmark on Windows: each tool on each archive from
# make-archives.sh, three runs each, the median is what counts. Put the
# archives and ZipClick.exe in C:\bench, run as the desktop user (Explorer
# needs a session), results land in C:\bench\results.csv.
$ErrorActionPreference = 'Continue'
$B = 'C:\bench'
$out = "$B\results.csv"
$sz = 'C:\Program Files\7-Zip\7z.exe'
$wr = 'C:\Program Files\WinRAR\WinRAR.exe'
$bz = 'C:\Program Files\Bandizip\bz.exe'
$zc = "$B\ZipClick.exe"
# Decimal points, not commas, whatever the Windows locale.
[Threading.Thread]::CurrentThread.CurrentCulture = 'en-US'
$archives = 'a.zip','aes.zip','a.7z','a.rar','a.tar.gz','a.tar.xz','a.tar.zst'
"format,tool,run,seconds,files,bytes" | Set-Content $out

function Measure-Tree($d) {
  $f = Get-ChildItem -Recurse -File $d -ErrorAction SilentlyContinue
  return @($f.Count, ($f | Measure-Object Length -Sum).Sum)
}

function Run-Tool($tool, $a, $dir) {
  $arc = "$dir\$a"
  $pw = ($a -eq 'aes.zip')
  $isTar = $a -like 'a.tar.*'
  $sw = [Diagnostics.Stopwatch]::StartNew()
  switch ($tool) {
    'ZipClick' {
      $args = @('--quiet'); if ($pw) { $args += '--password=x' }; $args += $arc
      & $zc @args | Out-Null
    }
    '7-Zip' {
      if ($isTar) { cmd /c "`"$sz`" x -so `"$arc`" | `"$sz`" x -si -ttar -y -o`"$dir\out`" >nul 2>&1" }
      else { $args = @('x','-y',"-o$dir\out"); if ($pw) { $args += '-px' }; $args += $arc; & $sz @args | Out-Null }
    }
    'WinRAR' {
      $args = @('x','-ibck','-y','-inul'); if ($pw) { $args += '-px' }; $args += @($arc, "$dir\out\")
      Start-Process -FilePath $wr -ArgumentList $args -Wait
    }
    'Bandizip' {
      $args = @('x','-y',"-o:$dir\out"); if ($pw) { $args += '-p:x' }; $args += $arc
      & $bz @args | Out-Null
    }
    'tar.exe' {
      New-Item -ItemType Directory "$dir\out" -Force | Out-Null
      $args = @('-xf', $arc, '-C', "$dir\out"); if ($pw) { $args = @('--passphrase','x') + $args }
      & tar.exe @args 2>&1 | Out-Null
    }
    'Explorer' {
      New-Item -ItemType Directory "$dir\out" -Force | Out-Null
      $sh = New-Object -ComObject Shell.Application
      $src = $sh.Namespace($arc)
      if ($src -eq $null) { return $null }
      $sh.Namespace("$dir\out").CopyHere($src.Items(), 4 + 16 + 1024)
      # CopyHere can return before it finishes: wait until every byte is there.
      $t0 = [Diagnostics.Stopwatch]::StartNew()
      while ($t0.Elapsed.TotalSeconds -lt 600) {
        $m = Measure-Tree "$dir\out"
        if ($m[0] -eq $script:wantFiles -and $m[1] -eq $script:wantBytes) { break }
        Start-Sleep -Milliseconds 100
      }
    }
    'Expand-Archive' {
      Expand-Archive -LiteralPath $arc -DestinationPath "$dir\out" -Force
    }
  }
  $sw.Stop()
  return $sw.Elapsed.TotalSeconds
}

# The reference result: what 7-Zip extracts from the plain zip.
$ref = "$B\ref"
Remove-Item -Recurse -Force $ref -ErrorAction SilentlyContinue
& $sz x -y "-o$ref" "$B\a.zip" | Out-Null
$m = Measure-Tree $ref
$script:wantFiles = $m[0]; $script:wantBytes = $m[1]
Remove-Item -Recurse -Force $ref

$tools = @{
  'a.zip'     = 'ZipClick','7-Zip','WinRAR','Bandizip','tar.exe','Explorer','Expand-Archive'
  'aes.zip'   = 'ZipClick','7-Zip','WinRAR','Bandizip','tar.exe'
  'a.7z'      = 'ZipClick','7-Zip','WinRAR','Bandizip','tar.exe','Explorer'
  'a.rar'     = 'ZipClick','7-Zip','WinRAR','Bandizip','tar.exe','Explorer'
  'a.tar.gz'  = 'ZipClick','7-Zip','WinRAR','Bandizip','tar.exe','Explorer'
  'a.tar.xz'  = 'ZipClick','7-Zip','WinRAR','Bandizip','tar.exe','Explorer'
  'a.tar.zst' = 'ZipClick','7-Zip','WinRAR','Bandizip','tar.exe','Explorer'
}
foreach ($a in $archives) {
  foreach ($tool in $tools[$a]) {
    for ($i = 1; $i -le 3; $i++) {
      $dir = "$B\run"
      Remove-Item -Recurse -Force $dir -ErrorAction SilentlyContinue
      New-Item -ItemType Directory $dir | Out-Null
      New-Item -ItemType HardLink -Path "$dir\$a" -Target "$B\$a" | Out-Null
      $s = Run-Tool $tool $a $dir
      Remove-Item -Force "$dir\$a"
      $m = Measure-Tree $dir
      "{0},{1},{2},{3},{4},{5}" -f $a, $tool, $i, $s, $m[0], $m[1] | Add-Content $out
    }
  }
}
Remove-Item -Recurse -Force "$B\run" -ErrorAction SilentlyContinue
"DONE" | Add-Content $out
