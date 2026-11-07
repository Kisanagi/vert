package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

// officeApps memetakan keluarga format ke aplikasi Office yang menanganinya.
var officeApps = map[Family]string{
	FamilyWriter:  "Word.Application",
	FamilyCalc:    "Excel.Application",
	FamilyImpress: "PowerPoint.Application",
}

// Kode penyimpanan COM (WdSaveFormat, XlFileFormat, PpSaveAsFileType).
var writerSaveCodes = map[Format]int{
	FormatPDF:  17, // wdFormatPDF
	FormatDOCX: 16, // wdFormatXMLDocument
	FormatODT:  23, // wdFormatOpenDocumentText
	FormatRTF:  6,  // wdFormatRTF
	FormatTXT:  2,  // wdFormatText
	FormatHTML: 10, // wdFormatFilteredHTML
}

var calcSaveCodes = map[Format]int{
	FormatPDF:  0,  // penanda: Excel memakai ExportAsFixedFormat
	FormatXLSX: 51, // xlOpenXMLWorkbook
	FormatODS:  60, // xlOpenDocumentSpreadsheet
	FormatCSV:  62, // xlCSVUTF8
}

var impressSaveCodes = map[Format]int{
	FormatPDF:  32, // ppSaveAsPDF
	FormatPPTX: 24, // ppSaveAsOpenXMLPresentation
	FormatODP:  35, // ppSaveAsOpenDocumentPresentation
}

var (
	officeCacheMu sync.Mutex
	officeCache   = map[Family]bool{}
)

// OfficeAppAvailable melaporkan apakah aplikasi Office untuk keluarga itu terdaftar
// di sistem. Hasilnya dicache karena deteksinya memanggil PowerShell.
func OfficeAppAvailable(family Family) bool {
	progID, ok := officeApps[family]
	if !ok || runtime.GOOS != "windows" {
		return false
	}

	officeCacheMu.Lock()
	if value, seen := officeCache[family]; seen {
		officeCacheMu.Unlock()
		return value
	}
	officeCacheMu.Unlock()

	script := fmt.Sprintf(`if (Test-Path "Registry::HKEY_CLASSES_ROOT\%s") { exit 0 } else { exit 1 }`, progID)
	cmd := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-Command", script)
	hideWindow(cmd)
	available := cmd.Run() == nil

	officeCacheMu.Lock()
	officeCache[family] = available
	officeCacheMu.Unlock()
	return available
}

// OfficeAvailable melaporkan apakah pasangan format ini bisa ditangani Office.
// PDF tidak pernah masuk ke sini: pembacaan PDF ditangani jalur pure-Go.
func OfficeAvailable(from, to Format) bool {
	if FamilyOf(from) == FamilyPDF {
		return false
	}
	if !engineCanConvert(from, to) {
		return false
	}
	return OfficeAppAvailable(FamilyOf(from))
}

// officeTarget menentukan aplikasi dan kode penyimpanan untuk sebuah pasangan format.
func officeTarget(from, to Format) (app string, save int, ok bool) {
	switch FamilyOf(from) {
	case FamilyWriter:
		app = "Word.Application"
		save, ok = writerSaveCodes[to]
	case FamilyCalc:
		app = "Excel.Application"
		save, ok = calcSaveCodes[to]
	case FamilyImpress:
		app = "PowerPoint.Application"
		save, ok = impressSaveCodes[to]
	}
	return app, save, ok
}

// officeScript mengotomasi aplikasi Office lewat COM. try/finally memastikan
// aplikasi selalu ditutup, dan DisplayAlerts dimatikan agar tidak ada dialog yang
// menggantung. Jendela editor disembunyikan bila aplikasinya mengizinkan
// (PowerPoint menolak Visible = $false, jadi hanya berkas tanpa jendela yang dipakai).
const officeScript = `
param(
  [Parameter(Mandatory=$true)][string]$In,
  [Parameter(Mandatory=$true)][string]$Out,
  [Parameter(Mandatory=$true)][string]$ProgID,
  [Parameter(Mandatory=$true)][int]$Save
)
$ErrorActionPreference = 'Stop'

# Nama variabel tidak boleh berbeda hanya pada besar-kecil huruf: PowerShell
# menganggapnya variabel yang sama.
$app = New-Object -ComObject $ProgID
try {
  switch ($ProgID) {
    'Word.Application' {
      $app.Visible = $false
      $app.DisplayAlerts = 0
      $app.AutomationSecurity = 3
      $doc = $app.Documents.Open($In, $false, $true, $false)
      try { $doc.SaveAs2($Out, $Save) } finally { $doc.Close($false) }
    }
    'Excel.Application' {
      $app.Visible = $false
      $app.DisplayAlerts = $false
      $book = $app.Workbooks.Open($In, 0, $true)
      try {
        if ($Save -eq 0) { $book.ExportAsFixedFormat(0, $Out) }
        else { $book.SaveAs($Out, $Save) }
      } finally { $book.Close($false) }
    }
    'PowerPoint.Application' {
      $app.DisplayAlerts = 1
      $pres = $app.Presentations.Open($In, $true, $false, $false)
      try { $pres.SaveAs($Out, $Save) } finally { $pres.Close() }
    }
    default { throw "aplikasi tidak dikenal: $ProgID" }
  }
} finally {
  $app.Quit()
}
`

// ConvertWithOffice menjalankan aplikasi Office lewat COM untuk satu langkah konversi.
func ConvertWithOffice(from, to Format, data []byte) ([]byte, error) {
	if runtime.GOOS != "windows" {
		return nil, errors.New("mesin Microsoft Office hanya tersedia di Windows")
	}

	app, save, ok := officeTarget(from, to)
	if !ok {
		return nil, fmt.Errorf("Office tidak bisa menangani %s → %s", from, to)
	}

	dir, err := os.MkdirTemp("", "vert-office-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)

	input := filepath.Join(dir, "input."+string(from))
	if err := os.WriteFile(input, data, 0o644); err != nil {
		return nil, err
	}

	// Berkas masukan ditulis ulang di sini, jadi penanda "berkas dari internet"
	// tidak terbawa dan Protected View tidak ikut aktif.
	output := filepath.Join(dir, "output."+string(to))
	script := filepath.Join(dir, "convert.ps1")
	if err := os.WriteFile(script, []byte(officeScript), 0o644); err != nil {
		return nil, err
	}

	cmd := exec.Command("powershell.exe",
		"-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass",
		"-File", script,
		"-In", input,
		"-Out", output,
		"-ProgID", app,
		"-Save", strconv.Itoa(save))
	hideWindow(cmd)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return nil, err
	}

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	select {
	case err := <-done:
		if err != nil {
			// Hanya proses PowerShell ini yang dibunuh saat gagal; aplikasi Office
			// milik pengguna tidak boleh diganggu.
			message := strings.TrimSpace(stderr.String())
			if message == "" {
				message = err.Error()
			}
			return nil, fmt.Errorf("Office gagal: %s", message)
		}
	case <-time.After(convertTimeout):
		_ = cmd.Process.Kill()
		return nil, errors.New("konversi Office melebihi batas waktu")
	}

	content, err := os.ReadFile(output)
	if err != nil || len(content) == 0 {
		return nil, fmt.Errorf("Office tidak menghasilkan berkas %s", to)
	}
	return content, nil
}
