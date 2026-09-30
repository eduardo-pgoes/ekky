// Package harness is the scenario harness: a fake `claude` that replays the captured stream-json shape,
// and a temp EKKY_ROOT with scratch repositories for tests. Nothing here touches the real agent-tasks/.
package harness

import (
	"bufio"
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// Fixture is one real `claude -p --output-format stream-json --verbose` transcript, captured on 2.1.251
// from the _/00-smoke record. The fake takes its event shapes from here.
//
//go:embed testdata/stream.jsonl
var Fixture []byte

// FixtureEvents is the fixture parsed, one map per line.
func FixtureEvents() ([]map[string]any, error) {
	var out []map[string]any
	for _, line := range bytes.Split(Fixture, []byte("\n")) {
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		var m map[string]any
		if err := json.Unmarshal(line, &m); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, nil
}

// FakeEnv is the variable that turns a test binary into the fake claude.
const FakeEnv = "EKKY_FAKE_CLAUDE"

// MaybeFake runs the fake claude and exits when FakeEnv is set. Call it first in TestMain, and point
// EKKY_CLAUDE at os.Args[0].
func MaybeFake() {
	if os.Getenv(FakeEnv) != "1" {
		return
	}
	os.Exit(FakeClaude(os.Stdin, os.Stdout, os.Stderr, os.Args[1:]))
}

type templates struct {
	init, toolUse, toolResult, text, result []byte
}

func loadTemplates() (templates, error) {
	var t templates
	for _, line := range bytes.Split(Fixture, []byte("\n")) {
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		var ev struct {
			Type    string `json:"type"`
			Subtype string `json:"subtype"`
			Message struct {
				Content []struct {
					Type string `json:"type"`
				} `json:"content"`
			} `json:"message"`
		}
		if err := json.Unmarshal(line, &ev); err != nil {
			return t, err
		}
		switch {
		case ev.Type == "system" && ev.Subtype == "init" && t.init == nil:
			t.init = line
		case ev.Type == "assistant" && len(ev.Message.Content) == 1 && ev.Message.Content[0].Type == "tool_use" && t.toolUse == nil:
			t.toolUse = line
		case ev.Type == "assistant" && len(ev.Message.Content) == 1 && ev.Message.Content[0].Type == "text" && t.text == nil:
			t.text = line
		case ev.Type == "user" && t.toolResult == nil:
			t.toolResult = line
		case ev.Type == "result":
			t.result = line
		}
	}
	if t.init == nil || t.toolUse == nil || t.toolResult == nil || t.text == nil || t.result == nil {
		return t, fmt.Errorf("fixture lacks a template event")
	}
	return t, nil
}

func clone(raw []byte) map[string]any {
	var m map[string]any
	_ = json.Unmarshal(raw, &m)
	return m
}

type fake struct {
	out     *bufio.Writer
	err     io.Writer
	tpl     templates
	taskDir string
	model   string
	turns   int
	started time.Time
	pushed  string
}

func (f *fake) emit(ev map[string]any) {
	b, _ := json.Marshal(ev)
	f.out.Write(b)
	f.out.WriteByte('\n')
	f.out.Flush()
}

func (f *fake) tool(name string, input map[string]any) {
	f.turns++
	ev := clone(f.tpl.toolUse)
	msg := ev["message"].(map[string]any)
	block := msg["content"].([]any)[0].(map[string]any)
	block["name"], block["input"], block["id"] = name, input, fmt.Sprintf("toolu_fake_%d", f.turns)
	f.emit(ev)
}

func (f *fake) toolResult(content string, isErr bool) {
	ev := clone(f.tpl.toolResult)
	msg := ev["message"].(map[string]any)
	block := msg["content"].([]any)[0].(map[string]any)
	block["content"], block["is_error"], block["tool_use_id"] = content, isErr, fmt.Sprintf("toolu_fake_%d", f.turns)
	delete(ev, "tool_use_result")
	f.emit(ev)
}

func (f *fake) text(s string) {
	ev := clone(f.tpl.text)
	msg := ev["message"].(map[string]any)
	msg["content"].([]any)[0].(map[string]any)["text"] = s
	f.emit(ev)
}

func (f *fake) result(status, summary string) {
	so := map[string]any{
		"status": status, "summary": summary,
		"decisions":  []any{map[string]any{"fork": "fake fork", "chosen": "a", "rejected": "b", "why": "because"}},
		"deviations": "", "verify": "cat hello.txt", "question": "",
	}
	if status == "needs-input" {
		so["question"] = "which one? a or b"
	}
	ev := clone(f.tpl.result)
	ev["structured_output"] = so
	raw, _ := json.Marshal(so)
	ev["result"] = string(raw)
	ev["total_cost_usd"] = 0.0042
	ev["duration_ms"] = time.Since(f.started).Milliseconds()
	ev["num_turns"] = f.turns
	ev["is_error"] = false
	ev["subtype"] = "success"
	f.emit(ev)
}

func (f *fake) errorResult() {
	ev := clone(f.tpl.result)
	delete(ev, "structured_output")
	ev["result"] = "fake: error during execution"
	ev["is_error"] = true
	ev["subtype"] = "error_during_execution"
	ev["num_turns"] = f.turns
	f.emit(ev)
}

func (f *fake) sh(command string) (string, error) {
	f.tool("Bash", map[string]any{"command": command})
	cmd := exec.Command("sh", "-c", command)
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=ekky fake", "GIT_AUTHOR_EMAIL=fake@ekky.invalid",
		"GIT_COMMITTER_NAME=ekky fake", "GIT_COMMITTER_EMAIL=fake@ekky.invalid")
	b, err := cmd.CombinedOutput()
	f.toolResult(strings.TrimSpace(string(b)), err != nil)
	return string(b), err
}

func fakeSection(record string) []string {
	var lines []string
	in := false
	for _, line := range strings.Split(record, "\n") {
		if strings.HasPrefix(line, "## ") {
			in = line == "## Fake"
			continue
		}
		if !in {
			continue
		}
		line = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "- "))
		if line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}

// FakeClaude is the fake CLI. It reads the record on stdin, does what its `## Fake` section says, and writes
// stream-json events in the fixture's shape. Actions, one per line:
//
//	commit <file> [content]   write the file, git add, git commit
//	sleep <seconds>
//	push                      run git push origin HEAD (the fences should refuse it)
//	text <words>              an assistant text event
//	hang                      spawn a child `sleep`, write its pid to <taskdir>/fake-child.pid, block forever
//	exit <code>               exit with that code and no result
//	error                     a result event with is_error and no structured_output, exit 1
//	emit done|failed|needs-input [summary]
//
// Without an emit, exit or error line the fake ends with `emit done`.
func FakeClaude(stdin io.Reader, stdout, stderr io.Writer, args []string) int {
	f := &fake{out: bufio.NewWriter(stdout), err: stderr, started: time.Now()}
	var format string
	verbose := false
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--add-dir":
			i++
			f.taskDir = args[i]
		case "--model":
			i++
			f.model = args[i]
		case "--output-format":
			i++
			format = args[i]
		case "--verbose":
			verbose = true
		case "--append-system-prompt-file", "--max-budget-usd", "--json-schema", "--setting-sources":
			i++
		}
	}
	if format == "stream-json" && !verbose {
		fmt.Fprintln(stderr, "Error: When using --print, --output-format=stream-json requires --verbose")
		return 1
	}
	if format != "stream-json" {
		fmt.Fprintf(stderr, "fake claude: unexpected --output-format %q\n", format)
		return 1
	}
	var tpl templates
	var err error
	if tpl, err = loadTemplates(); err != nil {
		fmt.Fprintln(stderr, "fake claude:", err)
		return 1
	}
	f.tpl = tpl
	rec, err := io.ReadAll(stdin)
	if err != nil {
		fmt.Fprintln(stderr, "fake claude:", err)
		return 1
	}
	init := clone(tpl.init)
	if f.model != "" {
		init["model"] = f.model
	}
	if cwd, err := os.Getwd(); err == nil {
		init["cwd"] = cwd
	}
	f.emit(init)

	for _, action := range fakeSection(string(rec)) {
		verb, rest, _ := strings.Cut(action, " ")
		switch verb {
		case "commit":
			file, content, hasContent := strings.Cut(rest, " ")
			if !hasContent {
				content = "fake"
			}
			f.tool("Write", map[string]any{"file_path": file, "content": content + "\n"})
			if err := os.WriteFile(file, []byte(content+"\n"), 0o644); err != nil {
				f.toolResult(err.Error(), true)
				return 1
			}
			f.toolResult("File written", false)
			if _, err := f.sh(fmt.Sprintf("git add %s && git commit -q -m 'feat: add %s'", file, file)); err != nil {
				fmt.Fprintln(stderr, "fake claude: commit failed:", err)
				return 1
			}
		case "sleep":
			secs, _ := strconv.ParseFloat(rest, 64)
			f.tool("Bash", map[string]any{"command": "sleep " + rest})
			time.Sleep(time.Duration(secs * float64(time.Second)))
			f.toolResult("", false)
		case "push":
			out, err := f.sh("git push origin HEAD")
			if err != nil {
				f.pushed = "push refused: " + strings.TrimSpace(out)
			} else {
				f.pushed = "pushed"
			}
		case "text":
			f.text(rest)
		case "hang":
			f.tool("Bash", map[string]any{"command": "sleep 3600"})
			child := exec.Command("sleep", "3600")
			if err := child.Start(); err != nil {
				fmt.Fprintln(stderr, "fake claude:", err)
				return 1
			}
			if f.taskDir != "" {
				_ = os.WriteFile(filepath.Join(f.taskDir, "fake-child.pid"), []byte(strconv.Itoa(child.Process.Pid)), 0o644)
			}
			_ = child.Wait()
			return 1
		case "exit":
			code, _ := strconv.Atoi(rest)
			f.out.Flush()
			return code
		case "error":
			f.errorResult()
			return 1
		case "emit":
			status, summary, _ := strings.Cut(rest, " ")
			if summary == "" {
				summary = "fake run ended " + status
			}
			if f.pushed != "" {
				summary += " (" + f.pushed + ")"
			}
			f.result(status, summary)
			return 0
		default:
			fmt.Fprintf(stderr, "fake claude: unknown action %q\n", action)
			return 1
		}
	}
	summary := "fake run ended done"
	if f.pushed != "" {
		summary += " (" + f.pushed + ")"
	}
	f.result("done", summary)
	return 0
}

// Killable is the process group id of the current process, for tests that check the group dies.
func Killable() int { return syscall.Getpgrp() }
