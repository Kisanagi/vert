package main

import (
	"path/filepath"
	"strings"
)

// Format adalah id format dokumen yang didukung.
type Format string

const (
	FormatPDF  Format = "pdf"
	FormatDOCX Format = "docx"
	FormatODT  Format = "odt"
	FormatRTF  Format = "rtf"
	FormatTXT  Format = "txt"
	FormatHTML Format = "html"
	FormatMD   Format = "md"
	FormatXLSX Format = "xlsx"
	FormatODS  Format = "ods"
	FormatCSV  Format = "csv"
	FormatPPTX Format = "pptx"
	FormatODP  Format = "odp"
)

// Formats menentukan urutan tampil di UI.
var Formats = []Format{
	FormatPDF, FormatDOCX, FormatODT, FormatRTF, FormatTXT, FormatHTML, FormatMD,
	FormatXLSX, FormatODS, FormatCSV, FormatPPTX, FormatODP,
}

var extToFormat = map[string]Format{
	".pdf": FormatPDF, ".docx": FormatDOCX, ".odt": FormatODT, ".rtf": FormatRTF,
	".txt": FormatTXT, ".text": FormatTXT, ".html": FormatHTML, ".htm": FormatHTML,
	".md": FormatMD, ".markdown": FormatMD, ".mdown": FormatMD, ".mkd": FormatMD, ".mkdn": FormatMD,
	".xlsx": FormatXLSX, ".ods": FormatODS, ".csv": FormatCSV,
	".pptx": FormatPPTX, ".odp": FormatODP,
}

// DetectFormat menebak format dari nama/path berkas berdasarkan ekstensinya.
func DetectFormat(input string) (Format, bool) {
	ext := strings.ToLower(filepath.Ext(input))
	f, ok := extToFormat[ext]
	return f, ok
}

// Family mengelompokkan format; menentukan pasangan mana yang wajar ditangani LibreOffice.
type Family string

const (
	FamilyPDF     Family = "pdf"
	FamilyWriter  Family = "writer"
	FamilyCalc    Family = "calc"
	FamilyImpress Family = "impress"
	FamilyMarkup  Family = "markup"
)

var formatFamily = map[Format]Family{
	FormatPDF:  FamilyPDF,
	FormatDOCX: FamilyWriter, FormatODT: FamilyWriter, FormatRTF: FamilyWriter,
	FormatTXT: FamilyWriter, FormatHTML: FamilyWriter,
	FormatMD:   FamilyMarkup,
	FormatXLSX: FamilyCalc, FormatODS: FamilyCalc, FormatCSV: FamilyCalc,
	FormatPPTX: FamilyImpress, FormatODP: FamilyImpress,
}

// FamilyOf mengembalikan keluarga sebuah format.
func FamilyOf(f Format) Family { return formatFamily[f] }
