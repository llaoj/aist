package ui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
)

// selectModel is a custom select model using bubbletea
type selectModel struct {
	prompt      string
	keyHint     string
	options     []string
	selected    int
	done        bool
	interrupted bool
}

func newSelectModel(prompt, keyHint string, options []string) selectModel {
	return selectModel{
		prompt:   prompt,
		keyHint:  keyHint,
		options:  options,
		selected: 0,
	}
}

func (m selectModel) Init() tea.Cmd {
	return nil
}

func (m selectModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch msg.String() {
		case "enter":
			m.done = true
			return m, tea.Quit
		case "ctrl+c":
			m.done = true
			m.interrupted = true
			return m, tea.Quit
		case "up", "left":
			if m.selected > 0 {
				m.selected--
			}
		case "down", "right":
			if m.selected < len(m.options)-1 {
				m.selected++
			}
		}
	}

	return m, nil
}

func (m selectModel) View() tea.View {
	var b strings.Builder
	b.WriteString(m.prompt + "\n")

	for i, option := range m.options {
		cursor := " "
		checked := " "
		if m.selected == i {
			cursor = ">"
			checked = "x"
		}

		b.WriteString(fmt.Sprintf("%s [%s] %s\n", cursor, checked, option))
	}
	if !m.done && m.keyHint != "" {
		b.WriteString(keyHintStyle.Sprint(m.keyHint) + "\n")
	}

	return tea.NewView(b.String())
}

// PromptSelect displays a single-choice selector and returns the selected index.
func PromptSelect(prompt, keyHint string, options []string) (int, error) {
	model := newSelectModel(prompt, keyHint, options)
	p := tea.NewProgram(model)
	final, err := p.Run()
	fmt.Println()

	if err != nil {
		return 0, fmt.Errorf("select error: %w", err)
	}
	m := final.(selectModel)
	if m.interrupted {
		return 0, ErrInterrupted
	}
	return m.selected, nil
}
