// Package model reads agent-tasks/ into typed tasks with their derived states.
// It never writes a file and never spawns a process; every verb is a function over what it returns.
package model

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Status is a status word: the five written to disk, `new` for an absent file, and the three derived ones.
type Status string

const (
	New        Status = "new"
	Ready      Status = "ready"
	Running    Status = "running"
	Done       Status = "done"
	Failed     Status = "failed"
	NeedsInput Status = "needs-input"

	Accepted Status = "accepted" // done + the `accepted` marker
	Blocked  Status = "blocked"  // ready + an upstream that is not done
	Promoted Status = "promoted" // the batch's promote.md has a `branch <slug>:` line
)

// Writable reports whether s is one of the five words the dispatcher writes to `status`.
func Writable(s Status) bool {
	switch s {
	case Ready, Running, Done, Failed, NeedsInput:
		return true
	}
	return false
}

// Owner is who a task is waiting on.
type Owner string

const (
	OwnerMe       Owner = "me"
	OwnerMachine  Owner = "machine"
	OwnerFinished Owner = "finished"
)

// Record is the flat frontmatter of task.md.
type Record struct {
	Repo    string `json:"repo"`
	Base    string `json:"base"`
	Model   string `json:"model"`
	Account string `json:"account"`
	After   string `json:"after"`
	Source  string `json:"source"`
}

// Decision is one fork the agent took.
type Decision struct {
	Fork     string `json:"fork"`
	Chosen   string `json:"chosen"`
	Rejected string `json:"rejected"`
	Why      string `json:"why"`
}

// Result is the parsed result.json: the structured output plus the envelope fields the card shows.
type Result struct {
	Status     string     `json:"status"`
	Summary    string     `json:"summary"`
	Decisions  []Decision `json:"decisions"`
	Deviations string     `json:"deviations"`
	Verify     string     `json:"verify"`
	Question   string     `json:"question"`
	CostUSD    float64    `json:"cost_usd"`
	DurationMS int64      `json:"duration_ms"`
	Turns      int        `json:"turns"`
	IsError    bool       `json:"is_error"`
	Subtype    string     `json:"subtype"`
}

// Task is one task directory with everything derived from it. Its JSON is the shape a UI reads.
type Task struct {
	ID    string `json:"id"`
	RFC   string `json:"rfc"`
	Slug  string `json:"task"`
	Dir   string `json:"dir"`
	Title string `json:"title"`
	Record
	Raw         Status   `json:"status"`
	Shown       Status   `json:"shown"`
	Owner       Owner    `json:"owner"`
	Accepted    bool     `json:"accepted"`
	Live        bool     `json:"live"`
	Upstream    string   `json:"upstream"`
	Descendants []string `json:"descendants"`
	Branch      string   `json:"branch"`
	PR          string   `json:"pr"`
	Result      *Result  `json:"result"`
}

// Tree is one reading of agent-tasks/.
type Tree struct {
	Root  string
	Tasks []*Task
	byID  map[string]*Task
}

// TasksDir is where the tasks live under an EKKY_ROOT.
func TasksDir(root string) string { return filepath.Join(root, "agent-tasks") }

// ParseID splits <rfc>/<nn-slug>.
func ParseID(id string) (rfc, slug string, err error) {
	i := strings.Index(id, "/")
	if i <= 0 || i == len(id)-1 || strings.Contains(id[i+1:], "/") {
		return "", "", fmt.Errorf("task id must be <rfc>/<nn-slug>: %s", id)
	}
	return id[:i], id[i+1:], nil
}

// TaskDir is the directory of a task id under root; it does not check that it exists.
func TaskDir(root, id string) (string, error) {
	rfc, slug, err := ParseID(id)
	if err != nil {
		return "", err
	}
	return filepath.Join(TasksDir(root), rfc, "tasks", slug), nil
}

// BranchOf is the local branch the agent commits to before promote renames it.
func BranchOf(id string) string { return "ekky/" + strings.Replace(id, "/", "-", 1) }

// ConfigDirOf is the CLAUDE_CONFIG_DIR that bills an attempt for an `account:` value.
func ConfigDirOf(account string) string {
	home, _ := os.UserHomeDir()
	switch account {
	case "":
		if d := os.Getenv("CLAUDE_CONFIG_DIR"); d != "" {
			return d
		}
		return filepath.Join(home, ".claude")
	case "perso":
		return filepath.Join(home, ".claude")
	default:
		return filepath.Join(home, ".claude-"+account)
	}
}

// StatusOf is the raw word in a task dir's status file, `new` when absent.
func StatusOf(dir string) Status {
	b, err := os.ReadFile(filepath.Join(dir, "status"))
	if err != nil {
		return New
	}
	return Status(strings.TrimSpace(string(b)))
}

// ReadRecord parses task.md: the flat frontmatter (first line ---, up to the next ---, first match of a key wins)
// and the title (the first `# ` line).
func ReadRecord(path string) (rec Record, title string, err error) {
	f, err := os.Open(path)
	if err != nil {
		return rec, "", err
	}
	defer f.Close()
	fields := map[string]string{}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	inFront, first := false, true
	for sc.Scan() {
		line := sc.Text()
		if first {
			first = false
			inFront = line == "---"
			continue
		}
		if inFront {
			if line == "---" {
				inFront = false
				continue
			}
			if k, v, ok := strings.Cut(line, ": "); ok {
				if _, seen := fields[k]; !seen {
					fields[k] = v
				}
			}
			continue
		}
		if title == "" && strings.HasPrefix(line, "# ") {
			title = strings.TrimPrefix(line, "# ")
		}
	}
	rec = Record{
		Repo: fields["repo"], Base: fields["base"], Model: fields["model"],
		Account: fields["account"], After: fields["after"], Source: fields["source"],
	}
	return rec, title, sc.Err()
}

// ReadResult parses a result.json in the `json` output format's shape.
func ReadResult(path string) (*Result, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var env struct {
		StructuredOutput *struct {
			Status     string     `json:"status"`
			Summary    string     `json:"summary"`
			Decisions  []Decision `json:"decisions"`
			Deviations string     `json:"deviations"`
			Verify     string     `json:"verify"`
			Question   string     `json:"question"`
		} `json:"structured_output"`
		CostUSD    float64 `json:"total_cost_usd"`
		DurationMS int64   `json:"duration_ms"`
		Turns      int     `json:"num_turns"`
		IsError    bool    `json:"is_error"`
		Subtype    string  `json:"subtype"`
	}
	if err := json.Unmarshal(b, &env); err != nil {
		return nil, err
	}
	r := &Result{CostUSD: env.CostUSD, DurationMS: env.DurationMS, Turns: env.Turns, IsError: env.IsError, Subtype: env.Subtype}
	if so := env.StructuredOutput; so != nil {
		r.Status, r.Summary, r.Decisions = so.Status, so.Summary, so.Decisions
		r.Deviations, r.Verify, r.Question = so.Deviations, so.Verify, so.Question
	}
	if r.Decisions == nil {
		r.Decisions = []Decision{}
	}
	return r, nil
}

var ledgerLine = regexp.MustCompile(`^(branch|pr) (\S+): (.*)$`)

type ledger struct {
	branch map[string]string
	pr     map[string]string
}

func readLedger(path string) ledger {
	l := ledger{branch: map[string]string{}, pr: map[string]string{}}
	b, err := os.ReadFile(path)
	if err != nil {
		return l
	}
	for _, line := range strings.Split(string(b), "\n") {
		m := ledgerLine.FindStringSubmatch(strings.TrimRight(line, "\r"))
		if m == nil {
			continue
		}
		v := strings.TrimSpace(m[3])
		if i := strings.Index(v, "  #"); i >= 0 {
			v = strings.TrimSpace(v[:i])
		}
		if m[1] == "branch" {
			l.branch[m[2]] = v
		} else {
			l.pr[m[2]] = v
		}
	}
	return l
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// Load reads every task under root's agent-tasks/ and derives their states. A missing agent-tasks/ is an empty tree.
func Load(root string) (*Tree, error) {
	t := &Tree{Root: root, byID: map[string]*Task{}}
	tasks := TasksDir(root)
	rfcs, err := os.ReadDir(tasks)
	if errors.Is(err, os.ErrNotExist) {
		return t, nil
	}
	if err != nil {
		return nil, err
	}
	for _, rfc := range rfcs {
		if !rfc.IsDir() {
			continue
		}
		led := readLedger(filepath.Join(tasks, rfc.Name(), "promote.md"))
		dirs, err := os.ReadDir(filepath.Join(tasks, rfc.Name(), "tasks"))
		if err != nil {
			continue
		}
		for _, d := range dirs {
			if !d.IsDir() || d.Name() == "" || d.Name()[0] < '0' || d.Name()[0] > '9' {
				continue
			}
			task := readTask(rfc.Name(), d.Name(), filepath.Join(tasks, rfc.Name(), "tasks", d.Name()), led)
			t.Tasks = append(t.Tasks, task)
			t.byID[task.ID] = task
		}
	}
	sort.Slice(t.Tasks, func(i, j int) bool { return t.Tasks[i].ID < t.Tasks[j].ID })
	for _, task := range t.Tasks {
		t.derive(task)
	}
	return t, nil
}

func readTask(rfc, slug, dir string, led ledger) *Task {
	task := &Task{ID: rfc + "/" + slug, RFC: rfc, Slug: slug, Dir: dir, Descendants: []string{}}
	if rec, title, err := ReadRecord(filepath.Join(dir, "task.md")); err == nil {
		task.Record, task.Title = rec, title
	}
	task.Raw = StatusOf(dir)
	task.Accepted = exists(filepath.Join(dir, "accepted"))
	task.Live = exists(filepath.Join(dir, "pgid"))
	if task.After != "" {
		task.Upstream = rfc + "/" + task.After
	}
	task.Branch = BranchOf(task.ID)
	if b, ok := led.branch[slug]; ok {
		task.Branch = b
		task.Shown = Promoted
	}
	task.PR = led.pr[slug]
	if r, err := ReadResult(filepath.Join(dir, "result.json")); err == nil {
		task.Result = r
	}
	return task
}

// derive fills Shown, Owner and Descendants once every task of the tree is known.
func (t *Tree) derive(task *Task) {
	if task.Shown != Promoted {
		task.Shown = task.Raw
		switch task.Raw {
		case Done:
			if task.Accepted {
				task.Shown = Accepted
			}
		case Ready:
			if t.blocked(task) {
				task.Shown = Blocked
			}
		}
	}
	switch task.Shown {
	case Ready, Blocked, Running:
		task.Owner = OwnerMachine
	case Accepted, Promoted:
		task.Owner = OwnerFinished
	default:
		task.Owner = OwnerMe
	}
	task.Descendants = t.descendants(task, map[string]bool{}, 0)
}

// blocked: has an upstream and it is not done. A typo'd upstream never becomes done, so it blocks; the table shows it.
func (t *Tree) blocked(task *Task) bool {
	if task.Upstream == "" {
		return false
	}
	up := t.byID[task.Upstream]
	return up == nil || up.Raw != Done
}

const cycleDepth = 64

// descendants: every task downstream of task, parents before children. A cycle stops at cycleDepth
// instead of failing the load; tackle refuses a task that appears among its own descendants.
func (t *Tree) descendants(task *Task, seen map[string]bool, depth int) []string {
	out := []string{}
	if depth >= cycleDepth {
		return out
	}
	for _, c := range t.Tasks {
		if c.RFC != task.RFC || c.After != task.Slug || seen[c.ID] {
			continue
		}
		seen[c.ID] = true
		out = append(out, c.ID)
		out = append(out, t.descendants(c, seen, depth+1)...)
	}
	return out
}

// Get is the task with that id, or nil.
func (t *Tree) Get(id string) *Task { return t.byID[id] }

// InRFC is every task of one rfc, in id order.
func (t *Tree) InRFC(rfc string) []*Task {
	var out []*Task
	for _, task := range t.Tasks {
		if task.RFC == rfc {
			out = append(out, task)
		}
	}
	return out
}

// Cyclic reports whether task is among its own descendants.
func (t *Tree) Cyclic(task *Task) bool {
	for _, d := range task.Descendants {
		if d == task.ID {
			return true
		}
	}
	return false
}

// Chain is the task followed by its descendants, as tasks, in cascade order.
func (t *Tree) Chain(task *Task) []*Task {
	out := []*Task{task}
	for _, id := range task.Descendants {
		if c := t.byID[id]; c != nil {
			out = append(out, c)
		}
	}
	return out
}
