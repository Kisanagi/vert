package main

import "fmt"

// pipelines: jalur pure-Go per pasangan format (dipakai bila engine tidak perlu/tidak ada).
var pipelines = map[string][]string{
	"md->txt":   {"mdToText"},
	"md->html":  {"mdToHTML"},
	"txt->md":   {},
	"xlsx->csv": {"xlsxToCSV"},
	"csv->xlsx": {"csvToXLSX"},

	// PDF -> teks memakai pdftotext (Poppler), bukan LibreOffice: hasil impor PDF
	// di LibreOffice tersimpan di text frame dan ekspornya membuang isi.
	"pdf->txt":  {"pdfToText"},
	"pdf->md":   {"pdfToMarkdown"},
	"pdf->docx": {"pdfToDocx"},
}

// adapterChecks menandai adapter yang butuh alat eksternal. Bila alatnya tidak
// tersedia, pasangan format itu tidak ditawarkan sama sekali.
var adapterChecks = map[string]func() bool{
	"pdfToMarkdown": pdftotextAvailable,
	"pdfToText":     pdftotextAvailable,
	"pdfToDocx":     pdftotextAvailable,
}

func adaptersReady(steps []string) bool {
	for _, name := range steps {
		if check, ok := adapterChecks[name]; ok && !check() {
			return false
		}
	}
	return true
}

// enginePreferred: pasangan yang sengaja dialihkan ke LibreOffice walau jalur pure-Go ada.
var enginePreferred = map[string]bool{
	"docx->pdf": true,
	"pdf->docx": true,
}

// maxHops membatasi panjang jalur berantai. Jalur langsung selalu diutamakan; rantai
// hanya dipakai bila tidak ada jalur langsung. Makin panjang rantai, makin rendah
// kualitas hasilnya.
const maxHops = 2

// Hop adalah satu langkah konversi: adapter pure-Go, atau engine LibreOffice.
type Hop struct {
	From     Format
	To       Format
	Engine   bool     // true = jalankan LibreOffice
	Adapters []string // dipakai bila Engine == false
}

// Pipeline adalah rangkaian langkah dari format asal ke format tujuan.
type Pipeline struct {
	Hops []Hop
}

// Chained melaporkan apakah konversi ini butuh lebih dari satu langkah.
func (p Pipeline) Chained() bool { return len(p.Hops) > 1 }

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

// pdfImportTargets membatasi tujuan konversi dari PDF. LibreOffice menyimpan hasil
// impor PDF di text frame; ekspor ke TXT/HTML/ODT/RTF membuang isinya (file jadi
// kosong atau kehilangan paragraf). Hanya format ini yang terbukti menjaga isi.
var pdfImportTargets = map[Format]bool{
	FormatDOCX: true,
	FormatPPTX: true,
	FormatODP:  true,
}

func allowedFromPDF(to Format) bool { return pdfImportTargets[to] }

// directHop mengembalikan langkah langsung untuk pasangan format, bila ada.
func directHop(from, to Format) (Hop, bool) {
	if from == to {
		return Hop{}, false
	}
	key := pairKey(from, to)
	steps, hasPure := pipelines[key]

	if LibreOfficeAvailable() && (enginePreferred[key] || (!hasPure && engineCanConvert(from, to))) {
		// Ekspor teks/HTML dari hasil impor PDF membuang isi, jadi tujuan itu tidak
		// boleh lewat engine. Jalur pure-Go di bawah tetap dipakai bila tersedia.
		if from == FormatPDF && !allowedFromPDF(to) {
			return Hop{}, false
		}
		return Hop{From: from, To: to, Engine: true}, true
	}
	if hasPure && adaptersReady(steps) {
		return Hop{From: from, To: to, Adapters: steps}, true
	}
	return Hop{}, false
}

// searchOrder mendahulukan HTML sebagai perantara supaya rantai mempertahankan
// struktur (heading, paragraf) lebih baik daripada lewat teks polos.
func searchOrder() []Format {
	out := make([]Format, 0, len(Formats))
	out = append(out, FormatHTML)
	for _, format := range Formats {
		if format != FormatHTML {
			out = append(out, format)
		}
	}
	return out
}

// findPath mencari rantai format terpendek dari from ke to (BFS, maksimal limit langkah).
func findPath(from, to Format, limit int) ([]Format, bool) {
	type node struct {
		format Format
		depth  int
	}

	parent := map[Format]Format{}
	queue := []node{{from, 0}}
	order := searchOrder()

	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		if cur.depth >= limit {
			continue
		}

		for _, next := range order {
			if next == from {
				continue
			}
			if _, seen := parent[next]; seen {
				continue
			}
			if _, ok := directHop(cur.format, next); !ok {
				continue
			}
			parent[next] = cur.format

			if next == to {
				path := []Format{to}
				for back := cur.format; ; back = parent[back] {
					path = append([]Format{back}, path...)
					if back == from {
						break
					}
				}
				return path, true
			}

			queue = append(queue, node{next, cur.depth + 1})
		}
	}
	return nil, false
}

// PipelineFor memilih pipeline: jalur langsung bila ada, kalau tidak rantai maksimal maxHops.
func PipelineFor(from, to Format) (Pipeline, bool) {
	if from == to {
		return Pipeline{}, false
	}
	if hop, ok := directHop(from, to); ok {
		return Pipeline{Hops: []Hop{hop}}, true
	}

	// PDF tidak memakai rantai: turunan hasil impor PDF (terutama TXT/HTML) kehilangan
	// isi, jadi rantai dari PDF hanya akan menghasilkan file kosong/rusak.
	if from == FormatPDF {
		return Pipeline{}, false
	}

	path, ok := findPath(from, to, maxHops)
	if !ok || len(path) < 3 {
		return Pipeline{}, false
	}

	hops := make([]Hop, 0, len(path)-1)
	for i := 0; i+1 < len(path); i++ {
		hop, ok := directHop(path[i], path[i+1])
		if !ok {
			return Pipeline{}, false
		}
		hops = append(hops, hop)
	}
	return Pipeline{Hops: hops}, true
}

// AvailableTargets mengembalikan format tujuan yang bisa dihasilkan dari from,
// termasuk yang hanya tercapai lewat rantai.
func AvailableTargets(from Format) []Format {
	var out []Format
	for _, to := range Formats {
		if _, ok := PipelineFor(from, to); ok {
			out = append(out, to)
		}
	}
	return out
}

// RunConversion menjalankan seluruh langkah pipeline atas data masukan.
func RunConversion(from, to Format, data []byte) ([]byte, error) {
	pipeline, ok := PipelineFor(from, to)
	if !ok {
		return nil, fmt.Errorf("konversi %s → %s belum didukung", from, to)
	}

	value := data
	for _, hop := range pipeline.Hops {
		if hop.Engine {
			next, err := ConvertWithLibreOffice(hop.From, hop.To, value)
			if err != nil {
				return nil, fmt.Errorf("%s → %s: %w", hop.From, hop.To, err)
			}
			value = next
			continue
		}

		for _, name := range hop.Adapters {
			fn, ok := adaptersMap[name]
			if !ok {
				return nil, fmt.Errorf("adapter %s tidak ditemukan", name)
			}
			next, err := fn(value)
			if err != nil {
				return nil, fmt.Errorf("%s → %s: %w", hop.From, hop.To, err)
			}
			value = next
		}
	}
	return value, nil
}
