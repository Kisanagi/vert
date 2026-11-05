# Copot vert — hapus biner, entri PATH, dan sisa profil kosong.
#
# Pakai (PowerShell):
#   irm https://github.com/Kisanagi/vert/releases/latest/download/uninstall.ps1 | iex
#
# Tutup dulu aplikasi vert bila sedang berjalan.

$ErrorActionPreference = 'Stop'

$Target = Join-Path $env:USERPROFILE 'bin'
$Names = @('vert.exe', 'mdconv.exe')

foreach ($exe in $Names) {
    $path = Join-Path $Target $exe
    if (Test-Path $path) {
        try {
            Remove-Item -LiteralPath $path -Force -ErrorAction Stop
            Write-Host "Dihapus: $path"
        } catch {
            Write-Warning "Tidak bisa menghapus $path -- kemungkinan sedang berjalan. Tutup vert lalu jalankan lagi."
        }
    }
}

# Kalau folder jadi kosong, berarti hanya dipakai tool ini: hapus folder + entri PATH-nya.
if ((Test-Path $Target) -and -not (Get-ChildItem -Force $Target)) {
    Remove-Item -LiteralPath $Target -Force
    Write-Host "Folder dihapus: $Target"

    $UserPath = [Environment]::GetEnvironmentVariable('Path', 'User')
    $NewPath = ($UserPath -split ';' | Where-Object { $_ -and $_ -ne $Target }) -join ';'
    [Environment]::SetEnvironmentVariable('Path', $NewPath, 'User')
    Write-Host 'PATH diperbarui.'
}

# Profil PowerShell yang dibuat installer dulu (hanya dihapus bila benar-benar kosong).
$ProfilePath = Join-Path $env:USERPROFILE 'Documents\WindowsPowerShell\Microsoft.PowerShell_profile.ps1'
if ((Test-Path $ProfilePath) -and (Get-Item $ProfilePath).Length -eq 0) {
    Remove-Item -LiteralPath $ProfilePath -Force
    Write-Host "Profil kosong dihapus: $ProfilePath"
}

Write-Host ''
Write-Host 'Selesai. Buka terminal baru agar perubahan PATH berlaku.'
