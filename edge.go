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

// convertTimeout membatasi satu langkah konversi lewat mesin eksternal.
const convertTimeout = 120 * time.Second

var cachedEdge *string

// joinIfSet menggabungkan path hanya bila basisnya ada, supaya variabel lingkungan
// yang kosong tidak menghasilkan path relatif.
func joinIfSet(base string, parts ...string) string {
	if base == "" {
		return ""
	}
	return filepath.Join(append([]string{base}, parts...)...)
}

// edgeCandidates mendata lokasi biner Edge/Chromium yang mungkin ada. Edge sudah
// terpasang di setiap Windows 10/11, jadi mesin cetak ini tidak butuh instalasi.
func edgeCandidates() []string {
	var out []string
	if v := os.Getenv("MD_CONVERTER_EDGE"); v != "" {
		out = append(out, v)
	}

	dirs := []string{
		joinIfSet(os.Getenv("LOCALAPPDATA"), "Microsoft", "Edge", "Application"),
		joinIfSet(os.Getenv("ProgramFiles(x86)"), "Microsoft", "Edge", "Application"),
		joinIfSet(os.Getenv("ProgramFiles"), "Microsoft", "Edge", "Application"),
		joinIfSet(os.Getenv("LOCALAPPDATA"), "Google", "Chrome", "Application"),
		joinIfSet(os.Getenv("ProgramFiles"), "Google", "Chrome", "Application"),
		joinIfSet(os.Getenv("ProgramFiles(x86)"), "Google", "Chrome", "Application"),
	}
	for _, dir := range dirs {
		if dir == "" {
			continue
		}
		out = append(out, filepath.Join(dir, "msedge.exe"), filepath.Join(dir, "chrome.exe"))
	}

	if runtime.GOOS != "windows" {
		out = append(out, "microsoft-edge", "google-chrome", "chromium", "chromium-browser")
	}
	return out
}

// DetectEdge mengembalikan path biner Edge/Chrome bila ada. Hasil di-cache.
func DetectEdge() (string, bool) {
	if cachedEdge != nil {
		return *cachedEdge, *cachedEdge != ""
	}

	found := ""
	for _, candidate := range edgeCandidates() {
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			found = candidate
			break
		}
		if path, err := exec.LookPath(candidate); err == nil {
			found = path
			break
		}
	}

	cachedEdge = &found
	return found, found != ""
}

// EdgeAvailable melaporkan apakah mesin cetak HTML -> PDF tersedia.
func EdgeAvailable() bool {
	_, ok := DetectEdge()
	return ok
}

// ConvertHTMLToPDF mencetak HTML menjadi PDF memakai mesin cetak Edge/Chromium.
// Dukungan CSS-nya (ukuran halaman, margin, latar tabel) lebih setia daripada
// impor HTML mesin Office.
func ConvertHTMLToPDF(html []byte) ([]byte, error) {
	binary, ok := DetectEdge()
	if !ok {
		return nil, errors.New("Edge/Chrome tidak ditemukan")
	}

	dir, err := os.MkdirTemp("", "vert-edge-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)

	input := filepath.Join(dir, "input.html")
	if err := os.WriteFile(input, html, 0o644); err != nil {
		return nil, err
	}
	output := filepath.Join(dir, "output.pdf")

	// Mode headless baru dipakai lebih dulu; sebagian versi lama hanya mengenal
	// --headless biasa.
	var lastErr error
	for _, mode := range []string{"--headless=new", "--headless"} {
		content, err := runEdgePrint(binary, mode, dir, input, output)
		if err == nil {
			return content, nil
		}
		lastErr = err
	}
	return nil, lastErr
}

func runEdgePrint(binary, mode, dir, input, output string) ([]byte, error) {
	_ = os.Remove(output)

	url := "file:///" + filepath.ToSlash(input)
	args := []string{
		mode,
		"--disable-gpu",
		"--no-first-run",
		"--no-default-browser-check",
		"--user-data-dir=" + filepath.Join(dir, "profile"),
		"--print-to-pdf-no-header",
		"--no-pdf-header-footer",
		"--print-to-pdf=" + output,
		url,
	}

	cmd := exec.Command(binary, args...)
	hideWindow(cmd)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return nil, err
	}

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	select {
	case <-done:
		// Chromium kadang keluar dengan kode bukan nol walau PDF sudah ditulis,
		// jadi keberhasilan dinilai dari berkasnya, bukan dari kode keluar.
	case <-time.After(convertTimeout):
		_ = cmd.Process.Kill()
		return nil, errors.New("pencetakan PDF melebihi batas waktu")
	}

	content, err := os.ReadFile(output)
	if err != nil || len(content) == 0 {
		message := strings.TrimSpace(stderr.String())
		if message == "" {
			message = "tidak menghasilkan berkas PDF"
		}
		return nil, fmt.Errorf("Edge gagal: %s", message)
	}
	if !bytes.HasPrefix(content, []byte("%PDF")) {
		return nil, errors.New("Edge menghasilkan berkas yang bukan PDF")
	}
	return content, nil
}
