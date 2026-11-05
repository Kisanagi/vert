# AGENTS.md

Panduan singkat untuk agen atau kolaborator yang bekerja di repo ini.

## Ringkasan

`vert` — konverter dokumen **offline** berbasis CLI/TUI, ditulis dalam Go (satu binary,
tanpa runtime). Arsitekturnya meniru pola engine: setiap konversi dirutekan berdasarkan
**keluarga format**, dan LibreOffice dipakai sebagai mesin bila tersedia.

## Struktur

| Berkas | Isi |
| --- | --- |
| `main.go` | entry point: CLI (`--to`, `--out`, `-formats`, `-find`) dan TUI |
| `formats.go` | daftar format, deteksi ekstensi, peta keluarga (Writer/Calc/Impress/PDF/markdown) |
| `pipeline.go` | routing: jalur pure-Go vs engine LibreOffice |
| `adapters.go` | adapter pure-Go: `md↔txt`, `md→html`, `xlsx↔csv` |
| `search.go` | pencarian berkas rekursif (dipakai TUI dan `-find`) |
| `libreoffice.go` | deteksi biner + pemanggilan `soffice --convert-to` |
| `tui.go` | antarmuka Bubbletea/Lipgloss |
| `install.ps1` | installer satu perintah (unduh `vert.exe` dari GitHub Releases) |

## Code Style

- Komentar dan pesan ke pengguna memakai Bahasa Indonesia.
- Perubahan sekecil mungkin; jangan menambah abstraksi yang belum perlu.
- Setelah mengubah kode, jalankan: `go build ./...`, `go vet ./...`, `gofmt -l .`
  (`gofmt` harus tidak mengeluarkan apa pun).

## Architecture Notes

- `pipelineFor()` memilih antara jalur pure-Go dan engine **berdasarkan keluarga format**;
  jalur pure-Go yang sudah ada tidak pernah ditimpa kecuali pasangan yang diprioritaskan.
- Tanpa LibreOffice cakupan menyusut ke jalur pure-Go (`md↔txt`, `md→html`, `xlsx↔csv`).
  Itu **disengaja** — jangan "memperbaiki"-nya dengan menambah dependency berat,
  karena LibreOffice tidak di-bundle.

## Common Workflows

```
go build -o vert.exe .     # build
go vet ./...               # analisis statis
gofmt -l .                 # cek format (harus kosong)
./vert.exe                 # buka TUI
./vert.exe data.csv --to xlsx
git tag v0.1.0 && git push origin v0.1.0   # picu rilis di GitHub Actions
```
