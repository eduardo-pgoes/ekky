package render_test

import (
	"bytes"
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/eduardo-pgoes/ekky/internal/harness"
	"github.com/eduardo-pgoes/ekky/internal/model"
	"github.com/eduardo-pgoes/ekky/internal/render"
)

func TestMain(m *testing.M) {
	harness.MaybeFake()
	os.Exit(m.Run())
}

func tree(t *testing.T) *model.Tree {
	root := harness.NewRoot(t)
	root.EveryState(root.NewRepo("r"))
	tr, err := model.Load(root.Dir)
	if err != nil {
		t.Fatal(err)
	}
	return tr
}

func section(out, name string) string {
	i := strings.Index(out, name+"\n")
	if i < 0 {
		return ""
	}
	rest := out[i+len(name)+1:]
	if j := strings.Index(rest, "\n\n"); j >= 0 {
		rest = rest[:j]
	}
	return rest
}

func TestBacklogGroups(t *testing.T) {
	var b bytes.Buffer
	render.Backlog(&b, tree(t), false)
	out := b.String()
	me, machine, finished := section(out, "me"), section(out, "machine"), section(out, "finished")
	if me == "" || machine == "" || finished == "" {
		t.Fatalf("missing a group:\n%s", out)
	}
	if !strings.HasPrefix(strings.TrimSpace(me), "rfc  task") || !strings.Contains(me, "upstream") || !strings.Contains(me, "result") {
		t.Errorf("header:\n%s", me)
	}
	for _, want := range []string{"00-smoke", "05-e", "06-f", "07-g", "08-h", "01-x", "02-y"} {
		if !strings.Contains(me, want) {
			t.Errorf("me lacks %s:\n%s", want, me)
		}
	}
	for _, want := range []string{"02-b", "03-c", "04-d", "01-typo", "blocked", "running"} {
		if !strings.Contains(machine, want) {
			t.Errorf("machine lacks %s:\n%s", want, machine)
		}
	}
	for _, want := range []string{"01-a", "09-i", "10-j", "accepted", "promoted", "feat/r-09-i"} {
		if !strings.Contains(finished, want) {
			t.Errorf("finished lacks %s:\n%s", want, finished)
		}
	}
	for _, line := range strings.Split(machine, "\n") {
		f := strings.Fields(line)
		switch {
		case len(f) > 0 && f[1] == "03-c":
			if f[3] != "02-b" {
				t.Errorf("03-c upstream column %q", f[3])
			}
		case len(f) > 0 && f[1] == "01-typo":
			if f[3] != "99-nope" || f[2] != "blocked" {
				t.Errorf("typo row %v", f)
			}
		}
	}
	if !strings.Contains(me, "Added a pure shout") {
		t.Errorf("smoke result summary missing:\n%s", me)
	}
	if strings.Contains(out, "ENG-1") {
		t.Error("source column should be dropped from the table")
	}
	if strings.Index(out, "me\n") > strings.Index(out, "machine\n") || strings.Index(out, "machine\n") > strings.Index(out, "finished\n") {
		t.Error("group order")
	}
}

func TestBacklogNeedsMe(t *testing.T) {
	tr := tree(t)
	var b bytes.Buffer
	render.Backlog(&b, tr, true)
	out := b.String()
	if strings.Contains(out, "machine\n") || strings.Contains(out, "finished\n") || !strings.HasPrefix(out, "me\n") {
		t.Errorf("--needs-me printed more than the me group:\n%s", out)
	}
	for _, id := range []string{"02-b", "04-d", "01-a", "09-i"} {
		if strings.Contains(out, id) {
			t.Errorf("--needs-me printed %s", id)
		}
	}
	got := render.Filter(tr, true)
	for _, task := range got {
		if task.Owner != model.OwnerMe {
			t.Errorf("filter kept %s (%s)", task.ID, task.Owner)
		}
	}
	if len(got) != 7 {
		t.Errorf("%d me tasks, want 7", len(got))
	}
	var empty model.Tree
	b.Reset()
	render.Backlog(&b, &empty, false)
	if !strings.Contains(b.String(), "me\n  none") {
		t.Errorf("empty tree:\n%s", b.String())
	}
}

func TestJSONRoundTrips(t *testing.T) {
	tr := tree(t)
	var b bytes.Buffer
	if err := render.JSON(&b, render.Filter(tr, false)); err != nil {
		t.Fatal(err)
	}
	var back []*model.Task
	if err := json.Unmarshal(b.Bytes(), &back); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(back, tr.Tasks) {
		t.Error("json array does not round-trip through the model's types")
	}
	b.Reset()
	if err := render.JSON(&b, render.Filter(tr, true)); err != nil {
		t.Fatal(err)
	}
	back = nil
	_ = json.Unmarshal(b.Bytes(), &back)
	if len(back) != 7 {
		t.Errorf("--needs-me --json: %d tasks", len(back))
	}
	b.Reset()
	if err := render.JSONOne(&b, tr.Get("_/00-smoke")); err != nil {
		t.Fatal(err)
	}
	var one model.Task
	if err := json.Unmarshal(b.Bytes(), &one); err != nil || one.ID != "_/00-smoke" || one.Result == nil {
		t.Errorf("show --json: %v %s", err, b.String())
	}
	b.Reset()
	_ = render.JSON(&b, nil)
	if strings.TrimSpace(b.String()) != "[]" {
		t.Errorf("empty list is %q", b.String())
	}
}

func TestShow(t *testing.T) {
	tr := tree(t)
	var b bytes.Buffer
	render.Show(&b, tr.Get("_/00-smoke"), 2)
	out := b.String()
	for _, want := range []string{
		"_/00-smoke — Add a --shout flag to greet",
		"model:        sonnet",
		"status:       done (me)",
		"branch:       ekky/_-00-smoke",
		"descendants:  none",
		"## Result",
		"done · $0.24 · 34s · 12 turns",
		"decisions:\n- How should the shout",
		"→ A standalone",
		"    rejected: ",
		"deviations: none",
		"verify:\n    From the main checkout: python3",
		"## run.log (last 2 lines)\n\ntool Bash: ls\nekky: _/00-smoke → done\n",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("show lacks %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "question:") {
		t.Error("question printed on a done result")
	}
	b.Reset()
	render.Show(&b, tr.Get("r/01-a"), 40)
	out = b.String()
	if !strings.Contains(out, "descendants:  r/02-b, r/03-c") || !strings.Contains(out, "status:       accepted (finished)") || strings.Contains(out, "## Result") || strings.Contains(out, "run.log") {
		t.Errorf("show 01-a:\n%s", out)
	}
	b.Reset()
	render.Show(&b, tr.Get("r/09-i"), 40)
	if !strings.Contains(b.String(), "pr:           https://github.com/x/y/pull/9") || !strings.Contains(b.String(), "branch:       feat/r-09-i") {
		t.Errorf("show 09-i:\n%s", b.String())
	}
}

func TestEnvelopeMatchesScript(t *testing.T) {
	cases := []struct {
		r    model.Result
		want string
	}{
		{model.Result{Status: "done", CostUSD: 0.1530964, DurationMS: 35649, Turns: 14}, "done · $0.15 · 36s · 14 turns"},
		{model.Result{Status: "failed", CostUSD: 1.005, DurationMS: 153000, Turns: 3}, "failed · $1 · 3m · 3 turns"},
		{model.Result{Status: "done", CostUSD: 2, DurationMS: 90000, Turns: 0}, "done · $2 · 2m · 0 turns"},
		{model.Result{Status: "done", CostUSD: 1.015, DurationMS: 1000, Turns: 1}, "done · $1.01 · 1s · 1 turns"},
		{model.Result{Status: "done", CostUSD: 0.5, DurationMS: 59499, Turns: 1}, "done · $0.5 · 59s · 1 turns"},
	}
	for _, c := range cases {
		if got := render.Envelope(&c.r); got != c.want {
			t.Errorf("%+v → %q, want %q", c.r, got, c.want)
		}
	}
}
