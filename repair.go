package main

import (
	"regexp"
	"strings"
)

// inlineTableSeparator menandai tabel yang "runtuh": seluruh baris tabel berada
// pada satu baris teks, mis. "| a | b | |---|---| | 1 | 2 |".
var inlineTableSeparator = regexp.MustCompile(`\|\s*:?-{3,}:?\s*\|`)

// numberedHeading mencocokkan baris bernomor pendek yang sebenarnya heading.
var numberedHeading = regexp.MustCompile(`^(\d+(?:\.\d+)*)\.?\s+([^.]{1,70})$`)

// needsRepair melaporkan apakah markdown tampak hasil ekstraksi PDF yang rusak.
// Penanda khasnya: butir "•" atau tabel yang runtuh dalam satu baris.
func needsRepair(markdown string) bool {
	return strings.Contains(markdown, "•") || inlineTableSeparator.MatchString(markdown)
}

// prepareMarkdown menyiapkan markdown sebelum dirender: perbaiki dulu bila
// tampak hasil ekstraksi rusak, lalu rapikan tabel gaya ASCII.
func prepareMarkdown(src []byte) string {
	text := string(src)
	if needsRepair(text) {
		text = repairMarkdown(text)
	}
	return normalizeAsciiTables(text)
}

// repairMarkdown memperbaiki markdown hasil ekstraksi buruk:
//   - tabel runtuh (semua baris pada satu baris) dipecah kembali per baris,
//   - blok lanjutan disambungkan ke tabel sebelumnya,
//   - butir "•" diubah menjadi daftar "- " dan dipisah per baris,
//   - baris bernomor pendek dijadikan heading.
//
// Hanya dipanggil bila needsRepair() bernilai benar.
func repairMarkdown(markdown string) string {
	lines := strings.Split(markdown, "\n")
	out := make([]string, 0, len(lines))
	lastColumns := 0

	for i := 0; i < len(lines); i++ {
		line := lines[i]
		trimmed := strings.TrimSpace(line)

		isTableLine := strings.HasPrefix(trimmed, "|") ||
			inlineTableSeparator.MatchString(line) ||
			(lastColumns > 0 && strings.Contains(line, "|"))

		if isTableLine {
			// Kumpulkan baris-baris beruntun yang mengandung '|' (tabel terbungkus).
			block := make([]string, 0, 8)
			for i < len(lines) && strings.Contains(lines[i], "|") {
				block = append(block, strings.TrimSpace(lines[i]))
				i++
			}
			i--
			joined := strings.Join(block, " ")

			if table, columns, ok := rebuildCollapsedTable(joined); ok {
				out = append(out, table...)
				lastColumns = columns
				continue
			}

			// Blok tanpa pemisah: kemungkinan lanjutan baris tabel sebelumnya.
			tokens := nonEmpty(strings.Split(joined, "|"))
			if lastColumns > 0 && len(tokens) >= lastColumns {
				// Sambungkan ke tabel sebelumnya: buang baris kosong pemisah.
				if n := len(out); n > 0 && out[n-1] == "" {
					out = out[:n-1]
				}
				for k := 0; k+lastColumns <= len(tokens); k += lastColumns {
					out = append(out, tableRow(tokens[k:k+lastColumns], lastColumns))
				}
				out = append(out, "")
				continue
			}

			out = append(out, block...)
			lastColumns = 0
			continue
		}

		if trimmed != "" {
			lastColumns = 0
		}
		out = append(out, splitInlineBullets(line)...)
	}

	return restoreNumberedHeadings(joinTableGaps(strings.Join(out, "\n")))
}

// isTableLine menandai baris yang merupakan baris tabel Markdown.
func isTableLine(text string) bool {
	return strings.HasPrefix(strings.TrimSpace(text), "|")
}

// joinTableGaps membuang baris kosong yang berada di tengah tabel, karena baris
// kosong memutus tabel menjadi paragraf biasa.
func joinTableGaps(markdown string) string {
	lines := strings.Split(markdown, "\n")
	out := make([]string, 0, len(lines))

	for i, line := range lines {
		if strings.TrimSpace(line) == "" {
			previous := lastNonEmpty(out)
			var next string
			for j := i + 1; j < len(lines); j++ {
				if strings.TrimSpace(lines[j]) != "" {
					next = lines[j]
					break
				}
			}
			if isTableLine(previous) && isTableLine(next) {
				continue
			}
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}

func lastNonEmpty(lines []string) string {
	for i := len(lines) - 1; i >= 0; i-- {
		if strings.TrimSpace(lines[i]) != "" {
			return lines[i]
		}
	}
	return ""
}

// splitInlineBullets memecah baris yang memuat beberapa butir "•" menjadi daftar.
func splitInlineBullets(line string) []string {
	if !strings.Contains(line, "•") {
		return []string{line}
	}

	parts := strings.Split(line, "•")
	out := make([]string, 0, len(parts))
	if head := strings.TrimSpace(parts[0]); head != "" {
		out = append(out, head)
	}
	for _, part := range parts[1:] {
		if item := strings.TrimSpace(part); item != "" {
			out = append(out, "- "+item)
		}
	}
	if len(out) == 0 {
		return []string{line}
	}
	return out
}

// tableRow menyusun satu baris tabel Markdown dengan jumlah kolom tetap.
func tableRow(cells []string, columns int) string {
	parts := make([]string, columns)
	for i := 0; i < columns; i++ {
		if i < len(cells) {
			parts[i] = cells[i]
		}
	}
	return "| " + strings.Join(parts, " | ") + " |"
}

// rebuildCollapsedTable menyusun tabel Markdown dari satu baris panjang yang
// memuat seluruh baris tabel. Jumlah kolom diambil dari deret pemisah "---".
func rebuildCollapsedTable(joined string) ([]string, int, bool) {
	tokens := strings.Split(joined, "|")
	for i := range tokens {
		tokens[i] = strings.TrimSpace(tokens[i])
	}

	separatorStart := -1
	for i, token := range tokens {
		if isSeparatorToken(token) {
			separatorStart = i
			break
		}
	}
	if separatorStart < 0 {
		return nil, 0, false
	}

	columns := 0
	for separatorStart+columns < len(tokens) && isSeparatorToken(tokens[separatorStart+columns]) {
		columns++
	}
	if columns == 0 {
		return nil, 0, false
	}

	header := nonEmpty(tokens[:separatorStart])
	data := nonEmpty(tokens[separatorStart+columns:])
	if len(header) == 0 && len(data) == 0 {
		return nil, 0, false
	}

	// Bila sebelum pemisah ada token lebih banyak dari jumlah kolom, sisa awalnya
	// adalah teks biasa yang ikut tergabung (mis. judul tabel), bukan sel header.
	var preamble []string
	if len(header) > columns {
		preamble = header[:len(header)-columns]
		header = header[len(header)-columns:]
	}

	out := make([]string, 0, 3+len(data)/columns)
	if len(preamble) > 0 {
		out = append(out, strings.Join(preamble, " "), "")
	}
	if len(header) >= columns {
		out = append(out, tableRow(header[:columns], columns))
	} else {
		out = append(out, tableRow(nil, columns))
	}

	separator := make([]string, columns)
	for i := range separator {
		separator[i] = "---"
	}
	out = append(out, "| "+strings.Join(separator, " | ")+" |")

	for i := 0; i+columns <= len(data); i += columns {
		out = append(out, tableRow(data[i:i+columns], columns))
	}
	out = append(out, "")
	return out, columns, true
}

func isSeparatorToken(token string) bool {
	trimmed := strings.Trim(token, ":")
	return len(trimmed) >= 3 && strings.Trim(trimmed, "-") == ""
}

func nonEmpty(tokens []string) []string {
	out := make([]string, 0, len(tokens))
	for _, token := range tokens {
		if token != "" {
			out = append(out, token)
		}
	}
	return out
}

// restoreNumberedHeadings mengubah baris bernomor pendek menjadi heading.
// Level mengikuti kedalaman nomor: "1." -> ##, "6.1" -> ###, "6.1.2" -> ####.
func restoreNumberedHeadings(markdown string) string {
	lines := strings.Split(markdown, "\n")
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, "|") || strings.HasPrefix(trimmed, "- ") {
			continue
		}

		match := numberedHeading.FindStringSubmatch(trimmed)
		if match == nil {
			continue
		}
		// Heading biasanya berdiri sendiri: baris sebelumnya kosong.
		if i > 0 && strings.TrimSpace(lines[i-1]) != "" {
			continue
		}
		if len([]rune(trimmed)) > 80 {
			continue
		}

		level := 1 + strings.Count(match[1], ".")
		if level < 2 {
			level = 2
		}
		if level > 4 {
			level = 4
		}
		lines[i] = strings.Repeat("#", level) + " " + trimmed
	}
	return strings.Join(lines, "\n")
}
