package main

import (
	"bytes"
	"encoding/csv"
	"errors"
	"io"
	"regexp"
	"strings"

	"github.com/xuri/excelize/v2"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
)

// Adapter mengubah data masukan menjadi data keluaran (di memori).
type Adapter func([]byte) ([]byte, error)

var adaptersMap = map[string]Adapter{
	"mdToText":  mdToText,
	"mdToHTML":  mdToHTML,
	"xlsxToCSV": xlsxToCSV,
	"csvToXLSX": csvToXLSX,

	"pdfToMarkdown": pdfToMarkdown,
	"pdfToText":     pdfToText,
	"pdfToDocx":     pdfToDocx,
}

var (
	fencedCode = regexp.MustCompile("(?ms)^```.*?^```")
	mdHeading  = regexp.MustCompile(`(?m)^\s{0,3}#{1,6}\s*`)
	mdImage    = regexp.MustCompile(`!\[(.*?)\]\([^)]*\)`)
	mdLink     = regexp.MustCompile(`\[(.*?)\]\([^)]*\)`)
	mdBoldStar = regexp.MustCompile(`\*\*(.+?)\*\*`)
	mdBoldUnd  = regexp.MustCompile(`__(.+?)__`)
	mdEmphStar = regexp.MustCompile(`\*(.+?)\*`)
	mdEmphUnd  = regexp.MustCompile(`_(.+?)_`)
	mdCode     = regexp.MustCompile("`([^`]+)`")
	mdQuote    = regexp.MustCompile(`(?m)^\s*>\s?`)
	mdStrike   = regexp.MustCompile(`~~(.+?)~~`)
	mdTask     = regexp.MustCompile(`\[[ xX]\]\s*`)
)

// mdToText membuang markup Markdown seadanya. Cukup untuk konversi teks, bukan render.
func mdToText(src []byte) ([]byte, error) {
	text := fencedCode.ReplaceAllString(prepareMarkdown(src), "")
	text = mdHeading.ReplaceAllString(text, "")
	for _, re := range []*regexp.Regexp{
		mdImage, mdLink, mdBoldStar, mdBoldUnd, mdEmphStar, mdEmphUnd, mdCode, mdQuote,
	} {
		text = re.ReplaceAllString(text, "$1")
	}
	text = mdStrike.ReplaceAllString(text, "$1")
	text = mdTask.ReplaceAllString(text, "")
	text = stripMarkdownTables(text)

	lines := strings.Split(strings.TrimSpace(text), "\n")
	return []byte(strings.Join(lines, "\n") + "\n"), nil
}

// isTableSeparator menandai baris pemisah tabel Markdown, mis. "| --- | --- |".
func isTableSeparator(line string) bool {
	if !strings.Contains(line, "|") || !strings.Contains(line, "-") {
		return false
	}
	for _, r := range line {
		switch r {
		case '|', '-', ':', ' ', '\t':
		default:
			return false
		}
	}
	return true
}

// stripMarkdownTables mengubah tabel Markdown menjadi baris teks biasa.
func stripMarkdownTables(text string) string {
	lines := strings.Split(text, "\n")
	out := make([]string, 0, len(lines))

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "|") {
			out = append(out, line)
			continue
		}
		if isTableSeparator(trimmed) {
			continue
		}

		cells := strings.Split(strings.Trim(trimmed, "|"), "|")
		parts := make([]string, 0, len(cells))
		for _, cell := range cells {
			parts = append(parts, strings.TrimSpace(cell))
		}
		out = append(out, strings.Join(parts, "  "))
	}
	return strings.Join(out, "\n")
}

// markdownRenderer memakai ekstensi GFM: tabel, strikethrough, task list, dan
// autolink. Tanpa ini, sintaks seperti tabel Markdown tercetak sebagai teks
// mentah ("| --- | --- |") di HTML/PDF hasil konversi.
var markdownRenderer = goldmark.New(goldmark.WithExtensions(extension.GFM))

// printCSS menentukan tipografi PDF hasil MD->PDF. Mesin cetak Edge/Chromium
// membaca CSS ini, dan dukungan CSS-nya jauh lebih lengkap daripada impor HTML
// mesin Office: ukuran halaman, margin, dan latar tabel benar-benar diterapkan.
const printCSS = `
  @page { size: A4; margin: 2cm 2.2cm; }
  body {
    font-family: "Segoe UI", Calibri, Arial, sans-serif;
    font-size: 11pt; line-height: 1.5; color: #111827;
    -webkit-print-color-adjust: exact; print-color-adjust: exact;
  }
  h1, h2, h3, h4, h5, h6 {
    font-family: "Segoe UI", Calibri, Arial, sans-serif;
    font-weight: bold; line-height: 1.25; margin: 1.1em 0 0.45em;
  }
  h1 { font-size: 20pt; }
  h2 { font-size: 15pt; }
  h3 { font-size: 13pt; }
  h4, h5, h6 { font-size: 11pt; }
  p, ul, ol, blockquote, table, pre { margin: 0 0 0.7em; }
  ul, ol { padding-left: 1.4em; }
  pre {
    font-family: Consolas, "Cascadia Mono", monospace; font-size: 9.5pt;
    background-color: #f3f4f6; padding: 8pt; border: 1px solid #d1d5db;
    white-space: pre-wrap;
  }
  code { font-family: Consolas, "Cascadia Mono", monospace; font-size: 9.5pt; }
  table { border-collapse: collapse; width: 100%; }
  th, td { border: 1px solid #9ca3af; padding: 4pt 6pt; overflow-wrap: break-word; }
  th { background-color: #f3f4f6; font-weight: bold; }
  blockquote { padding-left: 10pt; border-left: 3px solid #d1d5db; color: #4b5563; }
  hr { border: none; border-top: 1px solid #e5e7eb; margin: 1em 0; }
  img { max-width: 100%; }
`

// mdToHTML merender Markdown menjadi dokumen HTML utuh memakai goldmark.
// Sengaja dibungkus <html>/<head>/<meta charset> agar mesin tujuan (Edge atau
// Office) menerima dokumen lengkap beserta stylesheet-nya, bukan fragmen.
func mdToHTML(src []byte) ([]byte, error) {
	normalized := prepareMarkdown(src)

	var body bytes.Buffer
	if err := markdownRenderer.Convert([]byte(normalized), &body); err != nil {
		return nil, err
	}

	var out bytes.Buffer
	out.WriteString("<!DOCTYPE html>\n<html><head><meta charset=\"utf-8\"><title>Dokumen</title><style>")
	out.WriteString(printCSS)
	out.WriteString("</style></head><body>\n")
	out.WriteString(withTableBorders(body.String()))
	out.WriteString("\n</body></html>\n")
	return out.Bytes(), nil
}

// withTableBorders menambahkan atribut border pada <table>. Word dan Excel kadang
// mengabaikan `border` dari CSS pada sel HTML, tetapi menghormati atribut border
// pada tabel.
func withTableBorders(html string) string {
	return strings.ReplaceAll(html, "<table>", `<table border="1" cellspacing="0" cellpadding="4">`)
}

// xlsxToCSV mengekspor sheet pertama sebuah XLSX ke CSV.
func xlsxToCSV(src []byte) ([]byte, error) {
	file, err := excelize.OpenReader(bytes.NewReader(src))
	if err != nil {
		return nil, err
	}
	defer file.Close()

	sheets := file.GetSheetList()
	if len(sheets) == 0 {
		return nil, errors.New("xlsx tidak punya sheet")
	}
	rows, err := file.GetRows(sheets[0])
	if err != nil {
		return nil, err
	}

	var buf bytes.Buffer
	writer := csv.NewWriter(&buf)
	for _, row := range rows {
		if err := writer.Write(row); err != nil {
			return nil, err
		}
	}
	writer.Flush()
	return buf.Bytes(), writer.Error()
}

// csvToXLSX mengubah CSV menjadi XLSX (sheet pertama).
func csvToXLSX(src []byte) ([]byte, error) {
	reader := csv.NewReader(bytes.NewReader(src))
	reader.FieldsPerRecord = -1

	file := excelize.NewFile()
	sheet := file.GetSheetName(0)

	row := 1
	for {
		record, err := reader.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			file.Close()
			return nil, err
		}
		for i, value := range record {
			cell, err := excelize.CoordinatesToCellName(i+1, row)
			if err != nil {
				file.Close()
				return nil, err
			}
			if err := file.SetCellValue(sheet, cell, value); err != nil {
				file.Close()
				return nil, err
			}
		}
		row++
	}

	buf, err := file.WriteToBuffer()
	file.Close()
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
