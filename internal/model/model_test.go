package model_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/eduardo-pgoes/ekky/internal/harness"
	"github.com/eduardo-pgoes/ekky/internal/model"
)

func TestMain(m *testing.M) {
	harness.MaybeFake()
	os.Exit(m.Run())
}

func buildTree(t *testing.T) (*harness.Root, *model.Tree) {
	root := harness.NewRoot(t)
	root.EveryState(root.NewRepo("r"))
	tree, err := model.Load(root.Dir)
	if err != nil {
		t.Fatal(err)
	}
	return root, tree
}

func TestDerivedStates(t *testing.T) {
	_, tree := buildTree(t)
	want := map[string][3]string{
		"r/01-a":     {"done", "accepted", "finished"},
		"r/02-b":     {"ready", "ready", "machine"},
		"r/03-c":     {"ready", "blocked", "machine"},
		"r/04-d":     {"running", "running", "machine"},
		"r/05-e":     {"failed", "failed", "me"},
		"r/06-f":     {"needs-input", "needs-input", "me"},
		"r/07-g":     {"new", "new", "me"},
		"r/08-h":     {"done", "done", "me"},
		"r/09-i":     {"done", "promoted", "finished"},
		"r/10-j":     {"done", "promoted", "finished"},
		"_/00-smoke": {"done", "done", "me"},
		"t/01-typo":  {"ready", "blocked", "machine"},
		"c/01-x":     {"new", "new", "me"},
		"c/02-y":     {"new", "new", "me"},
	}
	if len(tree.Tasks) != len(want) {
		t.Fatalf("%d tasks, want %d", len(tree.Tasks), len(want))
	}
	for id, w := range want {
		task := tree.Get(id)
		if task == nil {
			t.Errorf("%s missing", id)
			continue
		}
		if got := [3]string{string(task.Raw), string(task.Shown), string(task.Owner)}; got != w {
			t.Errorf("%s: raw/shown/owner %v, want %v", id, got, w)
		}
	}
	ids := make([]string, len(tree.Tasks))
	for i, task := range tree.Tasks {
		ids[i] = task.ID
	}
	if ids[0] != "_/00-smoke" || ids[1] != "c/01-x" || ids[len(ids)-1] != "t/01-typo" {
		t.Errorf("order: %v", ids)
	}
}

func TestBranchesUpstreamsAndDescendants(t *testing.T) {
	_, tree := buildTree(t)
	a, b, c := tree.Get("r/01-a"), tree.Get("r/02-b"), tree.Get("r/03-c")
	if a.Branch != "ekky/r-01-a" || a.PR != "" {
		t.Errorf("01-a branch %q pr %q", a.Branch, a.PR)
	}
	if i := tree.Get("r/09-i"); i.Branch != "feat/r-09-i" || i.PR != "https://github.com/x/y/pull/9" {
		t.Errorf("09-i branch %q pr %q", i.Branch, i.PR)
	}
	if j := tree.Get("r/10-j"); j.Branch != "feat/r-10-j" || j.PR != "" {
		t.Errorf("10-j branch %q pr %q", j.Branch, j.PR)
	}
	if a.Upstream != "" || b.Upstream != "r/01-a" || c.Upstream != "r/02-b" {
		t.Errorf("upstreams: %q %q %q", a.Upstream, b.Upstream, c.Upstream)
	}
	if !reflect.DeepEqual(a.Descendants, []string{"r/02-b", "r/03-c"}) || !reflect.DeepEqual(b.Descendants, []string{"r/03-c"}) || len(c.Descendants) != 0 {
		t.Errorf("descendants: %v %v %v", a.Descendants, b.Descendants, c.Descendants)
	}
	if chain := tree.Chain(a); len(chain) != 3 || chain[2].ID != "r/03-c" {
		t.Errorf("chain: %v", chain)
	}
	if typo := tree.Get("t/01-typo"); typo.Upstream != "t/99-nope" {
		t.Errorf("typo upstream %q", typo.Upstream)
	}
	x := tree.Get("c/01-x")
	if !tree.Cyclic(x) || tree.Cyclic(a) {
		t.Errorf("cycle detection: x %v a %v", tree.Cyclic(x), tree.Cyclic(a))
	}
	if !tree.Get("r/04-d").Live || tree.Get("r/05-e").Live {
		t.Error("live flag")
	}
	if len(tree.InRFC("r")) != 10 {
		t.Errorf("InRFC(r) = %d", len(tree.InRFC("r")))
	}
}

func TestRecordAndResult(t *testing.T) {
	_, tree := buildTree(t)
	s := tree.Get("_/00-smoke")
	if s.Title != "Add a --shout flag to greet" || s.Model != "sonnet" || s.Source != "hand-written smoke fixture" || s.Account != "" {
		t.Errorf("record: %+v title %q", s.Record, s.Title)
	}
	if s.Result == nil || s.Result.Status != "done" || s.Result.Turns != 12 || len(s.Result.Decisions) != 1 || s.Result.Decisions[0].Fork == "" {
		t.Errorf("result: %+v", s.Result)
	}
	if tree.Get("r/01-a").Result != nil {
		t.Error("result without result.json")
	}
	b, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	var back model.Task
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(&back, s) {
		t.Errorf("json round trip:\n%s", b)
	}
	var m map[string]any
	_ = json.Unmarshal(b, &m)
	for _, k := range []string{"id", "rfc", "task", "title", "repo", "base", "model", "account", "after", "source", "status", "shown", "owner", "accepted", "live", "upstream", "descendants", "branch", "pr", "result"} {
		if _, ok := m[k]; !ok {
			t.Errorf("json lacks %q", k)
		}
	}
}

func TestFrontmatterRules(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "task.md")
	os.WriteFile(path, []byte("---\nrepo: /a\nrepo: /b\nbase: main\nmodel:\nafter: 01-x\n---\nintro\n# The title\n\n# not this\n"), 0o644)
	rec, title, err := model.ReadRecord(path)
	if err != nil {
		t.Fatal(err)
	}
	if rec.Repo != "/a" || rec.Base != "main" || rec.Model != "" || rec.After != "01-x" || title != "The title" {
		t.Errorf("%+v %q", rec, title)
	}
	os.WriteFile(path, []byte("repo: /a\n---\n# t\n"), 0o644)
	rec, _, _ = model.ReadRecord(path)
	if rec.Repo != "" {
		t.Error("frontmatter without a leading --- was parsed")
	}
	if _, _, err := model.ParseID("nope"); err == nil {
		t.Error("ParseID accepted an id without a slash")
	}
	if d, _ := model.TaskDir("/r", "x/01-y"); d != "/r/agent-tasks/x/tasks/01-y" {
		t.Error(d)
	}
	if model.BranchOf("x/01-y") != "ekky/x-01-y" {
		t.Error("BranchOf")
	}
	t.Setenv("HOME", "/h")
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	if model.ConfigDirOf("perso") != "/h/.claude" || model.ConfigDirOf("work") != "/h/.claude-work" || model.ConfigDirOf("") != "/h/.claude" {
		t.Error("ConfigDirOf")
	}
	t.Setenv("CLAUDE_CONFIG_DIR", "/c")
	if model.ConfigDirOf("") != "/c" {
		t.Error("ConfigDirOf inherits CLAUDE_CONFIG_DIR")
	}
	if tree, err := model.Load(filepath.Join(dir, "missing")); err != nil || len(tree.Tasks) != 0 {
		t.Error("missing agent-tasks is not an empty tree")
	}
}
