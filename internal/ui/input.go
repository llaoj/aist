package ui

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/fatih/color"
	"github.com/llaoj/aist/internal/interactive"
	"golang.org/x/term"
)

// ErrInterrupted is returned when the user presses ctrl+c to quit the prompt.
var ErrInterrupted = interactive.ErrInterrupted

var keyHintStyle = color.New(color.Faint)

// inputModel is a custom text input model using bubbletea
type inputModel struct {
	textArea    textarea.Model
	prompt      string
	keyHint     string
	done        bool
	interrupted bool
}

func newInputModel(prompt, keyHint string) inputModel {
	ti := textarea.New()
	ti.SetStyles(textarea.DefaultStyles(true))
	ti.SetVirtualCursor(false)
	ti.Placeholder = ""
	ti.ShowLineNumbers = false
	ti.DynamicHeight = true
	ti.MinHeight = 1
	ti.MaxHeight = 15
	ti.MaxContentHeight = 1 << 20
	ti.SetHeight(1)
	ti.Focus()

	km := textarea.DefaultKeyMap()
	km.InsertNewline = key.NewBinding(
		key.WithKeys("alt+enter"),
		key.WithHelp("alt+enter", "insert newline"),
	)
	ti.KeyMap = km

	return inputModel{
		textArea: ti,
		prompt:   prompt,
		keyHint:  keyHint,
	}
}

func (m inputModel) Init() tea.Cmd {
	// Avoid textarea.Blink so idle prompts emit no periodic stdout redraws,
	// which would otherwise snap scrollback to the bottom in terminals like PuTTY.
	return tea.RequestBackgroundColor
}

func (m inputModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.textArea.SetWidth(msg.Width)

	case tea.BackgroundColorMsg:
		m.textArea.SetStyles(textarea.DefaultStyles(msg.IsDark()))

	case tea.KeyPressMsg:
		switch msg.String() {
		case "enter":
			m.textArea.Blur()
			m.done = true
			return m, tea.Quit
		case "ctrl+c":
			m.textArea.Blur()
			m.done = true
			m.interrupted = true
			return m, tea.Quit
		default:
			if !m.textArea.Focused() {
				cmd = m.textArea.Focus()
				cmds = append(cmds, cmd)
			}
		}
	}

	m.textArea, cmd = m.textArea.Update(msg)
	cmds = append(cmds, cmd)
	return m, tea.Batch(cmds...)
}

func (m inputModel) headerView() string {
	return m.prompt
}

func (m inputModel) View() tea.View {
	var c *tea.Cursor
	if !m.textArea.VirtualCursor() {
		c = m.textArea.Cursor()
		if c != nil {
			offset := lipgloss.Height(m.headerView())
			c.Y += offset
		}
	}

	lines := []string{m.headerView(), m.textArea.View()}
	// The hint disappears on submit so it does not pile up in scrollback.
	if !m.done && m.keyHint != "" {
		lines = append(lines, keyHintStyle.Sprint(m.keyHint))
	}
	f := strings.Join(append(lines, "\n"), "\n")

	v := tea.NewView(f)
	v.Cursor = c
	return v
}

// PromptInput prints a prompt and reads one line of visible input from stdin.
func PromptInput(prompt string) (string, error) {
	fmt.Fprint(os.Stdout, prompt)
	return readLine(os.Stdin)
}

func readLine(r io.Reader) (string, error) {
	line, err := bufio.NewReader(r).ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", fmt.Errorf("failed to read input: %w", err)
	}
	return strings.TrimRight(line, "\r\n"), nil
}

// PromptEditor displays a multi-line editor and returns its content.
func PromptEditor(prompt, keyHint string) (string, error) {
	model := newInputModel(prompt, keyHint)
	p := tea.NewProgram(model)
	final, err := p.Run()
	// Compensate for the trailing newline in View() which prevents bubbletea's
	// cursedRenderer.close() from erasing the last content line with EraseScreenBelow.
	fmt.Print("\033[A")
	fmt.Println()

	if err != nil {
		return "", fmt.Errorf("input error: %w", err)
	}

	m := final.(inputModel)
	if m.interrupted {
		return "", ErrInterrupted
	}
	return m.textArea.Value(), nil
}

// PromptSecret displays a prompt and reads input without echoing it, suitable
// for passwords, tokens and API keys. Surrounding whitespace is trimmed.
func PromptSecret(prompt string) (string, error) {
	fmt.Fprint(os.Stdout, prompt)
	data, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Println()
	if err != nil {
		return "", fmt.Errorf("failed to read secret: %w", err)
	}
	return strings.TrimSpace(string(data)), nil
}
