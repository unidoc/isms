// Package tui provides an interactive terminal UI for the ISMS, focused on
// reading documents. A collapsible folder tree on the left mirroring the real
// documents/ directory (template → subfolders → docs, closed by default), and the
// selected document rendered in a scrollable pane on the right. The tree stays
// visible while reading: enter/→ moves focus into the document to scroll it,
// esc/←/h returns focus to the tree.
package tui

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
	"isms.sh/internal/isms/store"
)

// treeItem is one row of the flattened document tree (pre-order DFS). Folders
// carry an expand flag and depth; a folder's descendants are the following items
// with a greater depth. Documents carry cached metadata + body (offline read).
type treeItem struct {
	isFolder bool
	label    string // folder .title/name, or document title
	depth    int
	expanded bool // folders only
	docID    string
	status   string
	version  string
	body     string // markdown body (frontmatter already parsed out by store)
	path     string // absolute file path (for opening in $EDITOR)
}

// display is the row's shown name — the label, falling back to the document id
// when a doc has no title (matches how the web viewer labels documents).
func (it treeItem) display() string {
	if !it.isFolder && strings.TrimSpace(it.label) == "" {
		return it.docID
	}
	return it.label
}

// tnode is the nested tree built while reading the clone; flattened into
// []treeItem for display.
type tnode struct {
	isFolder bool
	label    string
	docID    string
	status   string
	version  string
	body     string
	path     string
	children []tnode
}

// editDoneMsg is delivered after $EDITOR exits (idx = the item edited).
type editDoneMsg struct {
	idx int
	err error
}

// Model is the bubbletea model for the document TUI.
type Model struct {
	store      *store.Store
	width      int
	height     int
	ready      bool
	reading    bool // focus is on the document pane (scrolling) vs the tree
	items      []treeItem
	cursor     int    // index into visibleRows()
	listOffset int    // first visible row (stable scroll)
	loadedDoc  string // docID currently rendered in the viewport
	filter     string
	filterOn   bool
	viewport   viewport.Model
	renderer   *glamour.TermRenderer
	loadErr    string
}

const (
	listPaneWidth = 38
	helpHeight    = 1
	readerHeader  = 2 // title + meta lines above the scrollable body
)

// New creates a document-focused TUI model that reads the local clone at root.
func New(root string) Model {
	r, _ := glamour.NewTermRenderer(
		glamour.WithAutoStyle(),
		glamour.WithWordWrap(80),
	)
	m := Model{
		store:    store.New(root),
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
		m.loadedDoc = "" // force a re-render at the new width
		m.syncPreview()
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)

	case editDoneMsg:
		// Re-read the edited file from the working tree so the change shows.
		if msg.err == nil && msg.idx >= 0 && msg.idx < len(m.items) && m.items[msg.idx].path != "" {
			if pf, err := m.store.LoadDocument(m.items[msg.idx].path); err == nil {
				m.items[msg.idx].label = pf.Frontmatter.Title
				m.items[msg.idx].status = pf.Frontmatter.Status
				m.items[msg.idx].version = pf.Frontmatter.Version
				m.items[msg.idx].body = pf.Body
			}
		}
		m.loadedDoc = "" // force the viewport to re-render
		m.syncPreview()
		return m, nil
	}
	return m, nil
}

// editCurrent opens the selected document in $EDITOR (if set), suspending the TUI
// while the editor runs. Read-only stays the default; this is the power-user path
// to edit the raw markdown in the local clone (edits become un-synced working-tree
// changes, consistent with clone→edit→sync).
func (m Model) editCurrent() (tea.Model, tea.Cmd) {
	it, ok := m.currentItem()
	if !ok || it.isFolder || it.path == "" {
		return m, nil
	}
	editor := strings.TrimSpace(os.Getenv("EDITOR"))
	if editor == "" {
		m.loadErr = "Set $EDITOR to edit documents from the reader."
		return m, nil
	}
	idx := m.cursorItemIndex()
	// $EDITOR may carry flags (e.g. "code -w"); split into command + args.
	parts := strings.Fields(editor)
	args := append(parts[1:], it.path)
	c := exec.Command(parts[0], args...)
	return m, tea.ExecProcess(c, func(err error) tea.Msg {
		return editDoneMsg{idx: idx, err: err}
	})
}

// rightPaneWidth is the usable width of the document pane.
func (m Model) rightPaneWidth() int {
	w := m.width - listPaneWidth - 4
	if w < 20 {
		w = 20
	}
	return w
}

// listHeight is the number of rows in each pane (tree rows / reader lines).
func (m Model) listHeight() int {
	h := m.height - helpHeight - 2
	if h < 5 {
		h = 5
	}
	return h
}

func (m *Model) rebuildViewport() {
	w := m.rightPaneWidth()
	h := m.listHeight() - readerHeader
	if h < 3 {
		h = 3
	}
	m.viewport = viewport.New(w, h)
	m.viewport.Style = lipgloss.NewStyle().PaddingLeft(1)
	m.renderer, _ = glamour.NewTermRenderer(
		glamour.WithAutoStyle(),
		glamour.WithWordWrap(w-2),
	)
}

// syncPreview loads the currently-selected document into the viewport (once per
// selection) so the right pane always reflects the tree cursor. Folders clear it.
func (m *Model) syncPreview() {
	it, ok := m.currentItem()
	if !ok || it.isFolder {
		m.loadedDoc = ""
		return
	}
	if it.docID == m.loadedDoc {
		return
	}
	m.loadedDoc = it.docID
	body := it.body
	if m.renderer != nil {
		if out, err := m.renderer.Render(it.body); err == nil {
			body = out
		}
	}
	m.viewport.SetContent(body)
	m.viewport.GotoTop()
}

// currentItem returns the tree item under the cursor.
func (m Model) currentItem() (treeItem, bool) {
	rows := m.visibleRows()
	if m.cursor < 0 || m.cursor >= len(rows) {
		return treeItem{}, false
	}
	return m.items[rows[m.cursor]], true
}

// clampCursor keeps the cursor in range and scrolls the tree window just enough
// to keep it visible — the offset moves only when the cursor would leave the pane.
func (m *Model) clampCursor(n int) {
	if n <= 0 {
		m.cursor = 0
		m.listOffset = 0
		return
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
	if m.cursor > n-1 {
		m.cursor = n - 1
	}
	h := m.listHeight()
	if m.cursor < m.listOffset {
		m.listOffset = m.cursor
	}
	if m.cursor >= m.listOffset+h {
		m.listOffset = m.cursor - h + 1
	}
	if m.listOffset < 0 {
		m.listOffset = 0
	}
	if m.listOffset > n-1 {
		m.listOffset = n - 1
	}
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	// Filter input mode — keys flow into the filter string.
	if m.filterOn {
		switch key {
		case "esc":
			m.filterOn = false
			m.filter = ""
			m.cursor, m.listOffset = 0, 0
			m.syncPreview()
			return m, nil
		case "enter":
			m.filterOn = false
			return m, nil
		case "backspace":
			if len(m.filter) > 0 {
				m.filter = m.filter[:len(m.filter)-1]
				m.cursor, m.listOffset = 0, 0
				m.syncPreview()
			}
			return m, nil
		default:
			if len(key) == 1 {
				m.filter += key
				m.cursor, m.listOffset = 0, 0
				m.syncPreview()
				return m, nil
			}
			return m, nil
		}
	}

	// Edit the selected document in $EDITOR (works from tree or reader focus).
	if key == "e" {
		return m.editCurrent()
	}

	// Reader focus — keys scroll the document; esc/←/h/q return to the tree.
	if m.reading {
		switch key {
		case "ctrl+c":
			return m, tea.Quit
		case "esc", "q", "h", "left":
			m.reading = false
			return m, nil
		case "g":
			m.viewport.GotoTop()
			return m, nil
		case "G":
			m.viewport.GotoBottom()
			return m, nil
		case "ctrl+f", "pgdown", " ":
			m.viewport.ViewDown()
			return m, nil
		case "ctrl+b", "pgup":
			m.viewport.ViewUp()
			return m, nil
		case "ctrl+d":
			m.viewport.HalfViewDown()
			return m, nil
		case "ctrl+u":
			m.viewport.HalfViewUp()
			return m, nil
		default:
			var cmd tea.Cmd
			m.viewport, cmd = m.viewport.Update(msg)
			return m, cmd
		}
	}

	// Tree focus.
	switch key {
	case "q", "ctrl+c":
		return m, tea.Quit

	case "esc":
		if m.filter != "" {
			m.filter = ""
			m.clampCursor(len(m.visibleRows()))
			m.syncPreview()
		}
		return m, nil

	case "/":
		m.filterOn = true
		return m, nil

	case "enter", "l", "right":
		rows := m.visibleRows()
		if m.cursor < len(rows) {
			it := m.items[rows[m.cursor]]
			if it.isFolder {
				if m.filter == "" {
					m.items[rows[m.cursor]].expanded = !it.expanded
					m.clampCursor(len(m.visibleRows()))
				}
			} else {
				// Focus the document to scroll it — the tree stays visible.
				m.reading = true
			}
		}
		return m, nil

	case "h", "left":
		if m.filter == "" {
			rows := m.visibleRows()
			if m.cursor < len(rows) {
				ci := rows[m.cursor]
				it := m.items[ci]
				if it.isFolder && it.expanded {
					m.items[ci].expanded = false
				} else {
					for j := ci - 1; j >= 0; j-- {
						if m.items[j].isFolder && m.items[j].depth < it.depth {
							m.items[j].expanded = false
							m.cursor = visIndexOf(m.visibleRows(), j)
							break
						}
					}
				}
				m.clampCursor(len(m.visibleRows()))
				m.syncPreview()
			}
		}
		return m, nil

	case "r":
		m.loadDocuments()
		m.cursor, m.listOffset = 0, 0
		m.loadedDoc = ""
		m.syncPreview()
		return m, nil

	case "up", "k":
		m.cursor--
		m.clampCursor(len(m.visibleRows()))
		m.syncPreview()
		return m, nil

	case "down", "j":
		m.cursor++
		m.clampCursor(len(m.visibleRows()))
		m.syncPreview()
		return m, nil

	case "g":
		m.cursor = 0
		m.clampCursor(len(m.visibleRows()))
		m.syncPreview()
		return m, nil

	case "G":
		m.cursor = len(m.visibleRows()) - 1
		m.clampCursor(len(m.visibleRows()))
		m.syncPreview()
		return m, nil
	}

	return m, nil
}

// visIndexOf returns the position of item index `it` within the visible rows.
func visIndexOf(rows []int, it int) int {
	for i, r := range rows {
		if r == it {
			return i
		}
	}
	return 0
}

// View implements tea.Model.
func (m Model) View() string {
	if !m.ready {
		return "Loading..."
	}
	body := lipgloss.JoinHorizontal(lipgloss.Top, m.renderList(), m.renderReader())
	return lipgloss.JoinVertical(lipgloss.Left, body, m.helpLine())
}

func (m Model) helpLine() string {
	style := lipgloss.NewStyle().Foreground(lipgloss.Color("240")).PaddingLeft(1)
	if m.filterOn {
		return style.Render(fmt.Sprintf("filter: %s_ · esc cancel · enter accept", m.filter))
	}
	if m.reading {
		return style.Render("j/k scroll · ctrl+f/b page · g/G top/bottom · e edit · esc/←/h tree · ctrl+c quit")
	}
	hint := "j/k move · enter/→ open/expand · ←/h collapse · e edit · / filter · r reload · q quit"
	if m.filter != "" {
		hint = fmt.Sprintf("filter: %s · esc clear · / edit · enter read", m.filter)
	}
	return style.Render(hint)
}

func (m Model) renderList() string {
	rows := m.visibleRows()
	h := m.listHeight()
	var b strings.Builder

	end := m.listOffset + h
	if end > len(rows) {
		end = len(rows)
	}
	inner := listPaneWidth - 2

	for i := m.listOffset; i < end; i++ {
		it := m.items[rows[i]]
		selected := i == m.cursor
		indent := strings.Repeat("  ", it.depth)
		var line string
		if it.isFolder {
			glyph := "▸"
			if it.expanded || m.filter != "" {
				glyph = "▾"
			}
			label := truncRight(indent+glyph+" "+it.display(), inner)
			st := lipgloss.NewStyle().Foreground(lipgloss.Color("244")).Bold(true)
			if selected {
				st = st.Background(lipgloss.Color("236"))
			}
			line = st.Render(padRight(label, inner))
		} else {
			label := truncRight(indent+"  "+it.display(), inner)
			if selected {
				bg := lipgloss.Color("238") // dimmed when focus is on the reader
				if !m.reading {
					bg = lipgloss.Color("24")
				}
				line = lipgloss.NewStyle().Background(bg).Foreground(lipgloss.Color("15")).
					Render(padRight(label, inner))
			} else {
				line = label
			}
		}
		b.WriteString(line)
		b.WriteString("\n")
	}
	for i := end - m.listOffset; i < h; i++ {
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

// renderReader draws the right pane: a folder placeholder, or the selected
// document's title/meta header above its scrollable body. The tree always stays
// visible alongside it.
func (m Model) renderReader() string {
	w := m.rightPaneWidth()
	h := m.listHeight()
	dim := lipgloss.NewStyle().Width(w).Height(h).
		Foreground(lipgloss.Color("240")).PaddingLeft(2).PaddingTop(1)

	it, ok := m.currentItem()
	if !ok {
		if m.loadErr != "" {
			return dim.Foreground(lipgloss.Color("203")).Render(m.loadErr)
		}
		return dim.Render("No documents.")
	}
	if it.isFolder {
		verb := "enter/→ to expand"
		if it.expanded {
			verb = "←/h to collapse"
		}
		return dim.Render(fmt.Sprintf("%s\n\n%d document(s) · %s", it.display(), m.countDocs(m.cursorItemIndex()), verb))
	}

	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12")).PaddingLeft(1)
	if m.reading {
		titleStyle = titleStyle.Foreground(lipgloss.Color("14"))
	}
	header := titleStyle.Render(truncRight(it.display(), w-2))
	meta := lipgloss.NewStyle().Foreground(lipgloss.Color("240")).PaddingLeft(1).Render(
		truncRight(fmt.Sprintf("%s · %s · v%s%s", it.docID, it.status, it.version,
			readerHint(m.reading)), w-2))
	return lipgloss.JoinVertical(lipgloss.Left, header, meta, m.viewport.View())
}

func readerHint(reading bool) string {
	if reading {
		return "   [reading — esc to tree]"
	}
	return "   [enter to read]"
}

// cursorItemIndex returns the m.items index under the cursor (-1 if none).
func (m Model) cursorItemIndex() int {
	rows := m.visibleRows()
	if m.cursor < 0 || m.cursor >= len(rows) {
		return -1
	}
	return rows[m.cursor]
}

// countDocs counts the document descendants of the folder at item index fi.
func (m Model) countDocs(fi int) int {
	if fi < 0 {
		return 0
	}
	d := m.items[fi].depth
	n := 0
	for j := fi + 1; j < len(m.items) && m.items[j].depth > d; j++ {
		if !m.items[j].isFolder {
			n++
		}
	}
	return n
}

// loadDocuments reads the local clone into a nested folder tree that mirrors the
// real documents/ layout (template → subfolders → docs), with .title display
// names at every level — the same structure the web viewer shows. Folders start
// collapsed; a lone top-level folder opens so the reader isn't a single dead row.
func (m *Model) loadDocuments() {
	m.items = nil
	m.loadErr = ""
	var errs []string

	docsRoot := m.store.DocsRoot()
	entries, err := m.store.ReadDir(docsRoot)
	if err != nil {
		m.loadErr = "No documents found in the local clone (looked under documents/)."
		return
	}
	var tops []string
	for _, de := range entries {
		if de.IsDir() && !strings.HasPrefix(de.Name(), ".") {
			tops = append(tops, de.Name())
		}
	}
	sort.Strings(tops)

	var roots []tnode
	for _, td := range tops {
		if n, ok := m.buildFolder(filepath.Join(docsRoot, td), &errs); ok {
			roots = append(roots, n)
		}
	}

	var flatten func(nodes []tnode, depth int)
	flatten = func(nodes []tnode, depth int) {
		for _, n := range nodes {
			m.items = append(m.items, treeItem{
				isFolder: n.isFolder, label: n.label, depth: depth,
				docID: n.docID, status: n.status, version: n.version, body: n.body, path: n.path,
			})
			if n.isFolder {
				flatten(n.children, depth+1)
			}
		}
	}
	flatten(roots, 0)

	if len(roots) == 1 && len(m.items) > 0 && m.items[0].isFolder {
		m.items[0].expanded = true
	}

	if len(errs) > 0 {
		m.loadErr = "Failed to load: " + strings.Join(errs, "; ")
	} else if len(m.items) == 0 {
		m.loadErr = "No documents found in the local clone (looked under documents/)."
	}
}

// buildFolder recursively reads dir into a tnode (subfolders first, then docs by
// id). Returns ok=false for an empty folder so bare directories don't clutter the
// tree. Load errors are collected, not fatal.
func (m *Model) buildFolder(dir string, errs *[]string) (tnode, bool) {
	n := tnode{isFolder: true, label: m.folderLabel(dir)}
	entries, err := m.store.ReadDir(dir)
	if err != nil {
		*errs = append(*errs, fmt.Sprintf("%s: %v", filepath.Base(dir), err))
		return n, false
	}
	var subdirs, files []string
	for _, de := range entries {
		if strings.HasPrefix(de.Name(), ".") {
			continue
		}
		if de.IsDir() {
			subdirs = append(subdirs, de.Name())
		} else if strings.HasSuffix(de.Name(), ".md") {
			files = append(files, de.Name())
		}
	}
	sort.Strings(subdirs)
	sort.Strings(files)

	for _, sd := range subdirs {
		if sub, ok := m.buildFolder(filepath.Join(dir, sd), errs); ok {
			n.children = append(n.children, sub)
		}
	}
	var docs []tnode
	for _, f := range files {
		path := filepath.Join(dir, f)
		pf, lerr := m.store.LoadDocument(path)
		if lerr != nil {
			*errs = append(*errs, fmt.Sprintf("%s: %v", filepath.Base(dir), lerr))
			continue
		}
		docs = append(docs, tnode{
			docID:   pf.Frontmatter.DocumentID,
			label:   pf.Frontmatter.Title,
			status:  pf.Frontmatter.Status,
			version: pf.Frontmatter.Version,
			body:    pf.Body,
			path:    path,
		})
	}
	sort.SliceStable(docs, func(i, j int) bool { return docs[i].docID < docs[j].docID })
	n.children = append(n.children, docs...)
	return n, len(n.children) > 0
}

// folderLabel returns a folder's display name — the content of its .title file,
// the same convention the web/API folder tree honors — falling back to the raw
// directory name when no .title is set.
func (m *Model) folderLabel(dir string) string {
	data, err := m.store.ReadFile(filepath.Join(dir, ".title"))
	if err != nil || len(data) == 0 {
		return filepath.Base(dir)
	}
	return strings.TrimSpace(string(data))
}

// visibleRows flattens the tree into the item indices currently on screen: folder
// headers plus the contents of expanded folders. While filtering, only folders on
// the path to a matching document appear, with just the matching docs.
func (m Model) visibleRows() []int {
	if m.filter != "" {
		return m.filteredRows()
	}
	var rows []int
	skip := -1
	for i := range m.items {
		it := m.items[i]
		if skip >= 0 {
			if it.depth > skip {
				continue
			}
			skip = -1
		}
		rows = append(rows, i)
		if it.isFolder && !it.expanded {
			skip = it.depth
		}
	}
	return rows
}

// filteredRows returns matching documents and their ancestor folders, in order.
func (m Model) filteredRows() []int {
	q := strings.ToLower(m.filter)
	keep := make([]bool, len(m.items))
	var stack []int
	for i := range m.items {
		it := m.items[i]
		for len(stack) > 0 && m.items[stack[len(stack)-1]].depth >= it.depth {
			stack = stack[:len(stack)-1]
		}
		if it.isFolder {
			stack = append(stack, i)
			continue
		}
		if strings.Contains(strings.ToLower(it.docID), q) ||
			strings.Contains(strings.ToLower(it.label), q) {
			keep[i] = true
			for _, a := range stack {
				keep[a] = true
			}
		}
	}
	var rows []int
	for i := range m.items {
		if keep[i] {
			rows = append(rows, i)
		}
	}
	return rows
}

func padRight(s string, n int) string {
	w := len([]rune(s))
	if w >= n {
		return s
	}
	return s + strings.Repeat(" ", n-w)
}

func truncRight(s string, n int) string {
	if n <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	if n <= 1 {
		return string(r[:n])
	}
	return string(r[:n-1]) + "…"
}

// Run starts the TUI against the local clone at root.
func Run(root string) error {
	m := New(root)
	p := tea.NewProgram(m, tea.WithAltScreen())
	_, err := p.Run()
	return err
}
