package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	ole "github.com/go-ole/go-ole"
	"github.com/go-ole/go-ole/oleutil"
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

// comJob adalah satu pekerjaan COM yang dijalankan di thread khusus.
type comJob struct {
	run  func() error
	done chan error
}

var (
	comOnce   sync.Once
	comErr    error
	comWorker chan comJob
)

// startCOM menjelaskan satu thread khusus dengan COM (STA) aktif. Seluruh otomasi
// Office berjalan di thread itu: objek Office memakai model apartment, jadi
// inisialisasi dan pemanggilannya harus berada di thread yang sama.
func startCOM() {
	comWorker = make(chan comJob)
	ready := make(chan error, 1)

	go func() {
		runtime.LockOSThread()
		ready <- ole.CoInitializeEx(0, ole.COINIT_APARTMENTTHREADED)
		for job := range comWorker {
			job.done <- job.run()
		}
	}()

	comErr = <-ready
}

// comDo menjalankan f di thread COM tersebut.
func comDo(f func() error) error {
	comOnce.Do(startCOM)
	if comErr != nil {
		return comErr
	}

	job := comJob{run: f, done: make(chan error, 1)}
	comWorker <- job

	select {
	case err := <-job.done:
		return err
	case <-time.After(convertTimeout):
		// Thread itu tetap sibuk, jadi pekerjaan berikutnya akan ikut menunggu.
		// Lebih baik memberi error jelas daripada menggantung tanpa batas.
		return errors.New("konversi Office melebihi batas waktu")
	}
}

// OfficeAppAvailable melaporkan apakah aplikasi Office untuk keluarga itu terdaftar.
// Pemeriksaannya hanya menerjemahkan ProgID ke CLSID (baca registry), jadi
// aplikasinya tidak dijalankan; hasilnya dicache.
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

	_, lookupErr := ole.ClassIDFrom(progID)
	available := lookupErr == nil

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

// ConvertWithOffice menjalankan aplikasi Office lewat COM untuk satu langkah konversi.
func ConvertWithOffice(from, to Format, data []byte) ([]byte, error) {
	if runtime.GOOS != "windows" {
		return nil, errors.New("mesin Microsoft Office hanya tersedia di Windows")
	}

	progID, save, ok := officeTarget(from, to)
	if !ok {
		return nil, fmt.Errorf("Office tidak bisa menangani %s → %s", from, to)
	}

	dir, err := os.MkdirTemp("", "vert-office-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)

	// Berkas masukan ditulis ulang di sini, jadi penanda "berkas dari internet"
	// tidak terbawa dan Protected View tidak ikut aktif.
	input := filepath.Join(dir, "input."+string(from))
	if err := os.WriteFile(input, data, 0o644); err != nil {
		return nil, err
	}
	output := filepath.Join(dir, "output."+string(to))

	if err := comDo(func() error { return runOffice(progID, input, output, save) }); err != nil {
		return nil, fmt.Errorf("Office gagal: %w", err)
	}

	content, err := os.ReadFile(output)
	if err != nil || len(content) == 0 {
		return nil, fmt.Errorf("Office tidak menghasilkan berkas %s", to)
	}
	return content, nil
}

// runOffice membuka berkas, menyimpannya dalam format tujuan, lalu menutup aplikasi.
func runOffice(progID, input, output string, save int) (err error) {
	unknown, err := oleutil.CreateObject(progID)
	if err != nil {
		return fmt.Errorf("tidak bisa menjalankan %s: %w", progID, err)
	}
	app, err := unknown.QueryInterface(ole.IID_IDispatch)
	unknown.Release()
	if err != nil {
		return err
	}
	defer app.Release()

	// Aplikasi harus selalu ditutup, termasuk saat konversi gagal.
	defer func() {
		if _, quitErr := oleutil.CallMethod(app, "Quit"); quitErr != nil && err == nil {
			err = quitErr
		}
	}()

	switch progID {
	case "Word.Application":
		return convertWord(app, input, output, save)
	case "Excel.Application":
		return convertExcel(app, input, output, save)
	case "PowerPoint.Application":
		return convertPowerPoint(app, input, output, save)
	}
	return fmt.Errorf("aplikasi tidak dikenal: %s", progID)
}

func convertWord(app *ole.IDispatch, input, output string, save int) error {
	if err := putProperty(app, "Visible", false); err != nil {
		return err
	}
	if err := putProperty(app, "DisplayAlerts", 0); err != nil {
		return err
	}
	if err := putProperty(app, "AutomationSecurity", 3); err != nil {
		return err
	}

	documents, err := oleutil.GetProperty(app, "Documents")
	if err != nil {
		return err
	}
	defer documents.Clear()
	docs, err := dispatchOf(documents)
	if err != nil {
		return err
	}

	// Open(FileName, ConfirmConversions, ReadOnly, AddToRecentFiles)
	opened, err := oleutil.CallMethod(docs, "Open", input, false, true, false)
	if err != nil {
		return err
	}
	defer opened.Clear()

	doc, err := dispatchOf(opened)
	if err != nil {
		return err
	}
	if _, err := oleutil.CallMethod(doc, "SaveAs2", output, save); err != nil {
		return err
	}
	_, err = oleutil.CallMethod(doc, "Close", false)
	return err
}

func convertExcel(app *ole.IDispatch, input, output string, save int) error {
	if err := putProperty(app, "Visible", false); err != nil {
		return err
	}
	if err := putProperty(app, "DisplayAlerts", false); err != nil {
		return err
	}

	workbooks, err := oleutil.GetProperty(app, "Workbooks")
	if err != nil {
		return err
	}
	defer workbooks.Clear()
	books, err := dispatchOf(workbooks)
	if err != nil {
		return err
	}

	// Open(FileName, UpdateLinks, ReadOnly)
	opened, err := oleutil.CallMethod(books, "Open", input, 0, true)
	if err != nil {
		return err
	}
	defer opened.Clear()

	book, err := dispatchOf(opened)
	if err != nil {
		return err
	}

	// Excel menyimpan PDF lewat ExportAsFixedFormat, bukan SaveAs.
	if save == 0 {
		if _, err := oleutil.CallMethod(book, "ExportAsFixedFormat", 0, output); err != nil {
			return err
		}
	} else if _, err := oleutil.CallMethod(book, "SaveAs", output, save); err != nil {
		return err
	}
	_, err = oleutil.CallMethod(book, "Close", false)
	return err
}

func convertPowerPoint(app *ole.IDispatch, input, output string, save int) error {
	// PowerPoint menolak Visible = false, jadi jendelanya disembunyikan lewat
	// argumen WithWindow = false saat membuka presentasi.
	if err := putProperty(app, "DisplayAlerts", 1); err != nil {
		return err
	}

	presentations, err := oleutil.GetProperty(app, "Presentations")
	if err != nil {
		return err
	}
	defer presentations.Clear()
	pres, err := dispatchOf(presentations)
	if err != nil {
		return err
	}

	// Open(FileName, ReadOnly, Untitled, WithWindow)
	opened, err := oleutil.CallMethod(pres, "Open", input, true, false, false)
	if err != nil {
		return err
	}
	defer opened.Clear()

	deck, err := dispatchOf(opened)
	if err != nil {
		return err
	}
	if _, err := oleutil.CallMethod(deck, "SaveAs", output, save); err != nil {
		return err
	}
	_, err = oleutil.CallMethod(deck, "Close")
	return err
}

func putProperty(disp *ole.IDispatch, name string, value interface{}) error {
	_, err := oleutil.PutProperty(disp, name, value)
	return err
}

func dispatchOf(value *ole.VARIANT) (*ole.IDispatch, error) {
	dispatch := value.ToIDispatch()
	if dispatch == nil {
		return nil, errors.New("aplikasi Office tidak mengembalikan objek")
	}
	return dispatch, nil
}
