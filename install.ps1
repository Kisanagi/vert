# Instalasi vert — unduh dari GitHub Releases lalu pasang ke PATH.
#
# Pakai (PowerShell):
#   irm https://github.com/Kisanagi/vert/releases/latest/download/install.ps1 | iex

$ErrorActionPreference = 'Stop'

$Repo = 'Kisanagi/vert'
$Asset = 'vert.exe'
$Target = Join-Path $env:USERPROFILE 'bin'
$Url = "https://github.com/$Repo/releases/latest/download/$Asset"

Write-Host "Mengunduh $Asset dari $Repo ..."
New-Item -ItemType Directory -Force -Path $Target | Out-Null
$Destination = Join-Path $Target $Asset
Invoke-WebRequest -Uri $Url -OutFile $Destination -UseBasicParsing

$UserPath = [Environment]::GetEnvironmentVariable('Path', 'User')
if ($UserPath -notlike "*$Target*") {
    $NewPath = if ([string]::IsNullOrWhiteSpace($UserPath)) { $Target } else { $UserPath.TrimEnd(';') + ';' + $Target }
    [Environment]::SetEnvironmentVariable('Path', $NewPath, 'User')
    Write-Host "PATH diperbarui: $Target"
}

Write-Host ''
Write-Host "Selesai: $Destination"

if (-not (Test-Path 'Registry::HKEY_CLASSES_ROOT\Word.Application')) {
    Write-Host ''
    Write-Host 'Peringatan: Microsoft Office tidak terdeteksi di sistem ini.'
    Write-Host 'Tanpa Office, hanya konversi ini yang tersedia:'
    Write-Host '  PDF -> DOCX/TXT/MD, MD <-> TXT/HTML, XLSX <-> CSV'
}

Write-Host 'Buka terminal baru, lalu jalankan:  vert'
