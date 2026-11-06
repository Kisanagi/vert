package main

import (
	"regexp"
	"strings"
)

// asciiTableBorder mencocokkan garis tabel gaya ASCII, mis. "+------+------+".
var asciiTableBorder = regexp.MustCompile(`^\+(?:[-=]+\+)+$`)

// normalizeAsciiTables mengubah tabel bergaya ASCII ("+---+", "| sel |") menjadi
// tabel Markdown asli. Isi blok kode (```/~~~) tidak disentuh.
func normalizeAsciiTables(markdown string) string {
	lines := strings.Split(markdown, "\n")
	out := make([]string, 0, len(lines))

	inFence := false
	for i := 0; i < len(lines); i++ {
		line := lines[i]
		trimmed := strings.TrimSpace(line)

		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			inFence = !inFence
			out = append(out, line)
			continue
		}
		if inFence || !asciiTableBorder.MatchString(trimmed) {
			out = append(out, line)
			continue
		}

		// Kumpulkan baris tabel sampai ada baris yang bukan garis/isi tabel.
		var rows [][]string
		j := i + 1
		for ; j < len(lines); j++ {
			row := strings.TrimSpace(lines[j])
			if asciiTableBorder.MatchString(row) {
				continue // garis pemisah, dilewati
			}
			if !strings.HasPrefix(row, "|") {
				break
			}
			rows = append(rows, splitAsciiRow(row))
		}

		if len(rows) > 0 {
			if table, ok := asciiRowsToMarkdown(rows); ok {
				out = append(out, table...)
				i = j - 1
				continue
			}
		}
		out = append(out, line)
	}

	return strings.Join(out, "\n")
}

// splitAsciiRow memecah satu baris "| a | b |" menjadi sel-selnya.
func splitAsciiRow(line string) []string {
	trimmed := strings.TrimSpace(line)
	trimmed = strings.TrimPrefix(trimmed, "|")
	trimmed = strings.TrimSuffix(trimmed, "|")

	cells := strings.Split(trimmed, "|")
	for i := range cells {
		cells[i] = strings.TrimSpace(cells[i])
	}
	return cells
}

// asciiRowsToMarkdown menyusun tabel Markdown dari baris-baris hasil parsing.
// Baris pertama dipakai sebagai header (tabel Markdown mewajibkannya).
func asciiRowsToMarkdown(rows [][]string) ([]string, bool) {
	width := 0
	for _, row := range rows {
		if len(row) > width {
			width = len(row)
		}
	}
	if width == 0 {
		return nil, false
	}

	write := func(cells []string) string {
		parts := make([]string, width)
		for i := 0; i < width; i++ {
			cell := ""
			if i < len(cells) {
				cell = strings.TrimSpace(cells[i])
			}
			parts[i] = strings.ReplaceAll(cell, "|", "\\|")
		}
		return "| " + strings.Join(parts, " | ") + " |"
	}

	separator := make([]string, width)
	for i := range separator {
		separator[i] = "---"
	}

	out := make([]string, 0, len(rows)+1)
	out = append(out, write(rows[0]), write(separator))
	for _, row := range rows[1:] {
		out = append(out, write(row))
	}
	return out, true
}
