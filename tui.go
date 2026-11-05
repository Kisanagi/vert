package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/filepicker"
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
	dimStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	okStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	errStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	cursorStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))
)

type convertDoneMsg struct {
	output string
	err    error
}

type searchDoneMsg struct {
	gen     int
	results []SearchResult
}

type model struct {
	stage   stage
	picker  filepicker.Model
	targets []Format
	cursor  int
	file    string
	from    Format
	spin    spinner.Model
	result  string
	failure error

	searchQuery   string
	searchResults []SearchResult
	searchCursor  int
	searchGen     int
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
	fp := filepicker.New()
	fp.AllowedTypes = []string{
		".pdf", ".docx", ".odt", ".rtf", ".txt", ".html", ".htm",
		".md", ".markdown", ".xlsx", ".ods", ".csv", ".pptx", ".odp",
	}
	fp.CurrentDirectory = startDirectory(dir)
	fp.ShowHidden = false

	sp := spinner.New()
	sp.Spinner = spinner.Dot

	return model{stage: stageFile, picker: fp, spin: sp}
}

func (m model) Init() tea.Cmd { return m.picker.Init() }

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

	switch m.stage {
	case stageFile:
		if key, ok := msg.(tea.KeyMsg); ok {
			switch key.String() {
			case "q":
				return m, tea.Quit
			case "/":
				m.stage = stageSearch
				m.searchQuery = ""
				m.searchResults = nil
				m.searchCursor = 0
				m.searchGen++
				return m, m.searchCmd()
			case "~":
				m.picker.CurrentDirectory = startDirectory("")
				return m, m.picker.Init()
			case "tab":
				m.picker.CurrentDirectory = nextRoot(m.picker.CurrentDirectory)
				return m, m.picker.Init()
			}
		}
		var cmd tea.Cmd
		m.picker, cmd = m.picker.Update(msg)
		if selected, path := m.picker.DidSelectFile(msg); selected {
			if from, ok := DetectFormat(path); ok {
				m.file, m.from = path, from
				m.targets = AvailableTargets(from)
				m.cursor = 0
				m.stage = stageFormat
			}
		}
		return m, cmd

	case stageSearch:
		key, ok := msg.(tea.KeyMsg)
		if !ok {
			return m, nil
		}
		switch key.String() {
		case "esc":
			m.stage = stageFile
			return m, m.picker.Init()
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
			if from, ok := DetectFormat(path); ok {
				m.file, m.from = path, from
				m.targets = AvailableTargets(from)
				m.cursor = 0
				m.stage = stageFormat
			}
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
		case "q", "esc":
			return m, tea.Quit
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(m.targets)-1 {
				m.cursor++
			}
		case "enter":
			if len(m.targets) == 0 {
				return m, nil
			}
			m.stage = stageBusy
			return m, tea.Batch(m.spin.Tick, m.convertCmd(m.targets[m.cursor]))
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
	root, query, gen := m.picker.CurrentDirectory, m.searchQuery, m.searchGen
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

func (m model) View() string {
	switch m.stage {
	case stageFile:
		return titleStyle.Render("MD Converter — pilih berkas") + "\n" +
			dimStyle.Render("untuk: "+m.picker.CurrentDirectory) + "\n\n" +
			m.picker.View() + "\n" +
			dimStyle.Render("↑/↓ pilih · Enter buka/pilih · / cari · ~ home · Tab ganti drive · q keluar")

	case stageSearch:
		var b strings.Builder
		b.WriteString(titleStyle.Render("Cari berkas") + "\n")
		b.WriteString(dimStyle.Render("di: "+m.picker.CurrentDirectory) + "\n\n")
		b.WriteString("Cari: " + m.searchQuery + "▌\n\n")

		if len(m.searchResults) == 0 {
			b.WriteString(dimStyle.Render("tidak ada berkas yang cocok") + "\n")
		} else {
			from := 0
			if m.searchCursor >= 10 {
				from = m.searchCursor - 9
			}
			for i := from; i < len(m.searchResults) && i < from+10; i++ {
				item := m.searchResults[i]
				line := filepath.Base(item.Path) + "  " + humanSize(item.Size)
				detail := item.Path
				if i == m.searchCursor {
					b.WriteString(cursorStyle.Render("> "+line) + "\n")
					b.WriteString(dimStyle.Render("  "+detail) + "\n")
				} else {
					b.WriteString("  " + line + "\n")
				}
			}
		}

		b.WriteString("\n" + dimStyle.Render(fmt.Sprintf("%d hasil", len(m.searchResults))) +
			dimStyle.Render(" · ↑/↓ pilih · Enter pakai · Esc kembali · ketik untuk cari"))
		return b.String()

	case stageFormat:
		var b strings.Builder
		b.WriteString(titleStyle.Render("Konversi "+filepath.Base(m.file)) + "\n\n")
		if len(m.targets) == 0 {
			b.WriteString(errStyle.Render("Tidak ada format tujuan yang didukung untuk berkas ini.") + "\n")
		} else {
			for i, target := range m.targets {
				if i == m.cursor {
					b.WriteString(cursorStyle.Render("> "+label(target)) + "\n")
				} else {
					b.WriteString("  " + label(target) + "\n")
				}
			}
		}
		b.WriteString("\n" + dimStyle.Render("↑/↓ pilih · Enter konversi · q keluar"))
		return b.String()

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
