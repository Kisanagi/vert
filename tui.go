package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type stage int

const (
	stageFile stage = iota
	stageSearch
	stageFormat
	stageBusy
	stageDone
)

var formatLabels = map[Format]string{
	FormatPDF:  "PDF",
	FormatDOCX: "Word (DOCX)",
	FormatODT:  "OpenDocument Text (ODT)",
	FormatRTF:  "Rich Text (RTF)",
	FormatTXT:  "Teks (TXT)",
	FormatHTML: "HTML",
	FormatMD:   "Markdown (MD)",
	FormatXLSX: "Excel (XLSX)",
	FormatODS:  "OpenDocument Sheet (ODS)",
	FormatCSV:  "CSV",
	FormatPPTX: "PowerPoint (PPTX)",
	FormatODP:  "OpenDocument Presentation (ODP)",
}

func label(f Format) string {
	if l, ok := formatLabels[f]; ok {
		return l
	}
	return string(f)
}

var (
	titleStyle  = lipgloss.NewStyle().Bold(true)
	crumbStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("39"))
	dimStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	okStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	errStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	cursorStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))
	dirStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("33"))
)

type convertDoneMsg struct {
	output string
	err    error
}

type searchDoneMsg struct {
	gen     int
	results []SearchResult
}

// fsEntry adalah satu baris pada daftar berkas.
type fsEntry struct {
	name  string
	path  string
	isDir bool
	size  int64
	mod   time.Time
}

type model struct {
	stage stage

	dir     string
	entries []fsEntry
	cursor  int
	status  string
	width   int
	height  int

	file    string
	from    Format
	targets []Format
	choice  int

	spin    spinner.Model
	result  string
	failure error

	searchQuery   string
	searchResults []SearchResult
	searchCursor  int
	searchGen     int
}

// ---------- berkas ----------

// readEntries membaca satu folder: folder tampil lebih dulu, lalu berkas, keduanya terurut.
func readEntries(dir string) ([]fsEntry, error) {
	items, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	entries := make([]fsEntry, 0, len(items))
	for _, item := range items {
		name := item.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		info, err := item.Info()
		if err != nil {
			continue
		}
		entries = append(entries, fsEntry{
			name:  name,
			path:  filepath.Join(dir, name),
			isDir: item.IsDir(),
			size:  info.Size(),
			mod:   info.ModTime(),
		})
	}

	sort.Slice(entries, func(i, j int) bool {
		if entries[i].isDir != entries[j].isDir {
			return entries[i].isDir
		}
		return strings.ToLower(entries[i].name) < strings.ToLower(entries[j].name)
	})
	return entries, nil
}

// breadcrumb menampilkan path sebagai jejak, mis. "D:\ > Dokumen > arsip".
func breadcrumb(dir string) string {
	volume := filepath.VolumeName(dir)
	rest := strings.TrimPrefix(dir, volume)
	parts := strings.FieldsFunc(rest, func(r rune) bool { return r == '\\' || r == '/' })

	out := volume + string(filepath.Separator)
	for _, part := range parts {
		out += " > " + part
	}
	return out
}

// startDirectory menentukan folder awal TUI: argumen eksplisit, atau folder home user.
func startDirectory(dir string) string {
	if dir != "" {
		return dir
	}
	if home, err := os.UserHomeDir(); err == nil {
		return home
	}
	wd, _ := os.Getwd()
	return wd
}

// availableRoots mendata akar drive yang ada (Windows); kosong di sistem lain.
func availableRoots() []string {
	var roots []string
	for c := 'A'; c <= 'Z'; c++ {
		root := string(c) + ":\\"
		if info, err := os.Stat(root); err == nil && info.IsDir() {
			roots = append(roots, root)
		}
	}
	return roots
}

// nextRoot mengembalikan akar drive berikutnya, untuk berpindah drive dengan cepat.
func nextRoot(current string) string {
	roots := availableRoots()
	if len(roots) == 0 {
		return current
	}
	volume := strings.TrimRight(filepath.VolumeName(current), "\\")
	for i, root := range roots {
		if strings.EqualFold(strings.TrimRight(root, "\\"), volume) {
			return roots[(i+1)%len(roots)]
		}
	}
	return roots[0]
}

func newModel(dir string) model {
	start := startDirectory(dir)
	m := model{stage: stageFile, dir: start, width: 80, height: 24}
	m.spin = spinner.New()
	m.spin.Spinner = spinner.Dot

	entries, err := readEntries(start)
	if err != nil {
		m.status = "tidak bisa membuka folder"
		return m
	}
	m.entries = entries
	return m
}

func (m model) Init() tea.Cmd { return nil }

// setDir berpindah folder dan memuat isinya.
func (m *model) setDir(dir string) {
	entries, err := readEntries(dir)
	if err != nil {
		m.status = "tidak bisa membuka: " + dir
		return
	}
	m.dir = dir
	m.entries = entries
	m.cursor = 0
	m.status = ""
}

// open menjalankan aksi Enter: masuk folder, atau pilih berkas untuk dikonversi.
func (m *model) open() {
	if len(m.entries) == 0 {
		return
	}
	entry := m.entries[m.cursor]
	if entry.isDir {
		m.setDir(entry.path)
		return
	}
	from, ok := DetectFormat(entry.name)
	if !ok {
		m.status = entry.name + " tidak bisa dikonversi"
		return
	}
	m.file, m.from = entry.path, from
	m.targets = AvailableTargets(from)
	m.choice = 0
	m.stage = stageFormat
}

func (m *model) goParent() {
	if parent := filepath.Dir(m.dir); parent != m.dir {
		m.setDir(parent)
	}
}

// pageSize menentukan berapa baris yang muat di layar.
func (m *model) pageSize() int {
	visible := m.height - 8
	if visible < 5 {
		visible = 5
	}
	return visible
}

func (m *model) move(delta int) {
	if len(m.entries) == 0 {
		return
	}
	m.cursor += delta
	if m.cursor < 0 {
		m.cursor = 0
	}
	if m.cursor > len(m.entries)-1 {
		m.cursor = len(m.entries) - 1
	}
}

// windowStart menghitung baris pertama yang ditampilkan agar kursor tetap terlihat.
func windowStart(cursor, total, visible int) int {
	if total <= visible {
		return 0
	}
	start := cursor - visible/2
	if start < 0 {
		start = 0
	}
	if start > total-visible {
		start = total - visible
	}
	return start
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok && key.String() == "ctrl+c" {
		return m, tea.Quit
	}
	if done, ok := msg.(convertDoneMsg); ok {
		m.stage = stageDone
		m.result, m.failure = done.output, done.err
		return m, nil
	}
	if done, ok := msg.(searchDoneMsg); ok {
		if done.gen == m.searchGen {
			m.searchResults = done.results
			if m.searchCursor >= len(m.searchResults) {
				m.searchCursor = 0
			}
		}
		return m, nil
	}
	if size, ok := msg.(tea.WindowSizeMsg); ok {
		m.width, m.height = size.Width, size.Height
		return m, nil
	}

	switch m.stage {
	case stageFile:
		key, ok := msg.(tea.KeyMsg)
		if !ok {
			return m, nil
		}
		switch key.String() {
		case "q":
			return m, tea.Quit
		case "up", "k":
			m.move(-1)
		case "down", "j":
			m.move(1)
		case "pgup":
			m.move(-m.pageSize())
		case "pgdown":
			m.move(m.pageSize())
		case "home":
			m.cursor = 0
		case "end":
			if len(m.entries) > 0 {
				m.cursor = len(m.entries) - 1
			}
		case "enter", "right", "l":
			m.open()
		case "backspace", "left", "h":
			m.goParent()
		case "/":
			m.stage = stageSearch
			m.searchQuery = ""
			m.searchResults = nil
			m.searchCursor = 0
			m.searchGen++
			return m, m.searchCmd()
		case "~":
			m.setDir(startDirectory(""))
		case "tab":
			m.setDir(nextRoot(m.dir))
		}
		return m, nil

	case stageSearch:
		key, ok := msg.(tea.KeyMsg)
		if !ok {
			return m, nil
		}
		switch key.String() {
		case "esc":
			m.stage = stageFile
		case "q":
			return m, tea.Quit
		case "up", "ctrl+p":
			if m.searchCursor > 0 {
				m.searchCursor--
			}
		case "down", "ctrl+n":
			if m.searchCursor < len(m.searchResults)-1 {
				m.searchCursor++
			}
		case "enter":
			if len(m.searchResults) == 0 {
				return m, nil
			}
			path := m.searchResults[m.searchCursor].Path
			from, ok := DetectFormat(path)
			if !ok {
				return m, nil
			}
			m.file, m.from = path, from
			m.targets = AvailableTargets(from)
			m.choice = 0
			m.stage = stageFormat
		case "backspace":
			if len(m.searchQuery) > 0 {
				m.searchQuery = m.searchQuery[:len(m.searchQuery)-1]
				m.searchGen++
				return m, m.searchCmd()
			}
		default:
			if len(key.Runes) > 0 {
				m.searchQuery += string(key.Runes)
				m.searchGen++
				return m, m.searchCmd()
			}
		}
		return m, nil

	case stageFormat:
		key, ok := msg.(tea.KeyMsg)
		if !ok {
			return m, nil
		}
		switch key.String() {
		case "q":
			return m, tea.Quit
		case "esc":
			m.stage = stageFile
		case "up", "k":
			if m.choice > 0 {
				m.choice--
			}
		case "down", "j":
			if m.choice < len(m.targets)-1 {
				m.choice++
			}
		case "enter":
			if len(m.targets) == 0 {
				return m, nil
			}
			m.stage = stageBusy
			return m, tea.Batch(m.spin.Tick, m.convertCmd(m.targets[m.choice]))
		}
		return m, nil

	case stageBusy:
		var cmd tea.Cmd
		m.spin, cmd = m.spin.Update(msg)
		return m, cmd

	case stageDone:
		if key, ok := msg.(tea.KeyMsg); ok {
			switch key.String() {
			case "q", "esc", "enter":
				return m, tea.Quit
			}
		}
		return m, nil
	}
	return m, nil
}

// searchCmd menjalankan penelusuran di folder aktif; hasil dikirim sebagai pesan.
func (m model) searchCmd() tea.Cmd {
	root, query, gen := m.dir, m.searchQuery, m.searchGen
	return func() tea.Msg {
		return searchDoneMsg{gen: gen, results: FindFiles(root, query, maxSearchResults)}
	}
}

func (m model) convertCmd(to Format) tea.Cmd {
	from, path := m.from, m.file
	return func() tea.Msg {
		data, err := os.ReadFile(path)
		if err != nil {
			return convertDoneMsg{err: err}
		}
		result, err := RunConversion(from, to, data)
		if err != nil {
			return convertDoneMsg{err: err}
		}
		target := strings.TrimSuffix(path, filepath.Ext(path)) + "." + string(to)
		if err := os.WriteFile(target, result, 0o644); err != nil {
			return convertDoneMsg{err: err}
		}
		return convertDoneMsg{output: target}
	}
}

// ---------- tampilan ----------

// displayWidth menghitung lebar kolom; emoji dihitung dua sel agar kolom sejajar.
func displayWidth(s string) int {
	return len([]rune(s))
}

func padRight(s string, width int) string {
	if gap := width - displayWidth(s); gap > 0 {
		return s + strings.Repeat(" ", gap)
	}
	return s
}

func truncate(s string, width int) string {
	runes := []rune(s)
	if len(runes) <= width {
		return s
	}
	if width <= 3 {
		return string(runes[:width])
	}
	return string(runes[:width-3]) + "..."
}

func entryRow(entry fsEntry, selected bool) string {
	marker := "  "
	if selected {
		marker = "> "
	}

	kind, name, size := "   ", entry.name, humanSize(entry.size)
	if entry.isDir {
		kind, name, size = "[D]", entry.name+"/", "<DIR>"
	}

	row := marker + kind + " " + padRight(truncate(name, 34), 34) +
		fmt.Sprintf("%10s  %s", size, entry.mod.Format("02/01 15:04"))

	if selected {
		return cursorStyle.Render(row)
	}
	if entry.isDir {
		return dirStyle.Render(row)
	}
	return row
}

func (m model) browserView() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("vert - pilih berkas") + "\n")
	b.WriteString(crumbStyle.Render(breadcrumb(m.dir)) + "\n\n")

	if len(m.entries) == 0 {
		b.WriteString(dimStyle.Render("(folder kosong)") + "\n")
	} else {
		visible := m.pageSize()
		start := windowStart(m.cursor, len(m.entries), visible)
		for i := start; i < len(m.entries) && i < start+visible; i++ {
			b.WriteString(entryRow(m.entries[i], i == m.cursor) + "\n")
		}
		if len(m.entries) > visible {
			b.WriteString(fmt.Sprintf("  ... %d dari %d", m.cursor+1, len(m.entries)) + "\n")
		}
	}

	if m.status != "" {
		b.WriteString("\n" + errStyle.Render(m.status))
	}
	b.WriteString("\n" + dimStyle.Render("↑/↓ pilih · Enter buka · / cari · ~ home · Tab drive · Backspace naik · q keluar"))
	return b.String()
}

func (m model) searchView() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("Cari berkas") + "\n")
	b.WriteString(crumbStyle.Render(breadcrumb(m.dir)) + "\n\n")
	b.WriteString("Cari: " + m.searchQuery + "_\n\n")

	if len(m.searchResults) == 0 {
		b.WriteString(dimStyle.Render("tidak ada berkas yang cocok") + "\n")
	} else {
		start := windowStart(m.searchCursor, len(m.searchResults), 10)
		for i := start; i < len(m.searchResults) && i < start+10; i++ {
			item := m.searchResults[i]
			line := filepath.Base(item.Path) + "  " + humanSize(item.Size)
			if i == m.searchCursor {
				b.WriteString(cursorStyle.Render("> "+line) + "\n")
				b.WriteString(dimStyle.Render("  "+item.Path) + "\n")
			} else {
				b.WriteString("  " + line + "\n")
			}
		}
	}

	b.WriteString("\n" + dimStyle.Render(fmt.Sprintf("%d hasil · ↑/↓ pilih · Enter pakai · Esc kembali", len(m.searchResults))))
	return b.String()
}

func (m model) formatView() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("Konversi "+filepath.Base(m.file)) + "\n\n")
	if len(m.targets) == 0 {
		b.WriteString(errStyle.Render("Tidak ada format tujuan yang didukung untuk berkas ini.") + "\n")
	} else {
		for i, target := range m.targets {
			if i == m.choice {
				b.WriteString(cursorStyle.Render("> "+label(target)) + "\n")
			} else {
				b.WriteString("  " + label(target) + "\n")
			}
		}
	}

	if len(m.targets) > 0 && m.choice < len(m.targets) {
		if pipeline, ok := PipelineFor(m.from, m.targets[m.choice]); ok && pipeline.Chained() {
			b.WriteString("\n" + dimStyle.Render("catatan: jalur bertingkat - hasil bisa kurang presisi"))
		}
	}

	b.WriteString("\n" + dimStyle.Render("↑/↓ pilih · Enter konversi · Esc kembali · q keluar"))
	return b.String()
}

func (m model) View() string {
	switch m.stage {
	case stageFile:
		return m.browserView()
	case stageSearch:
		return m.searchView()
	case stageFormat:
		return m.formatView()
	case stageBusy:
		return m.spin.View() + " Mengonversi..."
	case stageDone:
		if m.failure != nil {
			return errStyle.Render("Gagal: "+m.failure.Error()) + "\n\n" + dimStyle.Render("Enter/q keluar")
		}
		return okStyle.Render("Berhasil: "+m.result) + "\n\n" + dimStyle.Render("Enter/q keluar")
	}
	return ""
}

func runTUI(dir string) error {
	_, err := tea.NewProgram(newModel(dir), tea.WithAltScreen()).Run()
	return err
}
