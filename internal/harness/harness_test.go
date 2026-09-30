package harness

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/eduardo-pgoes/ekky/internal/model"
)

func TestMain(m *testing.M) {
	MaybeFake()
	os.Exit(m.Run())
}

func keys(m map[string]any) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// The final stream-json event carries the envelope the `json` format produced on the same task, and its
// structured_output parses into the model's Result.
func TestFixtureResultMatchesJSONFormat(t *testing.T) {
	evs, err := FixtureEvents()
	if err != nil {
		t.Fatal(err)
	}
	if evs[0]["type"] != "system" || evs[0]["subtype"] != "init" {
		t.Fatalf("first event is %v/%v, want system/init", evs[0]["type"], evs[0]["subtype"])
	}
	last := evs[len(evs)-1]
	if last["type"] != "result" {
		t.Fatalf("last event is %v, want result", last["type"])
	}
	raw, err := os.ReadFile(filepath.Join("testdata", "json-format.json"))
	if err != nil {
		t.Fatal(err)
	}
	var jsonFormat map[string]any
	if err := json.Unmarshal(raw, &jsonFormat); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"structured_output", "total_cost_usd", "duration_ms", "num_turns", "is_error", "subtype", "result"} {
		if _, ok := last[k]; !ok {
			t.Errorf("stream-json result event lacks %q", k)
		}
		if _, ok := jsonFormat[k]; !ok {
			t.Errorf("json-format result lacks %q", k)
		}
	}
	so := last["structured_output"].(map[string]any)
	if got, want := keys(so), keys(jsonFormat["structured_output"].(map[string]any)); !reflect.DeepEqual(got, want) {
		t.Errorf("structured_output keys differ: stream %v, json %v", got, want)
	}
	var fromString map[string]any
	if err := json.Unmarshal([]byte(last["result"].(string)), &fromString); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(fromString, so) {
		t.Error("result string and structured_output differ")
	}

	dir := t.TempDir()
	line := bytes.Split(bytes.TrimSpace(Fixture), []byte("\n"))
	if err := os.WriteFile(filepath.Join(dir, "result.json"), line[len(line)-1], 0o644); err != nil {
		t.Fatal(err)
	}
	r, err := model.ReadResult(filepath.Join(dir, "result.json"))
	if err != nil {
		t.Fatal(err)
	}
	if r.Status != "done" || r.Summary == "" || len(r.Decisions) != 1 || r.Turns != 12 || r.CostUSD == 0 || r.DurationMS == 0 || r.IsError || r.Subtype != "success" {
		t.Errorf("parsed result: %+v", r)
	}
}

func TestFixtureRequiresVerbose(t *testing.T) {
	code := FakeClaude(strings.NewReader(""), &bytes.Buffer{}, &bytes.Buffer{}, []string{"-p", "--output-format", "stream-json"})
	if code != 1 {
		t.Errorf("fake accepted stream-json without --verbose")
	}
}

// The fake, run as a subprocess the way the dispatcher will run it, replays the fixture's shape.
func TestFakeReplaysFixtureShape(t *testing.T) {
	root := NewRoot(t)
	repo := root.NewRepo("r")
	taskDir := root.Task("_/01-fake", TaskSpec{Repo: repo, Base: "main", Model: "sonnet", Account: "fake",
		Fake: []string{"text starting", "commit hello.txt hi", "emit done all good"}})
	wt := filepath.Join(taskDir, "wt")
	Git(t, repo.Dir, "worktree", "add", "-q", "--no-track", "-B", "ekky/_-01-fake", wt, "origin/main")

	record, _ := os.ReadFile(filepath.Join(taskDir, "task.md"))
	cmd := exec.Command(os.Args[0], "-p", "--add-dir", taskDir, "--model", "sonnet", "--output-format", "stream-json", "--verbose", "--json-schema", "{}")
	cmd.Dir = wt
	cmd.Stdin = bytes.NewReader(record)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("fake failed: %v\n%s", err, stderr.String())
	}
	var events []map[string]any
	for _, l := range bytes.Split(bytes.TrimSpace(out), []byte("\n")) {
		var m map[string]any
		if err := json.Unmarshal(l, &m); err != nil {
			t.Fatalf("bad event %q: %v", l, err)
		}
		events = append(events, m)
	}
	if events[0]["type"] != "system" || events[0]["subtype"] != "init" || events[0]["cwd"] != wt {
		t.Errorf("init event: %v", events[0])
	}
	var tools []string
	for _, ev := range events {
		if ev["type"] != "assistant" {
			continue
		}
		for _, c := range ev["message"].(map[string]any)["content"].([]any) {
			b := c.(map[string]any)
			if b["type"] == "tool_use" {
				tools = append(tools, b["name"].(string))
			}
		}
	}
	if want := []string{"Write", "Bash"}; !reflect.DeepEqual(tools, want) {
		t.Errorf("tool calls %v, want %v", tools, want)
	}
	last := events[len(events)-1]
	so := last["structured_output"].(map[string]any)
	if last["type"] != "result" || so["status"] != "done" || so["summary"] != "all good" || last["is_error"] != false {
		t.Errorf("result event: %v", last)
	}
	fixture, _ := FixtureEvents()
	if got, want := keys(so), keys(fixture[len(fixture)-1]["structured_output"].(map[string]any)); !reflect.DeepEqual(got, want) {
		t.Errorf("fake structured_output keys %v, fixture %v", got, want)
	}
	if got := Git(t, wt, "log", "--format=%s", "-1"); strings.TrimSpace(got) != "feat: add hello.txt" {
		t.Errorf("fake did not commit: %q", got)
	}
	if b, _ := os.ReadFile(filepath.Join(wt, "hello.txt")); string(b) != "hi\n" {
		t.Errorf("hello.txt = %q", b)
	}
}
