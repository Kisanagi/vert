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

Skrip menghapus `vert.exe` dari `%USERPROFILE%\bin`, membereskan PATH,
dan menghapus folder itu bila sudah kosong. Buka terminal baru setelahnya.

> Tutup dulu aplikasi `vert` bila sedang berjalan — Windows mengunci file yang
> sedang dieksekusi.

Tidak ada yang perlu dibereskan dari sisi mesin: `vert` memakai Microsoft Office
dan Edge yang sudah ada di sistem (lihat bagian **Engine**).

## Pakai

```
vert                            # buka TUI (browse + cari berkas, lalu pilih format)
vert D:\Dokumen                 # buka TUI langsung di folder tertentu
vert laporan.docx --to pdf      # satu berkas
vert *.csv --to xlsx            # borongan
vert data.csv -formats          # lihat target yang didukung
vert data.csv --to pdf --out hasil/
vert -find laporan D:\Dokumen   # cari berkas (rekursif), cetak hasilnya
vert -repair catatan.md         # perbaiki markdown rusak -> catatan-fixed.md
vert -preview catatan.md        # cetak satu layar TUI (cek tata letak)
```

### Navigasi TUI

Satu panel: baris path (breadcrumb) di atas, lalu kolom nama, ukuran, dan waktu ubah.
Folder ditandai `[D]` dan kolom `<DIR>`.

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

Untuk berkas `.md`, daftar tujuan diawali opsi **Perbaiki markdown** — menulis
`<nama>-fixed.md` berisi versi yang sudah dirapikan (tabel runtuh dipecah,
butir `•` jadi daftar, heading bernomor dipulihkan). Perbaikan yang sama juga
berjalan otomatis saat mengonversi markdown yang terdeteksi rusak.

Pencarian hanya menampilkan berkas yang **bisa dikonversi**, menelusuri sampai 6 level,
melewati folder berat (`node_modules`, `.git`, `AppData`, `Windows`, dll), dan berhenti
di 300 hasil.

Setelah berkas dipilih, muncul daftar format tujuan → `↑/↓` pilih, `Enter` konversi.
Setelah selesai, `Enter` kembali ke daftar berkas (aplikasi tidak langsung keluar).
Hasil ditulis di folder yang sama dengan ekstensi baru.

## Format

| Keluarga | Format |
| --- | --- |
| Dokumen (Writer) | DOCX, ODT, RTF, TXT, HTML |
| Spreadsheet (Calc) | XLSX, ODS, CSV |
| Presentasi (Impress) | PPTX, ODP |
| Lain | PDF, Markdown |

- Dalam satu keluarga bebas bolak-balik — mis. `DOCX ↔ ODT`, `XLSX ↔ CSV`, `PPTX ↔ ODP`.
- Apa pun dari keluarga Office → **PDF** (Word/Excel/PowerPoint).
- `MD`/`HTML → PDF` dicetak oleh **Edge** (dukungan CSS terbaik).
- **Dari PDF:** ke **DOCX** lewat penulis DOCX bawaan, dan ke **TXT/MD** lewat
  `pdftotext` (Poppler). `PDF → PPTX/ODP` tidak didukung — PowerPoint tidak mengimpor
  PDF dengan andal.
- **Jalur berantai (maks. 2 langkah).** Bila tidak ada jalur langsung, konversi
  dirutekan lewat perantara — mis. `MD → HTML → PDF`, `CSV → PDF → DOCX`,
  `DOCX → HTML → MD`. Jalur langsung selalu diutamakan, dan UI menandai konversi
  bertingkat karena hasilnya bisa kurang presisi.
- Total pasangan yang didukung (Office + Edge + pdftotext terpasang): **67 dari 132**.

Belum didukung: `PDF → HTML/ODT/RTF/PPTX/ODP`, pasangan lintas-keluarga tanpa perantara
(mis. `DOCX → XLSX`), format Office lama (`.doc`/`.ppt`/`.xls`), serta
gambar/audio/video/arsip.

## Engine

`vert` tidak mem-bundle mesin apa pun — hanya memakai yang sudah ada di sistem.

| Mesin | Dipakai untuk | Cara dideteksi |
| --- | --- | --- |
| **Microsoft Office** (Word/Excel/PowerPoint) lewat COM | keluarga Office ↔ Office, dan semua → PDF | ProgID diterjemahkan ke CLSID (baca registry), tanpa menjalankan aplikasinya |
| **Edge** (cadangan: Chrome) headless | `HTML → PDF`, termasuk jalur `MD → PDF` | `MD_CONVERTER_EDGE`, lalu lokasi Edge/Chrome terpasang |
| **pdftotext** (Poppler) | `PDF → TXT/MD/DOCX` | `MD_CONVERTER_PDFTOTEXT`, lalu PATH, lalu lokasi umum (termasuk `mingw64` bawaan Git for Windows dan paket winget `oschwartz10612.Poppler`) |

- Otomasi Office berjalan **di dalam proses** (tanpa proses PowerShell terpisah), tak
  terlihat, dengan `DisplayAlerts` mati dan berkas dibuka read-only; aplikasinya selalu
  ditutup setelah selesai. Berkas masukan ditulis ulang di folder sementara, jadi
  penanda "berkas dari internet" tidak terbawa dan Protected View tidak aktif.
- Ekstraksi PDF memakai `pdftotext -bbox-layout`: blok jadi paragraf, dan tinggi baris
  dipakai menebak tingkat heading.
- **Tanpa Office**: tersisa `md↔txt`, `md→html`, `xlsx↔csv`, dan `PDF → DOCX/TXT/MD`.
  Target lain otomatis tidak ditawarkan.
- **Tanpa Edge**: `MD`/`HTML → PDF` tidak ditawarkan.

## Build

```
go build -o vert.exe .
```

Menghasilkan binary ~14 MB (tanpa engine). Butuh Go 1.26+.

## Catatan

- Nama `vert` dipilih karena tidak bentrok dengan perintah bawaan Windows. `convert`
  tidak bisa dipakai: `C:\Windows\System32\convert.exe` (alat konversi drive) sudah
  memakainya, dan di cmd nama itu tidak bisa ditimpa dari sisi user.
- Binary tidak mem-bundle mesin apa pun; ukurannya tetap kecil dan memanfaatkan Office
  dan Edge yang sudah ada di sistem.
- `md→txt` memakai penyaring markup sederhana, bukan render penuh.
