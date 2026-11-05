# MD Converter (`vert`)

Konverter dokumen **offline** berbasis **CLI/TUI**, ditulis dalam Go — satu binary
(±14 MB) tanpa runtime. Semua proses berjalan lokal, tidak ada koneksi jaringan.

## Install

Satu perintah di **PowerShell** (bawaan Windows 10/11):

```powershell
irm https://github.com/Kisanagi/vert/releases/latest/download/install.ps1 | iex
```

Dari **cmd**:

```
powershell -c "irm https://github.com/Kisanagi/vert/releases/latest/download/install.ps1|iex"
```

Skrip mengunduh `vert.exe` ke `%USERPROFILE%\bin` dan menambahkannya ke PATH.
Setelah selesai, buka terminal baru lalu jalankan `vert`.

Kalau sudah punya Go, alternatifnya satu perintah juga:

```
go install github.com/Kisanagi/vert@latest
```

## Uninstall

Satu perintah juga:

```powershell
irm https://github.com/Kisanagi/vert/releases/latest/download/uninstall.ps1 | iex
```

Skrip menghapus `vert.exe`/`mdconv.exe` dari `%USERPROFILE%\bin`, membereskan PATH,
dan menghapus folder itu bila sudah kosong. Buka terminal baru setelahnya.

> Tutup dulu aplikasi `vert` bila sedang berjalan — Windows mengunci file yang
> sedang dieksekusi.

Kalau LibreOffice dipasang khusus untuk tool ini, hapus juga lewat
**Settings → Apps**, atau `winget uninstall TheDocumentFoundation.LibreOffice`.

## Pakai

```
vert                            # buka TUI (browse + cari berkas, lalu pilih format)
vert D:\Dokumen                 # buka TUI langsung di folder tertentu
vert laporan.docx --to pdf      # satu berkas
vert *.csv --to xlsx            # borongan
vert data.csv -formats          # lihat target yang didukung
vert data.csv --to pdf --out hasil/
vert -find laporan D:\Dokumen   # cari berkas (rekursif), cetak hasilnya
```

### Navigasi TUI

Daftar berkas tampil dalam satu panel: baris path (breadcrumb) di atas, lalu kolom
nama, ukuran, dan waktu ubah.

| Tombol | Fungsi |
| --- | --- |
| `↑` `↓` | pindah pilihan |
| `Enter` | buka folder / pilih berkas |
| `Backspace` | naik satu folder |
| `/` | cari berkas (rekursif dari folder aktif) |
| `~` | lompat ke folder home |
| `Tab` | ganti drive (mis. `C:` → `D:`) |
| `q` / `Ctrl+C` | keluar |

Di mode cari: ketik untuk menyaring, `↑/↓` pilih, `Enter` pakai, `Esc` kembali.

Pencarian hanya menampilkan berkas yang **bisa dikonversi**, menelusuri sampai 6 level,
melewati folder berat (`node_modules`, `.git`, `AppData`, `Windows`, dll), dan berhenti
di 300 hasil.

Setelah berkas dipilih, muncul daftar format tujuan → `↑/↓` pilih, `Enter` konversi.
Hasil ditulis di folder yang sama dengan ekstensi baru.

## Format

| Keluarga | Format |
| --- | --- |
| Dokumen (Writer) | DOCX, ODT, RTF, TXT, HTML |
| Spreadsheet (Calc) | XLSX, ODS, CSV |
| Presentasi (Impress) | PPTX, ODP |
| Lain | PDF, Markdown |

- Dalam satu keluarga bebas bolak-balik — mis. `DOCX ↔ ODT`, `XLSX ↔ CSV`, `PPTX ↔ ODP`.
- Apa pun dari keluarga di atas → **PDF**.
- **PDF → dokumen** lewat import filter Writer (`writer_pdf_import`);
  **PDF → presentasi** lewat `impress_pdf_import` (tiap halaman jadi satu slide).
- **Markdown** (versi ini) hanya ke TXT dan HTML.

Belum didukung: `MD → PDF/DOCX`, `PDF → XLSX/CSV`, pasangan lintas-keluarga
(mis. `DOCX → XLSX`), format Office lama (`.doc`/`.ppt`/`.xls`), serta gambar/audio/video/arsip.

## Engine

Konversi keluarga Office memakai **LibreOffice** yang terpasang di sistem, dideteksi
otomatis (`MD_CONVERTER_SOFFICE`, lalu lokasi sistem).

- **Dengan LibreOffice**: seluruh keluarga Writer/Calc/Impress + rute PDF (termasuk
  `PDF → DOCX` lewat `writer_pdf_import` dan `PDF → PPTX` lewat `impress_pdf_import`).
- **Tanpa LibreOffice**: hanya jalur pure-Go yang jalan — `md↔txt`, `md→html`, `xlsx↔csv`.
  Target yang tidak bisa dihasilkan otomatis tidak ditawarkan.

## Build

```
go build -o vert.exe .
```

Menghasilkan binary ~14 MB (tanpa engine). Butuh Go 1.26+.

## Catatan

- Nama `vert` dipilih karena tidak bentrok dengan perintah bawaan Windows. `convert`
  tidak bisa dipakai: `C:\Windows\System32\convert.exe` (alat konversi drive) sudah
  memakainya, dan di cmd nama itu tidak bisa ditimpa dari sisi user.
- Binary tidak mem-bundle LibreOffice; ukurannya tetap kecil dan memanfaatkan engine
  yang sudah ada di sistem.
- `md→txt` memakai penyaring markup sederhana, bukan render penuh.
