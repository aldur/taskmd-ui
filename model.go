package main

import (
	"fmt"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
)

// reloadInterval is how often the task tree is re-read. Tasks are edited from
// outside the TUI constantly — by `taskmd`, by an editor, by an agent — so the
// list follows the files rather than a snapshot taken at startup. Reading a
// few hundred small markdown files is cheap enough to just redo.
const reloadInterval = 2 * time.Second

// noticeLifetime is how long an action's confirmation stays on the footer.
const noticeLifetime = 4 * time.Second

type focusArea int

const (
	focusList focusArea = iota
	focusDetail
)

// detailBreakpoint is the width below which the detail pane stops sharing the
// screen with the list and becomes a view you switch to instead.
const detailBreakpoint = 120

type model struct {
	proj Project

	tasks   []Task // everything on disk
	visible []Task // what the filters and sort leave, in display order
	cursor  int
	offset  int // first visible row, for scrolling

	filters Filters
	order   Sort

	search    textinput.Model
	searching bool

	detail     viewport.Model
	focus      focusArea
	detailFor  string // task id the viewport currently holds
	showHelp   bool
	helpOffset int // first visible line of the help page

	reading            bool // full-screen, read-only task view
	readerReturnOffset int  // restore the preview's scroll position on exit

	width, height int

	notice    string
	noticeErr bool
	noticeAt  time.Time
	loadErr   error

	ready bool
}

func newModel(proj Project) model {
	search := textinput.New()
	search.Prompt = "/"
	search.Placeholder = "filter by id, title, tag, body…"

	return model{
		proj:   proj,
		order:  Sort{Field: SortPriority},
		search: search,
		detail: viewport.New(0, 0),
	}
}

func (m model) Init() tea.Cmd {
	return tea.Batch(loadCmd(m.proj.TaskDir), tickCmd())
}

// tasksMsg carries a completed scan of the task directory.
type tasksMsg struct {
	tasks []Task
	err   error
}

// actionMsg reports the outcome of a `taskmd` invocation.
type actionMsg struct {
	notice string
	err    error
}

type tickMsg time.Time

func loadCmd(dir string) tea.Cmd {
	return func() tea.Msg {
		tasks, err := loadTasks(dir)
		return tasksMsg{tasks: tasks, err: err}
	}
}

func tickCmd() tea.Cmd {
	return tea.Tick(reloadInterval, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.ready = true
		m.layout()
		return m, nil

	case tickMsg:
		// Reload on a timer rather than watching the filesystem: no extra
		// dependency, and a two-second lag on an external edit is invisible.
		return m, tea.Batch(loadCmd(m.proj.TaskDir), tickCmd())

	case tasksMsg:
		m.loadErr = msg.err
		if msg.err == nil {
			m.tasks = msg.tasks
		}
		m.refilter()
		return m, nil

	case actionMsg:
		if msg.err != nil {
			m.setNotice(msg.err.Error(), true)
		} else {
			m.setNotice(msg.notice, false)
		}
		// The action wrote through `taskmd`; pick the change up now rather
		// than waiting out the tick.
		return m, loadCmd(m.proj.TaskDir)

	case tea.KeyMsg:
		return m.handleKey(msg)
	}

	return m, nil
}

func (m model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// The filter prompt swallows every key but the two that leave it, so
	// typing `q` or `j` into a query does what it looks like.
	if m.searching {
		switch msg.String() {
		case "esc":
			m.searching = false
			m.search.Blur()
			m.search.SetValue(m.filters.Query.Raw)
			return m, nil
		case "enter":
			m.searching = false
			m.search.Blur()
			return m, nil
		}
		var cmd tea.Cmd
		m.search, cmd = m.search.Update(msg)
		// Live: every keystroke re-parses and re-filters, so the list narrows
		// as the expression is typed. A half-typed term parses as an error and
		// is dropped, which keeps the rest of it filtering meanwhile.
		m.filters.Query = ParseQuery(m.search.Value())
		m.refilter()
		return m, cmd
	}

	if m.showHelp {
		switch msg.String() {
		case "?", "esc", "q", "ctrl+c":
			m.showHelp = false
			m.helpOffset = 0
		case "j", "down":
			m.scrollHelp(1)
		case "k", "up":
			m.scrollHelp(-1)
		case "ctrl+d", "pgdown", " ":
			m.scrollHelp(maxInt(1, m.height/2))
		case "ctrl+u", "pgup":
			m.scrollHelp(-maxInt(1, m.height/2))
		case "g", "home":
			m.helpOffset = 0
		case "G", "end":
			m.scrollHelp(m.helpLength())
		}
		return m, nil
	}
	if m.reading {
		return m.handleReaderKey(msg)
	}

	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "?":
		m.showHelp = true
		return m, nil
	case "ctrl+f":
		if m.splitPanes() || m.focus == focusDetail {
			m.detail.ViewDown()
		}
		return m, nil
	case "ctrl+b":
		if m.splitPanes() || m.focus == focusDetail {
			m.detail.ViewUp()
		}
		return m, nil

	// Navigation. The same keys drive whichever pane has focus, so the detail
	// scrolls with j/k once you tab into it.
	case "j", "down":
		if m.focus == focusDetail {
			m.detail.LineDown(1)
			return m, nil
		}
		m.move(1)
	case "k", "up":
		if m.focus == focusDetail {
			m.detail.LineUp(1)
			return m, nil
		}
		m.move(-1)
	case "ctrl+d", "pgdown":
		if m.focus == focusDetail {
			m.detail.HalfViewDown()
			return m, nil
		}
		m.move(maxInt(1, m.listHeight()/2))
	case "ctrl+u", "pgup":
		if m.focus == focusDetail {
			m.detail.HalfViewUp()
			return m, nil
		}
		m.move(-maxInt(1, m.listHeight()/2))
	case "g", "home":
		if m.focus == focusDetail {
			m.detail.GotoTop()
			return m, nil
		}
		m.moveTo(0)
	case "G", "end":
		if m.focus == focusDetail {
			m.detail.GotoBottom()
			return m, nil
		}
		m.moveTo(len(m.visible) - 1)
	case "tab":
		m.focus = 1 - m.focus
		return m, nil
	case "enter":
		if _, ok := m.selected(); ok {
			m.readerReturnOffset = m.detail.YOffset
			m.reading = true
			m.layout()
			m.detail.GotoTop()
		}
		return m, nil
	case "esc":
		m.focus = focusList
		return m, nil

	// Filtering.
	case "/":
		m.searching = true
		m.search.SetValue(m.filters.Query.Raw)
		m.search.CursorEnd()
		return m, m.search.Focus()
	case "o":
		m.filters.OpenOnly = !m.filters.OpenOnly
		// The two are contradictory: an explicit `status:completed` and
		// open-only would always come up empty.
		if m.filters.OpenOnly {
			m.filters.Status = anyValue
		}
		m.refilter()
	case "f", "F":
		m.filters.Status = cycle(m.facet(func(t Task) []string { return single(t.Status) }), m.filters.Status, msg.String() == "F")
		if m.filters.Status != anyValue {
			m.filters.OpenOnly = false
		}
		m.refilter()
	case "p", "P":
		m.filters.Priority = cycle(m.facet(func(t Task) []string { return single(t.Priority) }), m.filters.Priority, msg.String() == "P")
		m.refilter()
	case "t", "T":
		m.filters.Tag = cycle(m.facet(func(t Task) []string { return t.Tags }), m.filters.Tag, msg.String() == "T")
		m.refilter()
	case "h", "H":
		m.filters.Phase = cycle(m.facet(func(t Task) []string { return single(t.Phase) }), m.filters.Phase, msg.String() == "H")
		m.refilter()
	case "w", "W":
		m.filters.Owner = cycle(m.facet(func(t Task) []string { return single(t.Owner) }), m.filters.Owner, msg.String() == "W")
		m.refilter()
	case "n", "N":
		m.filters.Group = cycle(m.facet(func(t Task) []string { return single(t.Group) }), m.filters.Group, msg.String() == "N")
		m.refilter()
	case "y", "Y":
		m.filters.Type = cycle(m.facet(func(t Task) []string { return single(t.Type) }), m.filters.Type, msg.String() == "Y")
		m.refilter()
	case "c":
		m.filters = Filters{}
		m.search.SetValue("")
		m.refilter()

	// Sorting.
	case "s":
		m.order = m.order.Next()
		m.refilter()
	case "S":
		m.order.Reverse = !m.order.Reverse
		m.refilter()

	// Actions.
	case "r":
		return m, loadCmd(m.proj.TaskDir)
	case "e":
		task, ok := m.selected()
		if !ok {
			return m, nil
		}
		return m, editCmd(task)
	case " ":
		task, ok := m.selected()
		if !ok {
			return m, nil
		}
		next := advanceStatus(task.Status)
		return m, setStatusCmd(m.proj, task, next)
	case "1", "2", "3", "4", "5", "6":
		task, ok := m.selected()
		if !ok {
			return m, nil
		}
		status := Statuses[int(msg.String()[0]-'1')]
		return m, setStatusCmd(m.proj, task, status)
	}

	return m, nil
}

// The reader accepts navigation and reloads only. All editing, status,
// filtering and sorting shortcuts stop here so the task stays under focus.
func (m model) handleReaderKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "enter", "tab":
		m.reading = false
		m.layout()
		m.detail.SetYOffset(m.readerReturnOffset)
	case "q", "ctrl+c":
		return m, tea.Quit
	case "?":
		m.showHelp = true
	case "j", "down":
		m.detail.LineDown(1)
	case "k", "up":
		m.detail.LineUp(1)
	case "ctrl+d", "pgdown":
		m.detail.HalfViewDown()
	case "ctrl+u", "pgup":
		m.detail.HalfViewUp()
	case "ctrl+f":
		m.detail.ViewDown()
	case "ctrl+b":
		m.detail.ViewUp()
	case " ":
		m.detail.ViewDown()
	case "g", "home":
		m.detail.GotoTop()
	case "G", "end":
		m.detail.GotoBottom()
	case "r":
		return m, loadCmd(m.proj.TaskDir)
	}
	return m, nil
}

// facet lists the values a field takes across every loaded task — the cycle a
// filter key steps through.
func (m model) facet(get func(Task) []string) []string {
	return facetValues(m.tasks, get)
}

// refilter recomputes the visible list, keeping the cursor on the same task
// where it survives the new filters.
func (m *model) refilter() {
	selectedID := ""
	if task, ok := m.selected(); ok {
		selectedID = task.ID
	}

	m.visible = Apply(m.tasks, m.filters, m.order)

	m.cursor = 0
	if selectedID != "" {
		for i, t := range m.visible {
			if t.ID == selectedID {
				m.cursor = i
				break
			}
		}
	}
	if m.reading && (len(m.visible) == 0 || m.visible[m.cursor].ID != selectedID) {
		// A deleted or newly filtered-out task should not silently turn the
		// reader into a different task after an external edit.
		m.reading = false
		m.showHelp = false
		m.helpOffset = 0
		m.setNotice(fmt.Sprintf("Task %s left the current list", selectedID), false)
		m.layout()
		return
	}
	m.clampCursor()
	m.syncDetail()
}

func (m model) selected() (Task, bool) {
	if m.cursor < 0 || m.cursor >= len(m.visible) {
		return Task{}, false
	}
	return m.visible[m.cursor], true
}

func (m *model) move(delta int) {
	m.moveTo(m.cursor + delta)
}

func (m *model) moveTo(index int) {
	m.cursor = index
	m.clampCursor()
	m.syncDetail()
}

func (m *model) clampCursor() {
	if m.cursor >= len(m.visible) {
		m.cursor = len(m.visible) - 1
	}
	if m.cursor < 0 {
		m.cursor = 0
	}

	// Keep the cursor inside the scrolled window.
	height := m.listHeight()
	if height <= 0 {
		m.offset = 0
		return
	}
	if m.cursor < m.offset {
		m.offset = m.cursor
	}
	if m.cursor >= m.offset+height {
		m.offset = m.cursor - height + 1
	}
	if max := len(m.visible) - height; m.offset > max {
		m.offset = max
	}
	if m.offset < 0 {
		m.offset = 0
	}
}

// syncDetail refills the detail viewport, rewinding it only when the selection
// actually changed — a reload under a stationary cursor should not throw away
// where the reader had scrolled to.
func (m *model) syncDetail() {
	task, ok := m.selected()
	if !ok {
		m.detail.SetContent("")
		m.detailFor = ""
		return
	}
	m.detail.SetContent(renderDetail(m.proj, task, m.detail.Width))
	m.detail.SetYOffset(m.detail.YOffset)
	if m.detailFor != task.ID {
		m.detailFor = task.ID
		m.detail.GotoTop()
	}
}

// scrollHelp moves the help page, stopping at its last screenful.
func (m *model) scrollHelp(delta int) {
	m.helpOffset = minInt(maxInt(m.helpOffset+delta, 0), maxInt(0, m.helpLength()-maxInt(1, m.height-1)))
}

func (m *model) setNotice(text string, isErr bool) {
	m.notice = text
	m.noticeErr = isErr
	m.noticeAt = time.Now()
}

// layout recomputes the pane geometry after a resize.
func (m *model) layout() {
	width, height := m.detailSize()
	m.detail.Width = width
	m.detail.Height = height
	m.search.Width = maxInt(1, m.width-2) // prompt and cursor each need a cell
	m.clampCursor()
	m.syncDetail()
}

// splitPanes reports whether the list and the detail share the screen.
func (m model) splitPanes() bool { return !m.reading && m.width >= detailBreakpoint }

// bodyHeight is the space between the header and the footer.
func (m model) bodyHeight() int {
	return maxInt(0, m.height-m.headerHeight()-m.footerHeight())
}

// Keep at least one content row when the terminal is very short.
func (m model) headerHeight() int {
	if m.reading {
		return 0
	}
	if m.height >= 5 {
		return 2
	}
	if m.height >= 2 {
		return 1
	}
	return 0
}

func (m model) footerHeight() int {
	if m.height >= 3 {
		return 1
	}
	return 0
}

// Borders give way to content when there is no room for both.
func (m model) paneFrameSize() int {
	if m.reading {
		return 0
	}
	if m.width >= 4 && m.bodyHeight() >= 3 {
		return 2
	}
	return 0
}

// listWidth is the outer width of the list pane, borders included.
func (m model) listWidth() int {
	if !m.splitPanes() {
		return m.width
	}
	// A little over half to the list: the titles are what you scan, the detail
	// is what you read once you have found it.
	return m.width * 55 / 100
}

// listHeight is how many task rows fit.
func (m model) listHeight() int {
	return maxInt(0, m.bodyHeight()-m.paneFrameSize())
}

func (m model) detailSize() (width, height int) {
	if m.splitPanes() {
		return maxInt(0, m.width-m.listWidth()-m.paneFrameSize()), m.listHeight()
	}
	return maxInt(0, m.width-m.paneFrameSize()), m.listHeight()
}

// advanceStatus is the next state along the spec's happy path, which is what
// the space bar applies. Terminal and blocked states fold back into the flow
// so the key always does something.
func advanceStatus(current string) string {
	switch current {
	case "pending", "blocked", "":
		return "in-progress"
	case "in-progress":
		return "in-review"
	case "in-review":
		return "completed"
	default:
		return "pending"
	}
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// noticeText is the footer message while it is still fresh.
func (m model) noticeText() (string, bool) {
	if m.notice == "" || time.Since(m.noticeAt) > noticeLifetime {
		return "", false
	}
	return m.notice, m.noticeErr
}

func plural(n int, word string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, word)
	}
	return fmt.Sprintf("%d %ss", n, word)
}
