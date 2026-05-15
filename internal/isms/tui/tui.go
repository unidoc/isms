// Package tui provides an interactive terminal UI for the ISMS, focused on
// reading documents. List view on the left, rendered document on the right.
// Press enter to open a document in full-screen reader mode; q/esc to go back.
package tui

import (
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
	"isms.sh/internal/isms/client"
)

// mode controls the top-level layout.
type mode int

const (
	modeList   mode = iota // two-pane: list on left, preview on right
	modeReader              // full-screen rendered document
)

// item is a row in the document list — either a folder header or a document.
type item struct {
	docID    string // empty for folder rows
	title    string
	path     string // folder path for folder rows
	status   string
	isFolder bool
}

// Model is the bubbletea model for the document TUI.
type Model struct {
	client   *client.Client
	width    int
	height   int
	ready    bool
	mode     mode
	items    []item
	cursor   int
	filter   string
	filterOn bool
	viewport viewport.Model
	docBody  string // raw markdown of the currently-open document
	docTitle string
	rendered string
	renderer *glamour.TermRenderer
	loadErr  string
}

const (
	listPaneWidth = 38
	helpHeight    = 1
)

// New creates a new document-focused TUI model.
func New(c *client.Client) Model {
	r, _ := glamour.NewTermRenderer(
		glamour.WithAutoStyle(),
		glamour.WithWordWrap(80),
	)
	m := Model{
		client:   c,
		renderer: r,
	}
	m.loadDocuments()
	return m
}

// Init implements tea.Model.
func (m Model) Init() tea.Cmd { return nil }

// Update implements tea.Model.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.rebuildViewport()
		m.ready = true
		if m.docBody != "" {
			m.rerender()
		}
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)
	}

	if m.mode == modeReader {
		var cmd tea.Cmd
		m.viewport, cmd = m.viewport.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m *Model) rebuildViewport() {
	w := m.width - 4
	if m.mode == modeList {
		w = m.width - listPaneWidth - 4
	}
	if w < 20 {
		w = 20
	}
	h := m.height - helpHeight - 2
	if h < 5 {
		h = 5
	}
	m.viewport = viewport.New(w, h)
	m.viewport.Style = lipgloss.NewStyle().PaddingLeft(1).PaddingRight(1)
	m.renderer, _ = glamour.NewTermRenderer(
		glamour.WithAutoStyle(),
		glamour.WithWordWrap(w-2),
	)
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	// Filter input mode — keys flow into the filter string.
	if m.filterOn && m.mode == modeList {
		switch key {
		case "esc":
			m.filterOn = false
			m.filter = ""
			m.cursor = 0
			return m, nil
		case "enter":
			m.filterOn = false
			return m, nil
		case "backspace":
			if len(m.filter) > 0 {
				m.filter = m.filter[:len(m.filter)-1]
				m.cursor = 0
			}
			return m, nil
		default:
			if len(key) == 1 {
				m.filter += key
				m.cursor = 0
				return m, nil
			}
		}
	}

	switch key {
	case "q", "ctrl+c":
		if m.mode == modeReader {
			m.mode = modeList
			m.docBody = ""
			m.rendered = ""
			m.rebuildViewport()
			return m, nil
		}
		return m, tea.Quit

	case "esc":
		if m.mode == modeReader {
			m.mode = modeList
			m.docBody = ""
			m.rendered = ""
			m.rebuildViewport()
			return m, nil
		}
		if m.filter != "" {
			m.filter = ""
			m.cursor = 0
		}
		return m, nil

	case "/":
		if m.mode == modeList {
			m.filterOn = true
			return m, nil
		}

	case "enter", "l", "right":
		if m.mode == modeList {
			vis := m.visibleItems()
			if m.cursor < len(vis) && !vis[m.cursor].isFolder {
				m.openDocument(vis[m.cursor].docID)
			}
		}
		return m, nil

	case "r":
		// Reload list (handy after a sync)
		if m.mode == modeList {
			m.loadDocuments()
			m.cursor = 0
		}
		return m, nil

	case "up", "k":
		if m.mode == modeList {
			if m.cursor > 0 {
				m.cursor--
			}
			return m, nil
		}
		var cmd tea.Cmd
		m.viewport, cmd = m.viewport.Update(msg)
		return m, cmd

	case "down", "j":
		if m.mode == modeList {
			if m.cursor < len(m.visibleItems())-1 {
				m.cursor++
			}
			return m, nil
		}
		var cmd tea.Cmd
		m.viewport, cmd = m.viewport.Update(msg)
		return m, cmd

	case "g":
		if m.mode == modeReader {
			m.viewport.GotoTop()
		} else {
			m.cursor = 0
		}
		return m, nil

	case "G":
		if m.mode == modeReader {
			m.viewport.GotoBottom()
		} else if vis := m.visibleItems(); len(vis) > 0 {
			m.cursor = len(vis) - 1
		}
		return m, nil

	case "pgdown", "ctrl+d", "ctrl+f", " ":
		if m.mode == modeReader {
			var cmd tea.Cmd
			m.viewport, cmd = m.viewport.Update(msg)
			return m, cmd
		}

	case "pgup", "ctrl+u", "ctrl+b":
		if m.mode == modeReader {
			var cmd tea.Cmd
			m.viewport, cmd = m.viewport.Update(msg)
			return m, cmd
		}
	}

	return m, nil
}

// View implements tea.Model.
func (m Model) View() string {
	if !m.ready {
		return "Loading..."
	}
	if m.mode == modeReader {
		return m.viewReader()
	}
	return m.viewListMode()
}

func (m Model) viewListMode() string {
	list := m.renderList()
	preview := m.renderPreview()
	body := lipgloss.JoinHorizontal(lipgloss.Top, list, preview)
	return lipgloss.JoinVertical(lipgloss.Left, body, m.helpLine())
}

func (m Model) viewReader() string {
	header := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("12")).
		PaddingLeft(1).
		Render(m.docTitle)
	return lipgloss.JoinVertical(lipgloss.Left, header, m.viewport.View(), m.helpLine())
}

func (m Model) helpLine() string {
	style := lipgloss.NewStyle().Foreground(lipgloss.Color("240")).PaddingLeft(1)
	if m.mode == modeReader {
		return style.Render("j/k scroll · g/G top/bottom · q/esc back · ctrl+c quit")
	}
	if m.filterOn {
		return style.Render(fmt.Sprintf("filter: %s_ · esc cancel · enter accept", m.filter))
	}
	hint := "j/k move · enter read · / filter · r reload · q quit"
	if m.filter != "" {
		hint = fmt.Sprintf("filter: %s · esc clear · / edit · enter read", m.filter)
	}
	return style.Render(hint)
}

func (m Model) renderList() string {
	vis := m.visibleItems()
	h := m.height - helpHeight - 2
	if h < 5 {
		h = 5
	}
	var b strings.Builder

	// Window of items around cursor — simple paging by height.
	start := 0
	if len(vis) > h && m.cursor >= h-2 {
		start = m.cursor - (h - 3)
		if start < 0 {
			start = 0
		}
		if start+h > len(vis) {
			start = len(vis) - h
		}
	}
	end := start + h
	if end > len(vis) {
		end = len(vis)
	}

	for i := start; i < end; i++ {
		it := vis[i]
		var line string
		if it.isFolder {
			line = lipgloss.NewStyle().Foreground(lipgloss.Color("244")).Bold(true).Render(it.title)
		} else {
			prefix := "  "
			label := fmt.Sprintf("%s  %s", padRight(it.docID, 18), truncRight(it.title, listPaneWidth-22))
			if i == m.cursor {
				line = lipgloss.NewStyle().
					Background(lipgloss.Color("24")).
					Foreground(lipgloss.Color("15")).
					Render(prefix + label)
			} else {
				line = prefix + label
			}
		}
		b.WriteString(line)
		b.WriteString("\n")
	}

	// Fill remaining lines so right pane lines up.
	for i := end - start; i < h; i++ {
		b.WriteString("\n")
	}

	return lipgloss.NewStyle().
		Width(listPaneWidth).
		Height(h).
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("237")).
		BorderRight(true).
		Render(b.String())
}

func (m Model) renderPreview() string {
	vis := m.visibleItems()
	w := m.width - listPaneWidth - 4
	if w < 20 {
		w = 20
	}
	h := m.height - helpHeight - 2
	if h < 5 {
		h = 5
	}
	if len(vis) == 0 || m.cursor >= len(vis) {
		return lipgloss.NewStyle().Width(w).Height(h).
			Foreground(lipgloss.Color("240")).PaddingLeft(2).PaddingTop(1).
			Render("No documents.")
	}
	it := vis[m.cursor]
	if it.isFolder {
		return lipgloss.NewStyle().Width(w).Height(h).
			Foreground(lipgloss.Color("240")).PaddingLeft(2).PaddingTop(1).
			Render("Folder — select a document.")
	}
	doc, err := m.client.GetDocumentBody(it.docID)
	if err != nil || doc == nil {
		return lipgloss.NewStyle().Width(w).Height(h).
			Foreground(lipgloss.Color("203")).PaddingLeft(2).PaddingTop(1).
			Render("Could not load document.")
	}
	r, _ := glamour.NewTermRenderer(
		glamour.WithAutoStyle(),
		glamour.WithWordWrap(w-4),
	)
	body := stripFrontmatter(doc.Body)
	rendered, _ := r.Render(body)
	header := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12")).Render(doc.Title)
	meta := lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render(
		fmt.Sprintf("%s · %s · v%s · enter to read", doc.DocumentID, doc.Status, doc.Version))
	combined := header + "\n" + meta + "\n\n" + rendered
	return lipgloss.NewStyle().Width(w).Height(h).PaddingLeft(2).PaddingTop(1).Render(combined)
}

// openDocument fetches and renders a single document into the viewport.
func (m *Model) openDocument(docID string) {
	doc, err := m.client.GetDocumentBody(docID)
	if err != nil || doc == nil {
		m.loadErr = "Could not load document: " + docID
		return
	}
	m.docBody = stripFrontmatter(doc.Body)
	m.docTitle = fmt.Sprintf("%s — %s (%s, v%s)", doc.DocumentID, doc.Title, doc.Status, doc.Version)
	m.mode = modeReader
	m.rebuildViewport()
	m.rerender()
	m.viewport.GotoTop()
}

func (m *Model) rerender() {
	if m.renderer == nil || m.docBody == "" {
		return
	}
	out, err := m.renderer.Render(m.docBody)
	if err != nil {
		m.rendered = m.docBody
	} else {
		m.rendered = out
	}
	m.viewport.SetContent(m.rendered)
}

// loadDocuments fetches the document tree and flattens it into the list.
func (m *Model) loadDocuments() {
	folders, err := m.client.ListAllDocuments()
	if err != nil {
		m.loadErr = err.Error()
		return
	}
	m.items = nil
	for _, f := range folders {
		m.collectFolder(f, f.Title, 0)
	}
}

func (m *Model) collectFolder(f client.DocFolder, label string, depth int) {
	if label == "" {
		label = f.Name
	}
	all := append([]client.DocSummary{}, f.Files...)
	for _, sub := range f.SubFolders {
		// Inline subfolder files into the parent for a flat layout — folder
		// headers stay only at the top level so the list reads as section
		// dividers, not a nested tree.
		all = append(all, sub.Files...)
	}
	if len(all) == 0 && len(f.SubFolders) == 0 {
		return
	}

	if depth == 0 {
		m.items = append(m.items, item{title: strings.ToUpper(label), isFolder: true, path: f.Name})
	}

	sort.SliceStable(all, func(i, j int) bool {
		return all[i].DocumentID < all[j].DocumentID
	})
	for _, d := range all {
		m.items = append(m.items, item{
			docID:  d.DocumentID,
			title:  d.Title,
			status: d.Status,
		})
	}
	if depth == 0 {
		for _, sub := range f.SubFolders {
			m.collectFolder(sub, sub.Title, depth+1)
		}
	}
}

func (m Model) visibleItems() []item {
	if m.filter == "" {
		return m.items
	}
	q := strings.ToLower(m.filter)
	out := make([]item, 0, len(m.items))
	for _, it := range m.items {
		if it.isFolder {
			continue
		}
		if strings.Contains(strings.ToLower(it.docID), q) ||
			strings.Contains(strings.ToLower(it.title), q) {
			out = append(out, it)
		}
	}
	return out
}

// stripFrontmatter removes a leading YAML frontmatter block from the body.
func stripFrontmatter(s string) string {
	if !strings.HasPrefix(s, "---\n") {
		return s
	}
	end := strings.Index(s[4:], "\n---\n")
	if end < 0 {
		return s
	}
	return strings.TrimLeft(s[4+end+5:], "\n")
}

func padRight(s string, n int) string {
	if len(s) >= n {
		return s[:n]
	}
	return s + strings.Repeat(" ", n-len(s))
}

func truncRight(s string, n int) string {
	if n <= 0 {
		return ""
	}
	if len(s) <= n {
		return s
	}
	if n <= 1 {
		return s[:n]
	}
	return s[:n-1] + "…"
}

// Run starts the TUI.
func Run(c *client.Client) error {
	m := New(c)
	p := tea.NewProgram(m, tea.WithAltScreen())
	_, err := p.Run()
	return err
}
