package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	maxSearchDepth   = 6
	maxSearchResults = 300
)

// skippedDirs diabaikan saat penelusuran agar pencarian tidak menjelajah folder
// sistem atau folder berat yang tidak relevan.
var skippedDirs = map[string]bool{
	"node_modules": true, ".git": true, ".svn": true, ".hg": true,
	"AppData": true, "Windows": true, "ProgramData": true, "$Recycle.Bin": true,
	"System Volume Information": true, "dist": true, "build": true,
	".cache": true, "__pycache__": true, "vendor": true, ".venv": true, "venv": true,
}

// SearchResult adalah satu berkas yang cocok dengan pencarian.
type SearchResult struct {
	Path string
	Size int64
}

// FindFiles menelusuri root secara rekursif dan mengembalikan berkas yang bisa
// dikonversi dan namanya mengandung query (tanpa membedakan huruf besar/kecil).
// Query kosong berarti "semua berkas yang bisa dikonversi".
func FindFiles(root, query string, limit int) []SearchResult {
	needle := strings.ToLower(query)
	results := make([]SearchResult, 0, 32)

	var walk func(dir string, depth int)
	walk = func(dir string, depth int) {
		if len(results) >= limit || depth > maxSearchDepth {
			return
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			return
		}
		for _, entry := range entries {
			if len(results) >= limit {
				return
			}
			name := entry.Name()
			if entry.IsDir() {
				if skippedDirs[name] {
					continue
				}
				walk(filepath.Join(dir, name), depth+1)
				continue
			}
			if _, ok := DetectFormat(name); !ok {
				continue
			}
			if needle != "" && !strings.Contains(strings.ToLower(name), needle) {
				continue
			}
			var size int64
			if info, err := entry.Info(); err == nil {
				size = info.Size()
			}
			results = append(results, SearchResult{Path: filepath.Join(dir, name), Size: size})
		}
	}

	walk(root, 0)
	return results
}

// humanSize memformat ukuran berkas menjadi bentuk ringkas.
func humanSize(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	value := float64(bytes)
	for _, suffix := range []string{"KB", "MB", "GB", "TB"} {
		value /= unit
		if value < unit {
			return fmt.Sprintf("%.1f %s", value, suffix)
		}
	}
	return fmt.Sprintf("%.1f PB", value/unit)
}
