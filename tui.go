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

type focus int

const (
	focusTree focus = iota
	focusFiles
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
	frameStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("238"))
	activePane  = lipgloss.NewStyle().Foreground(lipgloss.Color("39")).Bold(true)
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

// treeNode adalah satu baris pada pohon folder (sudah diratakan ke daftar).
type treeNode struct {
	name  string
	path  string
	depth int
}

type model struct {
	stage stage

	root     string
	tree     []treeNode
	treeCur  int
	expanded map[string]bool

	dir     string
	entries []fsEntry
	fileCur int

	focus  focus
	status string
	width  int
	height int

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

// readEntries membaca satu folder: folder lebih dulu, lalu berkas, keduanya terurut.
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

// subDirectories mengembalikan hanya subfolder (untuk pohon di panel kiri).
func subDirectories(dir string) []fsEntry {
	items, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var dirs []fsEntry
	for _, item := range items {
		if !item.IsDir() || strings.HasPrefix(item.Name(), ".") {
			continue
		}
		dirs = append(dirs, fsEntry{
			name:  item.Name(),
			path:  filepath.Join(dir, item.Name()),
			isDir: true,
		})
	}
	sort.Slice(dirs, func(i, j int) bool {
		return strings.ToLower(dirs[i].name) < strings.ToLower(dirs[j].name)
	})
	return dirs
}

// breadcrumb menampilkan path sebagai jejak, mis. "D:\ > Dokumen > arsip".
func breadcrumb(dir string) string {
	volume := filepath.VolumeName(dir)
	rest := strings.TrimPrefix(dir, volume)
	parts := strings.FieldsFunc(rest, func(r rune) bool { return r == '\\' || r == '/' })

	out := volume + string(filepath.Separator)
	for _, part := range parts {
		out += " › " + part
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
	m := model{
		stage:    stageFile,
		dir:      start,
		width:    100,
		height:   28,
		expanded: map[string]bool{},
	}
	m.spin = spinner.New()
	m.spin.Spinner = spinner.Dot

	m.initTree(start)
	m.setDir(start)
	return m
}

func (m model) Init() tea.Cmd { return nil }

// initTree menyiapkan pohon dari akar drive, dengan semua leluhur folder awal terbuka.
func (m *model) initTree(start string) {
	m.root = filepath.VolumeName(start) + string(filepath.Separator)

	for cur := start; ; {
		m.expanded[cur] = true
		parent := filepath.Dir(cur)
		if parent == cur || len(cur) <= len(m.root) {
			break
		}
		cur = parent
	}

	m.rebuildTree()
	m.treeCursorTo(start)
}

// rebuildTree meratakan pohon yang sedang terbuka menjadi daftar baris.
func (m *model) rebuildTree() {
	m.tree = m.tree[:0]
	var walk func(dir string, depth int)
	walk = func(dir string, depth int) {
		for _, sub := range subDirectories(dir) {
			m.tree = append(m.tree, treeNode{name: sub.name, path: sub.path, depth: depth})
			if m.expanded[sub.path] {
				walk(sub.path, depth+1)
			}
		}
	}
	walk(m.root, 0)
}

// treeCursorTo memindahkan kursor pohon ke path tertentu.
func (m *model) treeCursorTo(path string) {
	for i, node := range m.tree {
		if strings.EqualFold(node.path, path) {
			m.treeCur = i
			return
		}
	}
}

// setDir berpindah folder dan memuat isinya.
func (m *model) setDir(dir string) {
	entries, err := readEntries(dir)
	if err != nil {
		m.status = "tidak bisa membuka: " + dir
		return
	}
	m.dir = dir
	m.entries = entries
	m.fileCur = 0
	m.status = ""
}

// syncTree memastikan folder aktif terlihat di pohon (terbuka + kursor di sana).
func (m *model) syncTree(path string) {
	m.expanded[path] = true
	m.rebuildTree()
	m.treeCursorTo(path)
}

// openTree menangani Enter/panah kanan di panel pohon.
func (m *model) openTree() {
	if len(m.tree) == 0 {
		return
	}
	node := m.tree[m.treeCur]
	m.expanded[node.path] = true
	m.setDir(node.path)
	m.rebuildTree()
	m.treeCursorTo(node.path)
}

// collapseTree menangani panah kiri di panel pohon: tutup folder, atau naik ke induk.
func (m *model) collapseTree() {
	if len(m.tree) == 0 {
		return
	}
	node := m.tree[m.treeCur]
	if m.expanded[node.path] {
		delete(m.expanded, node.path)
		m.rebuildTree()
		m.treeCursorTo(node.path)
		return
	}
	parent := filepath.Dir(node.path)
	if parent != node.path {
		m.setDir(parent)
		m.rebuildTree()
		m.treeCursorTo(parent)
	}
}

// openFile menangani Enter di panel kanan: masuk folder, atau pilih berkas.
func (m *model) openFile() {
	if len(m.entries) == 0 {
		return
	}
	entry := m.entries[m.fileCur]
	if entry.isDir {
		m.setDir(entry.path)
		m.syncTree(entry.path)
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

// goParent naik satu folder.
func (m *model) goParent() {
	if parent := filepath.Dir(m.dir); parent != m.dir {
		m.setDir(parent)
		m.syncTree(parent)
	}
}

func (m *model) rows() int {
	r := m.height - 5
	if r < 5 {
		r = 5
	}
	return r
}

func (m model) leftWidth() int {
	w := m.width * 2 / 5
	if w < 18 {
		w = 18
	}
	if w > 46 {
		w = 46
	}
	return w
}

func (m *model) moveTree(delta int) {
	if len(m.tree) == 0 {
		return
	}
	m.treeCur += delta
	if m.treeCur < 0 {
		m.treeCur = 0
	}
	if m.treeCur > len(m.tree)-1 {
		m.treeCur = len(m.tree) - 1
	}
}

func (m *model) moveFiles(delta int) {
	if len(m.entries) == 0 {
		return
	}
	m.fileCur += delta
	if m.fileCur < 0 {
		m.fileCur = 0
	}
	if m.fileCur > len(m.entries)-1 {
		m.fileCur = len(m.entries) - 1
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
		page := m.rows()

		switch key.String() {
		case "q":
			return m, tea.Quit
		case "tab":
			if m.focus == focusTree {
				m.focus = focusFiles
			} else {
				m.focus = focusTree
			}
		case "ctrl+d":
			next := nextRoot(m.dir)
			m.initTree(next)
			m.setDir(next)
		case "~":
			home := startDirectory("")
			m.initTree(home)
			m.setDir(home)
		case "/":
			m.stage = stageSearch
			m.searchQuery = ""
			m.searchResults = nil
			m.searchCursor = 0
			m.searchGen++
			return m, m.searchCmd()
		default:
			if m.focus == focusTree {
				switch key.String() {
				case "up", "k":
					m.moveTree(-1)
				case "down", "j":
					m.moveTree(1)
				case "pgup":
					m.moveTree(-page)
				case "pgdown":
					m.moveTree(page)
				case "home":
					m.treeCur = 0
				case "end":
					if len(m.tree) > 0 {
						m.treeCur = len(m.tree) - 1
					}
				case "enter", "right", "l":
					m.openTree()
				case "left", "h", "backspace":
					m.collapseTree()
				}
			} else {
				switch key.String() {
				case "up", "k":
					m.moveFiles(-1)
				case "down", "j":
					m.moveFiles(1)
				case "pgup":
					m.moveFiles(-page)
				case "pgdown":
					m.moveFiles(page)
				case "home":
					m.fileCur = 0
				case "end":
					if len(m.entries) > 0 {
						m.fileCur = len(m.entries) - 1
					}
				case "enter", "right", "l":
					m.openFile()
				case "left", "h", "backspace":
					m.goParent()
				}
			}
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
	width := 0
	for _, r := range s {
		if (r >= 0x1F300 && r <= 0x1FAFF) || (r >= 0x2600 && r <= 0x27BF) {
			width += 2
		} else {
			width++
		}
	}
	return width
}

func padRight(s string, width int) string {
	if gap := width - displayWidth(s); gap > 0 {
		return s + strings.Repeat(" ", gap)
	}
	return s
}

func truncate(s string, width int) string {
	if displayWidth(s) <= width {
		return s
	}
	runes := []rune(s)
	if len(runes) > width-1 {
		runes = runes[:width-1]
	}
	return string(runes) + "…"
}

func (m model) treeLines(rows int) []string {
	visible := rows - 1
	if visible < 1 {
		visible = 1
	}

	title := crumbStyle.Render("Folder")
	if m.focus == focusTree {
		title = activePane.Render("▸ Folder")
	}
	lines := []string{title}

	if len(m.tree) == 0 {
		return append(lines, dimStyle.Render("(tidak ada subfolder)"))
	}

	start := windowStart(m.treeCur, len(m.tree), visible)
	for i := start; i < len(m.tree) && i < start+visible; i++ {
		node := m.tree[i]
		marker := "▸ "
		if m.expanded[node.path] {
			marker = "▾ "
		}
		line := strings.Repeat("  ", node.depth) + marker + truncate(node.name, 22)

		switch {
		case i == m.treeCur && m.focus == focusTree:
			lines = append(lines, cursorStyle.Render(line))
		case i == m.treeCur:
			lines = append(lines, dirStyle.Render(line))
		default:
			lines = append(lines, line)
		}
	}
	return lines
}

func (m model) fileLines(rows int) []string {
	visible := rows - 1
	if visible < 1 {
		visible = 1
	}

	title := breadcrumb(m.dir)
	if m.focus == focusFiles {
		title = activePane.Render(title)
	} else {
		title = crumbStyle.Render(title)
	}
	lines := []string{title}

	if len(m.entries) == 0 {
		return append(lines, dimStyle.Render("(folder kosong)"))
	}

	start := windowStart(m.fileCur, len(m.entries), visible)
	for i := start; i < len(m.entries) && i < start+visible; i++ {
		entry := m.entries[i]
		icon, name, size := "📄", entry.name, humanSize(entry.size)
		if entry.isDir {
			icon, name, size = "📁", entry.name+"/", "<DIR>"
		}

		selected := i == m.fileCur && m.focus == focusFiles
		marker := "  "
		if selected {
			marker = "> "
		}
		row := marker + icon + " " + padRight(truncate(name, 26), 26) +
			fmt.Sprintf("%9s  %s", size, entry.mod.Format("02/01 15:04"))

		switch {
		case selected:
			lines = append(lines, cursorStyle.Render(row))
		case entry.isDir:
			lines = append(lines, dirStyle.Render(row))
		default:
			lines = append(lines, row)
		}
	}
	return lines
}

func (m model) browserView() string {
	rows := m.rows()
	left := m.treeLines(rows)
	right := m.fileLines(rows)

	leftStyle := lipgloss.NewStyle().Width(m.leftWidth())

	var b strings.Builder
	b.WriteString(titleStyle.Render("vert — pilih berkas") + "\n\n")
	for i := 0; i < rows; i++ {
		var leftLine, rightLine string
		if i < len(left) {
			leftLine = left[i]
		}
		if i < len(right) {
			rightLine = right[i]
		}
		b.WriteString(leftStyle.Render(leftLine) + frameStyle.Render(" │ ") + rightLine + "\n")
	}

	if m.status != "" {
		b.WriteString("\n" + errStyle.Render(m.status))
	}
	b.WriteString("\n" + dimStyle.Render("Tab pindah panel · ↑/↓ pilih · Enter buka · / cari · ~ home · Ctrl+D drive · q keluar"))
	return b.String()
}

func (m model) searchView() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("Cari berkas") + "\n")
	b.WriteString(crumbStyle.Render(breadcrumb(m.dir)) + "\n\n")
	b.WriteString("Cari: " + m.searchQuery + "▌\n\n")

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
		return m.spin.View() + " Mengonversi…"
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
