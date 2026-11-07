# AGENTS.md

Panduan singkat untuk agen atau kolaborator yang bekerja di repo ini.

## Ringkasan

`vert` — konverter dokumen **offline** berbasis CLI/TUI, ditulis dalam Go (satu binary,
tanpa runtime). Setiap konversi dirutekan berdasarkan **keluarga format**; mesin eksternal
dipakai bila ada (Microsoft Office lewat COM untuk keluarga Office, Edge headless untuk
`HTML → PDF`), dan sisanya jalur pure-Go.

## Struktur

| Berkas | Isi |
| --- | --- |
| `main.go` | entry point: CLI (`--to`, `--out`, `-formats`, `-find`, `-repair`, `-preview`) dan TUI |
| `formats.go` | daftar format, deteksi ekstensi, peta keluarga (Writer/Calc/Impress/PDF/markdown) |
| `pipeline.go` | routing: jalur pure-Go vs mesin eksternal, termasuk rantai maks. 2 langkah |
| `adapters.go` | adapter pure-Go: `md↔txt`, `md→html`, `xlsx↔csv` |
| `edge.go` | deteksi Edge/Chrome + pencetakan `HTML → PDF` lewat `--print-to-pdf` |
| `office.go` | deteksi aplikasi Office + otomasi COM dalam proses (`go-ole`) |
| `pdftotext.go` | ekstraksi PDF (Poppler `-bbox-layout`) + rekonstruksi paragraf/heading |
| `docx_writer.go` | penulis DOCX untuk `PDF → DOCX` |
| `markdown_tables.go` `repair.go` | normalisasi tabel ASCII dan perbaikan markdown rusak |
| `search.go` | pencarian berkas rekursif (dipakai TUI dan `-find`) |
| `tui.go` | antarmuka Bubbletea/Lipgloss |
| `install.ps1` `uninstall.ps1` | installer/penghapus satu perintah |

## Code Style

- Komentar dan pesan ke pengguna memakai Bahasa Indonesia.
- Perubahan sekecil mungkin; jangan menambah abstraksi yang belum perlu.
- Setelah mengubah kode, jalankan: `go build ./...`, `go vet ./...`, `gofmt -l .`
  (`gofmt` harus tidak mengeluarkan apa pun).

## Architecture Notes

- `directHop()` memilih antara jalur pure-Go dan mesin **berdasarkan keluarga format**;
  mesin dipakai hanya bila tidak ada jalur pure-Go untuk pasangan itu.
- `engineCanConvert()` **menolak PDF sebagai sumber**: pembacaan PDF selalu lewat
  `pdftotext`, karena Office tidak mengimpor PDF. Karena itu `PDF → PPTX/ODP` tidak ada.
- Ketersediaan mesin menyembunyikan target, bukan menimbulkan error: tanpa Office,
  tanpa Edge, atau tanpa `pdftotext`, pasangan terkait hanya tidak muncul di UI.
- Otomasi Office berjalan **di dalam proses** lewat `go-ole`, pada satu thread khusus
  dengan COM STA aktif (`startCOM`/`comDo`). Objek Office memakai model apartment, jadi
  inisialisasi dan pemanggilan COM wajib berada di thread yang sama.
- Deteksi Office memakai `CLSIDFromProgID` (baca registry), **bukan** dengan membuat
  objeknya — menjalankan Word/Excel/PowerPoint hanya untuk memeriksa terlalu lambat.
- Versi ini sengaja **tidak** memakai PowerShell: binary tak bertanda tangan yang
  menjalankan PowerShell tersembunyi dengan `-ExecutionPolicy Bypass` memicu deteksi
  antivirus (Defender menandainya `Trojan:Win32/Bearfoos.A!ml`, heuristik ML).

## Common Workflows

```
go build -o vert.exe .     # build
go vet ./...               # analisis statis
gofmt -l .                 # cek format (harus kosong)
./vert.exe                 # buka TUI
./vert.exe data.csv --to xlsx
git tag v0.1.3 && git push origin v0.1.3   # picu rilis di GitHub Actions
```
