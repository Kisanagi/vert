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
	text  string
	level int // 0 = teks biasa, 1..3 = heading
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
				return paragraphs, nil
			}
		}
	}

	out, err := runPdftotext(binary, "-layout", "-enc", "UTF-8", input, "-")
	if err != nil {
		return nil, err
	}
	paragraphs := splitParagraphs(string(out))
	if len(paragraphs) == 0 {
		return nil, errors.New("tidak ada teks yang bisa diekstrak (kemungkinan PDF hasil scan)")
	}
	return paragraphs, nil
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

// parseBBox mengubah XHTML -bbox-layout menjadi paragraf; heading dikenali dari
// tinggi baris relatif terhadap teks badan.
func parseBBox(data []byte) ([]pdfParagraph, error) {
	var doc bboxDocument
	if err := xml.Unmarshal(data, &doc); err != nil {
		return nil, err
	}

	type rawBlock struct {
		text   string
		height float64
	}

	var blocks []rawBlock
	for _, page := range doc.Pages {
		for _, block := range page.Blocks {
			var builder strings.Builder
			height := 0.0
			for _, line := range block.Lines {
				if h := line.YMax - line.YMin; h > height {
					height = h
				}
				for _, word := range line.Words {
					text := strings.TrimSpace(word.Text)
					if text == "" {
						continue
					}
					if builder.Len() > 0 {
						builder.WriteString(" ")
					}
					builder.WriteString(text)
				}
			}
			text := strings.TrimSpace(builder.String())
			if text == "" {
				continue
			}
			blocks = append(blocks, rawBlock{text: text, height: height})
		}
	}
	if len(blocks) == 0 {
		return nil, errors.New("tidak ada blok teks")
	}

	// Berat per tinggi (dibulatkan) untuk menebak ukuran teks badan.
	weights := map[int]float64{}
	for _, block := range blocks {
		key := int(math.Round(block.height))
		weights[key] += float64(len(block.text))
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

	paragraphs := make([]pdfParagraph, 0, len(blocks))
	for _, block := range blocks {
		paragraphs = append(paragraphs, pdfParagraph{
			text:  block.text,
			level: bboxHeadingLevel(block.height, body, block.text),
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

// splitParagraphs memecah keluaran -layout menjadi paragraf berdasarkan baris kosong.
func splitParagraphs(text string) []pdfParagraph {
	var paragraphs []pdfParagraph
	var chunk []string

	flush := func() {
		if len(chunk) == 0 {
			return
		}
		joined := strings.Join(chunk, " ")
		chunk = chunk[:0]
		joined = strings.Join(strings.Fields(joined), " ")
		if joined != "" {
			paragraphs = append(paragraphs, pdfParagraph{text: joined})
		}
	}

	for _, line := range strings.Split(text, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			flush()
			continue
		}
		chunk = append(chunk, trimmed)
	}
	flush()
	return paragraphs
}

// ---------- adapter ----------

// pdfToMarkdown mengekstrak PDF lalu merender paragraf sebagai Markdown.
func pdfToMarkdown(src []byte) ([]byte, error) {
	paragraphs, err := pdfParagraphs(src)
	if err != nil {
		return nil, err
	}

	var out strings.Builder
	for _, paragraph := range paragraphs {
		if paragraph.level > 0 {
			out.WriteString(strings.Repeat("#", paragraph.level) + " " + paragraph.text + "\n\n")
			continue
		}
		out.WriteString(paragraph.text + "\n\n")
	}
	return []byte(strings.TrimSpace(out.String()) + "\n"), nil
}

// pdfToText mengekstrak PDF menjadi teks polos (paragraf dipisah baris kosong).
func pdfToText(src []byte) ([]byte, error) {
	paragraphs, err := pdfParagraphs(src)
	if err != nil {
		return nil, err
	}

	var out strings.Builder
	for _, paragraph := range paragraphs {
		out.WriteString(paragraph.text + "\n\n")
	}
	return []byte(strings.TrimSpace(out.String()) + "\n"), nil
}
