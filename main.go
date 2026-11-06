package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// valueFlags adalah flag yang nilainya ditulis terpisah (mis. `--to pdf`).
var valueFlags = map[string]bool{"to": true, "out": true}

// normalizeArgs memindahkan flag ke depan agar `vert file.docx --to pdf` tetap terbaca.
func normalizeArgs(args []string) []string {
	var flags, positional []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if !strings.HasPrefix(arg, "-") || arg == "-" {
			positional = append(positional, arg)
			continue
		}
		flags = append(flags, arg)
		name := strings.TrimLeft(arg, "-")
		if !strings.Contains(name, "=") && valueFlags[name] && i+1 < len(args) {
			i++
			flags = append(flags, args[i])
		}
	}
	return append(flags, positional...)
}

// isTerminal melaporkan apakah berkas terhubung ke terminal (bukan pipa/berkas).
func isTerminal(f *os.File) bool {
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}

// isDir melaporkan apakah path menunjuk ke sebuah direktori.
func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func main() {
	initConsole()

	args := os.Args[1:]
	if len(args) == 0 || (len(args) == 1 && isDir(args[0])) {
		if !isTerminal(os.Stdin) || !isTerminal(os.Stdout) {
			fmt.Fprintln(os.Stderr, "TUI memerlukan terminal interaktif. Mode satu-perintah:")
			fmt.Fprintln(os.Stderr, "  vert <berkas> --to <format>")
			os.Exit(2)
		}
		dir := ""
		if len(args) == 1 {
			dir = args[0]
		}
		if err := runTUI(dir); err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(1)
		}
		return
	}

	fs := flag.NewFlagSet("vert", flag.ContinueOnError)
	to := fs.String("to", "", "format tujuan, mis. pdf")
	out := fs.String("out", "", "path hasil (opsional; default di sebelah berkas sumber)")
	listTargets := fs.Bool("formats", false, "tampilkan format tujuan yang didukung")
	find := fs.String("find", "", "cari berkas (nama mengandung teks) di dalam direktori, lalu cetak hasilnya")
	preview := fs.Bool("preview", false, "cetak satu tampilan TUI lalu keluar (untuk memeriksa tata letak)")
	repair := fs.Bool("repair", false, "tulis ulang markdown rusak (tabel runtuh) ke berkas baru")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "Pemakaian: vert <berkas...> --to <format> [--out <path>]")
		fmt.Fprintln(os.Stderr, "           vert                      # buka TUI")
		fmt.Fprintln(os.Stderr, "           vert <folder>             # buka TUI di folder itu")
		fmt.Fprintln(os.Stderr, "           vert <berkas> -formats    # lihat target yang didukung")
		fmt.Fprintln(os.Stderr, "           vert -find <teks> [dir]   # cari berkas yang bisa dikonversi")
		fmt.Fprintln(os.Stderr, "           vert -repair <berkas.md>  # perbaiki markdown rusak ke <nama>-fixed.md")
	}

	if err := fs.Parse(normalizeArgs(args)); err != nil {
		os.Exit(2)
	}

	inputs := fs.Args()

	if *repair {
		if len(inputs) == 0 {
			fs.Usage()
			os.Exit(2)
		}
		input := inputs[0]
		data, err := os.ReadFile(input)
		if err != nil {
			fmt.Fprintf(os.Stderr, "GAGAL %s: %v\n", filepath.Base(input), err)
			os.Exit(1)
		}
		target := *out
		if target == "" {
			ext := filepath.Ext(input)
			target = strings.TrimSuffix(input, ext) + "-fixed" + ext
		}
		if err := os.WriteFile(target, []byte(prepareMarkdown(data)), 0o644); err != nil {
			fmt.Fprintf(os.Stderr, "GAGAL menulis %s: %v\n", target, err)
			os.Exit(1)
		}
		fmt.Printf("OK   %s -> %s\n", filepath.Base(input), target)
		return
	}

	if *preview {
		path := ""
		if len(inputs) > 0 {
			path = inputs[0]
		}
		dir := path
		if path != "" && !isDir(path) {
			dir = filepath.Dir(path)
		}

		view := newModel(dir)
		view.width, view.height = 100, 24

		// Bila diberi berkas, tampilkan tahap pemilihan format untuk berkas itu.
		if path != "" && !isDir(path) {
			if from, ok := DetectFormat(path); ok {
				view.file, view.from = path, from
				view.targets = AvailableTargets(from)
				view.choice = 0
				view.canRepair = from == FormatMD
				view.stage = stageFormat
			}
		}

		fmt.Println(view.View())
		return
	}

	if *find != "" {
		root := "."
		if len(inputs) > 0 {
			root = inputs[0]
		}
		if !isDir(root) {
			fmt.Fprintf(os.Stderr, "bukan direktori: %s\n", root)
			os.Exit(1)
		}
		results := FindFiles(root, *find, maxSearchResults)
		if len(results) == 0 {
			fmt.Fprintln(os.Stderr, "tidak ada berkas yang cocok")
			os.Exit(1)
		}
		for _, result := range results {
			fmt.Printf("%10s  %s\n", humanSize(result.Size), result.Path)
		}
		return
	}

	if len(inputs) == 0 {
		fs.Usage()
		os.Exit(2)
	}

	if *listTargets {
		from, ok := DetectFormat(inputs[0])
		if !ok {
			fmt.Fprintf(os.Stderr, "format sumber tidak dikenal: %s\n", inputs[0])
			os.Exit(1)
		}
		fmt.Println(strings.Join(formatList(AvailableTargets(from)), " "))
		return
	}

	if *to == "" {
		fs.Usage()
		os.Exit(2)
	}

	target := Format(*to)
	for _, input := range inputs {
		if err := convertFile(input, target, *out); err != nil {
			fmt.Fprintf(os.Stderr, "GAGAL %s: %v\n", filepath.Base(input), err)
			os.Exit(1)
		}
	}
}

func formatList(formats []Format) []string {
	out := make([]string, len(formats))
	for i, f := range formats {
		out[i] = string(f)
	}
	return out
}

func convertFile(input string, to Format, out string) error {
	from, ok := DetectFormat(input)
	if !ok {
		return fmt.Errorf("format sumber tidak dikenal")
	}

	data, err := os.ReadFile(input)
	if err != nil {
		return err
	}

	result, err := RunConversion(from, to, data)
	if err != nil {
		return err
	}

	target := out
	if target == "" {
		target = strings.TrimSuffix(input, filepath.Ext(input)) + "." + string(to)
	} else if info, err := os.Stat(target); err == nil && info.IsDir() {
		base := strings.TrimSuffix(filepath.Base(input), filepath.Ext(input))
		target = filepath.Join(target, base+"."+string(to))
	}

	if err := os.WriteFile(target, result, 0o644); err != nil {
		return err
	}
	fmt.Printf("OK   %s -> %s\n", filepath.Base(input), target)
	return nil
}
