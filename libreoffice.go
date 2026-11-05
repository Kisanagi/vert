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
	"time"
)

const convertTimeout = 120 * time.Second

var cachedSoffice *string

// sofficeCandidates mendata lokasi biner LibreOffice yang mungkin ada.
func sofficeCandidates() []string {
	var out []string
	if v := os.Getenv("MD_CONVERTER_SOFFICE"); v != "" {
		out = append(out, v)
	}
	switch runtime.GOOS {
	case "windows":
		out = append(out,
			`C:\Program Files\LibreOffice\program\soffice.exe`,
			`C:\Program Files (x86)\LibreOffice\program\soffice.exe`)
	case "darwin":
		out = append(out, "/Applications/LibreOffice.app/Contents/MacOS/soffice")
	default:
		out = append(out, "/usr/bin/soffice", "/usr/local/bin/soffice", "/snap/bin/libreoffice")
	}
	return out
}

// DetectSoffice mengembalikan path biner LibreOffice bila ada. Hasil di-cache.
func DetectSoffice() (string, bool) {
	if cachedSoffice != nil {
		return *cachedSoffice, *cachedSoffice != ""
	}
	found := ""
	for _, candidate := range sofficeCandidates() {
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			found = candidate
			break
		}
	}
	cachedSoffice = &found
	return found, found != ""
}

// LibreOfficeAvailable melaporkan apakah engine eksternal tersedia.
func LibreOfficeAvailable() bool {
	_, ok := DetectSoffice()
	return ok
}

// importFilter memilih filter impor; tanpa itu PDF dibuka LibreOffice di Draw.
func importFilter(from, to Format) string {
	if from != FormatPDF {
		return ""
	}
	if FamilyOf(to) == FamilyImpress {
		return "impress_pdf_import"
	}
	return "writer_pdf_import"
}

// ConvertWithLibreOffice menjalankan soffice headless dengan kontrak
// berkas-masuk → engine → berkas-keluar.
func ConvertWithLibreOffice(from, to Format, data []byte) ([]byte, error) {
	binary, ok := DetectSoffice()
	if !ok {
		return nil, errors.New("LibreOffice tidak ditemukan")
	}

	dir, err := os.MkdirTemp("", "mdconv-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)

	input := filepath.Join(dir, "input."+string(from))
	if err := os.WriteFile(input, data, 0o644); err != nil {
		return nil, err
	}

	profile := "file:///" + filepath.ToSlash(filepath.Join(dir, "profile"))
	args := []string{"--headless", "--norestore", "-env:UserInstallation=" + profile}
	if filter := importFilter(from, to); filter != "" {
		args = append(args, "--infilter="+filter)
	}
	args = append(args, "--convert-to", string(to), "--outdir", dir, input)

	cmd := exec.Command(binary, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return nil, err
	}

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	select {
	case err := <-done:
		if err != nil {
			return nil, fmt.Errorf("LibreOffice gagal: %v: %s", err, strings.TrimSpace(stderr.String()))
		}
	case <-time.After(convertTimeout):
		_ = cmd.Process.Kill()
		return nil, errors.New("konversi LibreOffice melebihi batas waktu")
	}

	output := filepath.Join(dir, "input."+string(to))
	content, err := os.ReadFile(output)
	if err != nil {
		return nil, fmt.Errorf("LibreOffice tidak menghasilkan berkas %s", to)
	}
	return content, nil
}
