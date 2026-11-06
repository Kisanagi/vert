package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

var cachedPdftotext *string

// pdftotextCandidates mendata lokasi biner pdftotext (Poppler).
//
// Catatan Windows: Git for Windows membawa pdftotext di mingw64, dan winget
// memasang Poppler ke folder paket tersendiri (nama folder berisi versi).
func pdftotextCandidates() []string {
	var out []string

	if override := os.Getenv("MD_CONVERTER_PDFTOTEXT"); override != "" {
		out = append(out, override)
	}
	if found, err := exec.LookPath("pdftotext"); err == nil {
		out = append(out, found)
	}

	if runtime.GOOS == "windows" {
		out = append(out,
			`C:\Program Files\Git\mingw64\bin\pdftotext.exe`,
			`C:\Program Files\poppler\Library\bin\pdftotext.exe`,
			`C:\Program Files\poppler\bin\pdftotext.exe`,
		)
		if local := os.Getenv("LOCALAPPDATA"); local != "" {
			pattern := filepath.Join(local, "Microsoft", "WinGet", "Packages",
				"oschwartz10612.Poppler_*", "*", "Library", "bin", "pdftotext.exe")
			if matches, err := filepath.Glob(pattern); err == nil {
				out = append(out, matches...)
			}
		}
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

// pdftotextToText mengekstrak teks PDF memakai pdftotext (Poppler).
func pdftotextToText(src []byte) ([]byte, error) {
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

	// "-" sebagai berkas keluaran berarti tulis ke stdout.
	cmd := exec.Command(binary, "-enc", "UTF-8", input, "-")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("pdftotext gagal: %v: %s", err, strings.TrimSpace(stderr.String()))
	}

	text := strings.TrimSpace(stdout.String())
	if text == "" {
		return nil, errors.New("tidak ada teks yang bisa diekstrak (kemungkinan PDF hasil scan)")
	}
	return []byte(text + "\n"), nil
}
