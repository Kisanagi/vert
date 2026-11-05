package main

import "fmt"

// pipelines: jalur pure-Go per pasangan format (dipakai bila engine tidak perlu/tidak ada).
var pipelines = map[string][]string{
	"md->txt":   {"mdToText"},
	"md->html":  {"mdToHTML"},
	"txt->md":   {},
	"xlsx->csv": {"xlsxToCSV"},
	"csv->xlsx": {"csvToXLSX"},
}

// enginePreferred: pasangan yang sengaja dialihkan ke LibreOffice walau jalur pure-Go ada.
var enginePreferred = map[string]bool{
	"docx->pdf": true,
	"pdf->docx": true,
}

// Pipeline menjelaskan cara menjalankan satu konversi.
type Pipeline struct {
	Engine bool // true = pakai LibreOffice
	Steps  []string
}

func pairKey(from, to Format) string { return fmt.Sprintf("%s->%s", from, to) }

func engineCanConvert(from, to Format) bool {
	a, b := FamilyOf(from), FamilyOf(to)
	if a == FamilyMarkup || b == FamilyMarkup {
		return false
	}
	if a == FamilyPDF {
		// PDF diimpor ke Writer (dokumen) atau Impress (presentasi); Calc tidak punya impor PDF.
		return b == FamilyWriter || b == FamilyImpress
	}
	if b == FamilyPDF {
		return true
	}
	return a == b
}

// PipelineFor memilih pipeline untuk sebuah pasangan.
// LibreOffice dipakai untuk pasangan yang diprioritaskan atau pasangan baru tanpa jalur pure-Go.
func PipelineFor(from, to Format) (Pipeline, bool) {
	if from == to {
		return Pipeline{}, false
	}
	k := pairKey(from, to)
	steps, hasPure := pipelines[k]

	if LibreOfficeAvailable() && (enginePreferred[k] || (!hasPure && engineCanConvert(from, to))) {
		return Pipeline{Engine: true}, true
	}
	if hasPure {
		return Pipeline{Steps: steps}, true
	}
	return Pipeline{}, false
}

// AvailableTargets mengembalikan format tujuan yang benar-benar bisa dihasilkan dari from.
func AvailableTargets(from Format) []Format {
	var out []Format
	for _, to := range Formats {
		if _, ok := PipelineFor(from, to); ok {
			out = append(out, to)
		}
	}
	return out
}

// RunConversion menjalankan pipeline atas data masukan dan mengembalikan hasilnya.
func RunConversion(from, to Format, data []byte) ([]byte, error) {
	pipeline, ok := PipelineFor(from, to)
	if !ok {
		return nil, fmt.Errorf("konversi %s → %s belum didukung", from, to)
	}
	if pipeline.Engine {
		return ConvertWithLibreOffice(from, to, data)
	}

	value := data
	for _, name := range pipeline.Steps {
		fn, ok := adaptersMap[name]
		if !ok {
			return nil, fmt.Errorf("adapter %s tidak ditemukan", name)
		}
		next, err := fn(value)
		if err != nil {
			return nil, err
		}
		value = next
	}
	return value, nil
}
