package ai

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

const (
	droppedToolOutput = "[tool output dropped to fit the model context window]"
	bytesPerToken     = 4
	// outputReserveTokens is room left for the model's answer.
	outputReserveTokens = 4096
)

// TruncateToolOutput caps s at limit bytes on a rune boundary and appends a
// marker saying how many bytes were cut. limit <= 0 disables the cap.
func TruncateToolOutput(s string, limit int) string {
	if limit <= 0 || len(s) <= limit {
		return s
	}
	cut := limit
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return fmt.Sprintf("%s\n[truncated %d bytes]", s[:cut], len(s)-cut)
}

func estimateTokens(system string, tools []ToolSpec, msgs []Message) int {
	n := len(system)
	for _, t := range tools {
		n += len(t.Name) + len(t.Description) + len(t.InputSchema)
	}
	for _, m := range msgs {
		for _, b := range m.Content {
			n += len(b.Text) + len(b.ToolName) + len(b.ToolInput) + len(b.ToolResultContent) + 16
		}
	}
	return n / bytesPerToken
}

// GuardReport says what GuardContext changed.
type GuardReport struct {
	DroppedToolResults int
	DroppedMessages    int
}

// GuardContext shrinks msgs to fit window without touching the system prompt, tools, or latest user turn, and errors with context_too_long if that is not enough.
func GuardContext(system string, tools []ToolSpec, msgs []Message, window int) ([]Message, GuardReport, error) {
	var rep GuardReport
	budget := window - outputReserveTokens
	if window <= 0 || estimateTokens(system, tools, msgs) <= budget {
		return msgs, rep, nil
	}
	out := cloneMessages(msgs)
	last := lastUserTurnStart(out)

	for i := 0; i < last && estimateTokens(system, tools, out) > budget; i++ {
		for j := range out[i].Content {
			b := &out[i].Content[j]
			if b.Type == ContentBlockToolResult && b.ToolResultContent != droppedToolOutput {
				b.ToolResultContent = droppedToolOutput
				rep.DroppedToolResults++
			}
		}
	}
	for estimateTokens(system, tools, out) > budget {
		next := nextTurnStart(out, 1)
		if next <= 0 || next > last {
			break
		}
		out = out[next:]
		last -= next
		rep.DroppedMessages += next
	}
	if rep.DroppedMessages > 0 && len(out) > 0 {
		note := ContentBlock{Type: ContentBlockText, Text: fmt.Sprintf("[%d earlier messages were trimmed to fit the model context window]", rep.DroppedMessages)}
		out[0].Content = append([]ContentBlock{note}, out[0].Content...)
	}
	if estimateTokens(system, tools, out) > budget {
		return msgs, rep, &ProviderError{Class: ClassContextTooLong, Detail: "latest turn alone exceeds the context window"}
	}
	return out, rep, nil
}

func cloneMessages(msgs []Message) []Message {
	out := make([]Message, len(msgs))
	for i, m := range msgs {
		out[i] = Message{Role: m.Role, Content: append([]ContentBlock(nil), m.Content...)}
	}
	return out
}

func isUserTextTurn(m Message) bool {
	if m.Role != RoleUser {
		return false
	}
	for _, b := range m.Content {
		if b.Type == ContentBlockText && strings.TrimSpace(b.Text) != "" {
			return true
		}
	}
	return false
}

// lastUserTurnStart is the index of the latest user message that carries text
// (a real operator turn, not a tool_result carrier); everything from there on is protected.
func lastUserTurnStart(msgs []Message) int {
	for i := len(msgs) - 1; i >= 0; i-- {
		if isUserTextTurn(msgs[i]) {
			return i
		}
	}
	return 0
}

// nextTurnStart returns the index of the first user-text message at or after
// from, so dropping msgs[:idx] never splits a tool_use from its tool_result.
func nextTurnStart(msgs []Message, from int) int {
	for i := from; i < len(msgs); i++ {
		if isUserTextTurn(msgs[i]) {
			return i
		}
	}
	return -1
}
