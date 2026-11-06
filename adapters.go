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
)

// Adapter mengubah data masukan menjadi data keluaran (di memori).
type Adapter func([]byte) ([]byte, error)

var adaptersMap = map[string]Adapter{
	"mdToText":  mdToText,
	"mdToHTML":  mdToHTML,
	"xlsxToCSV": xlsxToCSV,
	"csvToXLSX": csvToXLSX,
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
)

// mdToText membuang markup Markdown seadanya. Cukup untuk konversi teks, bukan render.
func mdToText(src []byte) ([]byte, error) {
	text := fencedCode.ReplaceAllString(string(src), "")
	text = mdHeading.ReplaceAllString(text, "")
	for _, re := range []*regexp.Regexp{
		mdImage, mdLink, mdBoldStar, mdBoldUnd, mdEmphStar, mdEmphUnd, mdCode, mdQuote,
	} {
		text = re.ReplaceAllString(text, "$1")
	}
	lines := strings.Split(strings.TrimSpace(text), "\n")
	return []byte(strings.Join(lines, "\n") + "\n"), nil
}

// mdToHTML merender Markdown menjadi dokumen HTML utuh memakai goldmark.
// Sengaja dibungkus <html>/<head>/<meta charset> agar LibreOffice mengimpor
// seluruh isi dengan benar (fragmen saja membuat sebagian konten terlewat).
func mdToHTML(src []byte) ([]byte, error) {
	var body bytes.Buffer
	if err := goldmark.New().Convert(src, &body); err != nil {
		return nil, err
	}

	var out bytes.Buffer
	out.WriteString("<!DOCTYPE html>\n<html><head><meta charset=\"utf-8\"><title>Dokumen</title></head><body>\n")
	out.Write(body.Bytes())
	out.WriteString("\n</body></html>\n")
	return out.Bytes(), nil
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
