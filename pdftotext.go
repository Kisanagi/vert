package main

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

var (
	cachedPdftotext *string
	cachedBBox      *bool
)

// pdfParagraph adalah satu blok teks hasil ekstraksi PDF.
type pdfParagraph struct {
	text   string
	level  int  // 0 = teks biasa, 1..3 = heading
	bullet bool // true = butir daftar
}

// pdftotextCandidates mendata lokasi biner pdftotext.
//
// Catatan Windows: Git for Windows membawa pdftotext (Xpdf), dan winget memasang
// Poppler ke folder paket tersendiri (nama folder berisi versi).
func pdftotextCandidates() []string {
	var out []string

	if override := os.Getenv("MD_CONVERTER_PDFTOTEXT"); override != "" {
		out = append(out, override)
	}
	if found, err := exec.LookPath("pdftotext"); err == nil {
		out = append(out, found)
	}

	if runtime.GOOS == "windows" {
		if local := os.Getenv("LOCALAPPDATA"); local != "" {
			pattern := filepath.Join(local, "Microsoft", "WinGet", "Packages",
				"oschwartz10612.Poppler_*", "*", "Library", "bin", "pdftotext.exe")
			if matches, err := filepath.Glob(pattern); err == nil {
				out = append(out, matches...)
			}
		}
		out = append(out,
			`C:\Program Files\Git\mingw64\bin\pdftotext.exe`,
			`C:\Program Files\poppler\Library\bin\pdftotext.exe`,
			`C:\Program Files\poppler\bin\pdftotext.exe`,
		)
	} else {
		out = append(out,
			"/usr/bin/pdftotext",
			"/usr/local/bin/pdftotext",
			"/opt/homebrew/bin/pdftotext",
		)
	}

	return out
}

// resolvePdftotext mengembalikan path biner pdftotext yang ada, atau "".
func resolvePdftotext() string {
	if cachedPdftotext != nil {
		return *cachedPdftotext
	}

	found := ""
	for _, candidate := range pdftotextCandidates() {
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			found = candidate
			break
		}
	}
	cachedPdftotext = &found
	return found
}

// pdftotextAvailable melaporkan apakah ekstraktor PDF tersedia.
func pdftotextAvailable() bool { return resolvePdftotext() != "" }

// supportsBBox menandai biner Poppler (punya -bbox-layout). Xpdf tidak punya.
func supportsBBox(binary string) bool {
	if cachedBBox != nil {
		return *cachedBBox
	}

	cmd := exec.Command(binary, "-v")
	var report bytes.Buffer
	cmd.Stdout = &report
	cmd.Stderr = &report
	_ = cmd.Run() // -v keluar dengan kode bukan nol; kita hanya butuh teksnya

	parsed := strings.Contains(strings.ToLower(report.String()), "poppler")
	cachedBBox = &parsed
	return parsed
}

func runPdftotext(binary string, args ...string) ([]byte, error) {
	cmd := exec.Command(binary, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("pdftotext gagal: %v: %s", err, strings.TrimSpace(stderr.String()))
	}
	return stdout.Bytes(), nil
}

// pdfParagraphs mengekstrak PDF menjadi daftar paragraf. Memakai -bbox-layout
// (Poppler) agar blok dan tinggi baris terbaca, dengan fallback -layout.
func pdfParagraphs(src []byte) ([]pdfParagraph, error) {
	binary := resolvePdftotext()
	if binary == "" {
		return nil, errors.New("pdftotext tidak tersedia")
	}
	if len(src) == 0 {
		return nil, errors.New("berkas PDF kosong")
	}

	dir, err := os.MkdirTemp("", "vert-pdf-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)

	input := filepath.Join(dir, "input.pdf")
	if err := os.WriteFile(input, src, 0o644); err != nil {
		return nil, err
	}

	if supportsBBox(binary) {
		if out, err := runPdftotext(binary, "-bbox-layout", input, "-"); err == nil {
			if paragraphs, err := parseBBox(out); err == nil && len(paragraphs) > 0 {
				if cleaned := dropArtifacts(paragraphs); len(cleaned) > 0 {
					return cleaned, nil
				}
			}
		}
	}

	out, err := runPdftotext(binary, "-layout", "-enc", "UTF-8", input, "-")
	if err != nil {
		return nil, err
	}
	paragraphs := splitParagraphs(string(out))
	paragraphs = dropArtifacts(paragraphs)
	if len(paragraphs) == 0 {
		return nil, errors.New("tidak ada teks yang bisa diekstrak (kemungkinan PDF hasil scan)")
	}
	return paragraphs, nil
}

// isArtifactLine menandai baris yang hanya berisi karakter garis tabel, sehingga
// tidak layak masuk hasil konversi ("+---+", "|---|", "─────").
func isArtifactLine(text string) bool {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return true
	}
	for _, r := range trimmed {
		switch r {
		case '+', '-', '=', '|', '_', ' ', '\t', '·', '*',
			'─', '│', '┌', '┐', '└', '┘', '├', '┤', '┬', '┴', '┼',
			'━', '┃', '═', '║', '╔', '╗', '╚', '╝':
		default:
			return false
		}
	}
	return true
}

// dropArtifacts membuang paragraf yang isinya cuma garis tabel.
func dropArtifacts(paragraphs []pdfParagraph) []pdfParagraph {
	out := paragraphs[:0]
	for _, paragraph := range paragraphs {
		if isArtifactLine(paragraph.text) {
			continue
		}
		out = append(out, paragraph)
	}
	return out
}

// ---------- mode terstruktur (Poppler -bbox-layout) ----------

type bboxLine struct {
	YMin  float64 `xml:"yMin,attr"`
	YMax  float64 `xml:"yMax,attr"`
	Words []struct {
		Text string `xml:",chardata"`
	} `xml:"word"`
}

type bboxBlock struct {
	Lines []bboxLine `xml:"line"`
}

type bboxPage struct {
	Blocks []bboxBlock `xml:"flow>block"`
}

type bboxDocument struct {
	Pages []bboxPage `xml:"body>doc>page"`
}

// listMarkers adalah penanda butir daftar yang dikenali di awal baris.
var listMarkers = []string{"•", "◦", "▪", "‣", "·", "-", "*", "–", "—"}

// stripBullet memisahkan penanda butir daftar dari isinya.
func stripBullet(line string) (string, bool) {
	for _, marker := range listMarkers {
		if line == marker {
			return "", true
		}
		if strings.HasPrefix(line, marker+" ") {
			return strings.TrimSpace(strings.TrimPrefix(line, marker)), true
		}
	}

	// Penanda bernomor: "1.", "2)", atau "1.2".
	if i := strings.IndexByte(line, ' '); i > 0 {
		head := line[:i]
		if strings.HasSuffix(head, ".") || strings.HasSuffix(head, ")") {
			digits := strings.TrimRight(head, ".)")
			if digits != "" && strings.Trim(digits, "0123456789") == "" {
				return strings.TrimSpace(line[i+1:]), true
			}
		}
	}
	return "", false
}

// parseBBox mengubah XHTML -bbox-layout menjadi paragraf; heading dikenali dari
// tinggi baris relatif terhadap teks badan, butir daftar dari penandanya.
func parseBBox(data []byte) ([]pdfParagraph, error) {
	var doc bboxDocument
	if err := xml.Unmarshal(data, &doc); err != nil {
		return nil, err
	}

	type rawParagraph struct {
		text   string
		height float64
		bullet bool
	}

	var raw []rawParagraph
	for _, page := range doc.Pages {
		for _, block := range page.Blocks {
			height := 0.0
			for _, line := range block.Lines {
				if h := line.YMax - line.YMin; h > height {
					height = h
				}
			}

			var current strings.Builder
			bullet := false
			flush := func() {
				text := strings.TrimSpace(strings.Join(strings.Fields(current.String()), " "))
				current.Reset()
				if text != "" {
					raw = append(raw, rawParagraph{text: text, height: height, bullet: bullet})
				}
				bullet = false
			}

			for _, line := range block.Lines {
				words := make([]string, 0, len(line.Words))
				for _, word := range line.Words {
					if text := strings.TrimSpace(word.Text); text != "" {
						words = append(words, text)
					}
				}
				if len(words) == 0 {
					continue
				}
				text := strings.Join(words, " ")

				if rest, ok := stripBullet(text); ok {
					flush()
					current.WriteString(rest)
					bullet = true
					continue
				}

				if current.Len() > 0 {
					current.WriteString(" ")
				}
				current.WriteString(text)
			}
			flush()
		}
	}
	if len(raw) == 0 {
		return nil, errors.New("tidak ada blok teks")
	}

	// Berat per tinggi (dibulatkan) untuk menebak ukuran teks badan.
	weights := map[int]float64{}
	for _, paragraph := range raw {
		key := int(math.Round(paragraph.height))
		weights[key] += float64(len(paragraph.text))
	}
	bodyKey, bodyWeight := 0, -1.0
	for key, weight := range weights {
		if weight > bodyWeight {
			bodyKey, bodyWeight = key, weight
		}
	}
	body := float64(bodyKey)
	if body <= 0 {
		body = 12
	}

	paragraphs := make([]pdfParagraph, 0, len(raw))
	for _, paragraph := range raw {
		level := 0
		if !paragraph.bullet {
			level = bboxHeadingLevel(paragraph.height, body, paragraph.text)
		}
		paragraphs = append(paragraphs, pdfParagraph{
			text:   paragraph.text,
			level:  level,
			bullet: paragraph.bullet,
		})
	}
	return paragraphs, nil
}

// bboxHeadingLevel menentukan level heading dari tinggi baris relatif teks badan.
func bboxHeadingLevel(height, body float64, text string) int {
	if body <= 0 || height <= 0 || len(text) > 100 {
		return 0
	}
	if strings.ContainsAny(text[len(text)-1:], ".,;:") {
		return 0
	}

	ratio := height / body
	switch {
	case ratio >= 1.7:
		return 1
	case ratio >= 1.35:
		return 2
	case ratio >= 1.2:
		return 3
	}
	return 0
}

// ---------- fallback (-layout) ----------

// splitParagraphs memecah keluaran -layout menjadi paragraf: baris kosong memisahkan
// paragraf, penanda butir memulai butir daftar baru.
func splitParagraphs(text string) []pdfParagraph {
	var paragraphs []pdfParagraph
	var current strings.Builder
	bullet := false

	flush := func() {
		joined := strings.Join(strings.Fields(current.String()), " ")
		current.Reset()
		if joined != "" {
			paragraphs = append(paragraphs, pdfParagraph{text: joined, bullet: bullet})
		}
		bullet = false
	}

	for _, line := range strings.Split(text, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			flush()
			continue
		}
		if rest, ok := stripBullet(trimmed); ok {
			flush()
			current.WriteString(rest)
			bullet = true
			continue
		}
		if current.Len() > 0 {
			current.WriteString(" ")
		}
		current.WriteString(trimmed)
	}
	flush()
	return paragraphs
}

// ---------- adapter ----------

// renderParagraphs menulis paragraf sebagai Markdown (atau teks polos bila tanpaHeading).
func renderParagraphs(paragraphs []pdfParagraph, withHeading bool) []byte {
	var out strings.Builder
	previousBullet := false

	for _, paragraph := range paragraphs {
		if paragraph.bullet {
			out.WriteString("- " + paragraph.text + "\n")
			previousBullet = true
			continue
		}
		if previousBullet {
			out.WriteString("\n")
			previousBullet = false
		}
		if withHeading && paragraph.level > 0 {
			out.WriteString(strings.Repeat("#", paragraph.level) + " " + paragraph.text + "\n\n")
			continue
		}
		out.WriteString(paragraph.text + "\n\n")
	}
	if previousBullet {
		out.WriteString("\n")
	}
	return []byte(strings.TrimSpace(out.String()) + "\n")
}

// pdfToMarkdown mengekstrak PDF lalu merender paragraf sebagai Markdown.
func pdfToMarkdown(src []byte) ([]byte, error) {
	paragraphs, err := pdfParagraphs(src)
	if err != nil {
		return nil, err
	}
	return renderParagraphs(paragraphs, true), nil
}

// pdfToText mengekstrak PDF menjadi teks polos (paragraf dipisah baris kosong).
func pdfToText(src []byte) ([]byte, error) {
	paragraphs, err := pdfParagraphs(src)
	if err != nil {
		return nil, err
	}
	return renderParagraphs(paragraphs, false), nil
}
