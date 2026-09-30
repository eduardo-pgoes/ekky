package attempt

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// event is the slice of a stream-json line the log renderer looks at.
type event struct {
	Type    string `json:"type"`
	Subtype string `json:"subtype"`
	Model   string `json:"model"`
	Version string `json:"claude_code_version"`
	Session string `json:"session_id"`
	Kind    string `json:"kind"`
	Branch  string `json:"branch"`
	Message struct {
		Content []struct {
			Type  string          `json:"type"`
			Text  string          `json:"text"`
			Name  string          `json:"name"`
			Input json.RawMessage `json:"input"`
		} `json:"content"`
	} `json:"message"`
	IsError    *bool           `json:"is_error"`
	NumTurns   *int            `json:"num_turns"`
	CostUSD    *float64        `json:"total_cost_usd"`
	Result     json.RawMessage `json:"result"`
	Structured json.RawMessage `json:"structured_output"`
}

const lineWidth = 200

func oneLine(s string, width int) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > width {
		r := []rune(s)
		if len(r) > width {
			s = string(r[:width]) + "…"
		}
	}
	return s
}

// renderLines is what one stream-json event contributes to run.log: assistant text, each tool call as its
// name and a one-line summary of its input, and the result envelope. Tool results and bookkeeping events
// contribute nothing.
func renderLines(raw []byte, now time.Time) []string {
	var ev event
	if err := json.Unmarshal(raw, &ev); err != nil {
		return nil
	}
	stamp := now.Format("15:04:05")
	var out []string
	switch ev.Type {
	case "system":
		switch ev.Subtype {
		case "init":
			out = append(out, fmt.Sprintf("%s  init: model %s · claude %s · session %s", stamp, ev.Model, ev.Version, ev.Session))
		case "vcs_state_changed":
			out = append(out, fmt.Sprintf("%s  git: %s on %s", stamp, ev.Kind, ev.Branch))
		}
	case "assistant":
		for _, c := range ev.Message.Content {
			switch c.Type {
			case "text":
				if t := oneLine(c.Text, lineWidth); t != "" {
					out = append(out, fmt.Sprintf("%s  text: %s", stamp, t))
				}
			case "tool_use":
				out = append(out, fmt.Sprintf("%s  %s: %s", stamp, c.Name, toolSummary(c.Name, c.Input)))
			}
		}
	case "result":
		var parts []string
		parts = append(parts, ev.Subtype)
		if ev.CostUSD != nil {
			parts = append(parts, fmt.Sprintf("$%.2f", *ev.CostUSD))
		}
		if ev.NumTurns != nil {
			parts = append(parts, fmt.Sprintf("%d turns", *ev.NumTurns))
		}
		if ev.IsError != nil && *ev.IsError {
			parts = append(parts, "is_error")
		}
		out = append(out, fmt.Sprintf("%s  result: %s", stamp, strings.Join(parts, " · ")))
	}
	return out
}

func toolSummary(name string, input json.RawMessage) string {
	var m map[string]any
	if err := json.Unmarshal(input, &m); err != nil {
		return oneLine(string(input), lineWidth)
	}
	str := func(k string) (string, bool) {
		v, ok := m[k].(string)
		return v, ok && v != ""
	}
	switch name {
	case "Bash":
		if v, ok := str("command"); ok {
			return oneLine(v, lineWidth)
		}
	case "Read", "Write", "Edit", "MultiEdit", "NotebookEdit":
		if v, ok := str("file_path"); ok {
			return v
		}
		if v, ok := str("notebook_path"); ok {
			return v
		}
	case "Glob", "Grep":
		p, _ := str("pattern")
		if d, ok := str("path"); ok {
			return oneLine(p+" in "+d, lineWidth)
		}
		return oneLine(p, lineWidth)
	case "StructuredOutput":
		if v, ok := str("status"); ok {
			s := "status=" + v
			if sum, ok := str("summary"); ok {
				s += " · " + oneLine(sum, lineWidth-len(s)-3)
			}
			return s
		}
	case "Agent", "Task":
		if v, ok := str("description"); ok {
			return oneLine(v, lineWidth)
		}
	case "WebFetch":
		if v, ok := str("url"); ok {
			return v
		}
	case "WebSearch":
		if v, ok := str("query"); ok {
			return oneLine(v, lineWidth)
		}
	}
	b, _ := json.Marshal(m)
	return oneLine(string(b), lineWidth)
}
