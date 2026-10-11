package tui

import (
	"context"
	"fmt"
	"io"

	tea "github.com/charmbracelet/bubbletea"
)

// Run starts the UI on in/out and blocks until the user quits or ctx ends.
func Run(ctx context.Context, api API, opts Options, in io.Reader, out io.Writer) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	p := tea.NewProgram(New(ctx, api, opts), tea.WithAltScreen(), tea.WithInput(in), tea.WithOutput(out), tea.WithContext(ctx))
	if _, err := p.Run(); err != nil {
		return fmt.Errorf("run terminal UI: %w", err)
	}
	return nil
}
