package harness

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// Root is a temp EKKY_ROOT: agent/ copied from the module, an empty agent-tasks/, a HOME with credentials for
// the `perso` and `fake` accounts, and the environment that points the dispatcher at the fake claude.
type Root struct {
	T    *testing.T
	Dir  string
	Home string
}

// ModuleDir is the checkout this test binary was built from.
func ModuleDir() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}

// NewRoot builds the temp root and sets EKKY_ROOT, HOME, EKKY_CLAUDE, EKKY_FAKE_CLAUDE and EKKY_NO_SYSTEMD for the
// rest of the test.
func NewRoot(t *testing.T) *Root {
	t.Helper()
	dir := t.TempDir()
	home := filepath.Join(dir, "home")
	r := &Root{T: t, Dir: filepath.Join(dir, "root"), Home: home}
	must(t, os.MkdirAll(filepath.Join(r.Dir, "agent-tasks"), 0o755))
	must(t, os.MkdirAll(filepath.Join(r.Dir, "agent"), 0o755))
	for _, f := range []string{"execute.md", "result.schema.json"} {
		b, err := os.ReadFile(filepath.Join(ModuleDir(), "agent", f))
		must(t, err)
		must(t, os.WriteFile(filepath.Join(r.Dir, "agent", f), b, 0o644))
	}
	for _, acct := range []string{".claude", ".claude-fake"} {
		must(t, os.MkdirAll(filepath.Join(home, acct), 0o755))
		must(t, os.WriteFile(filepath.Join(home, acct, ".credentials.json"), []byte("{}"), 0o600))
	}
	t.Setenv("HOME", home)
	t.Setenv("EKKY_ROOT", r.Dir)
	t.Setenv("EKKY_CLAUDE", os.Args[0])
	t.Setenv(FakeEnv, "1")
	t.Setenv("EKKY_NO_SYSTEMD", "1")
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	os.Unsetenv("CLAUDE_CONFIG_DIR")
	return r
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

// Git runs git in dir and fails the test on error.
func Git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, err := GitOutput(dir, args...)
	if err != nil {
		t.Fatalf("git -C %s %s: %v\n%s", dir, strings.Join(args, " "), err, out)
	}
	return out
}

// GitOutput runs git in dir and returns its combined output.
func GitOutput(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=ekky harness", "GIT_AUTHOR_EMAIL=harness@ekky.invalid",
		"GIT_COMMITTER_NAME=ekky harness", "GIT_COMMITTER_EMAIL=harness@ekky.invalid")
	b, err := cmd.CombinedOutput()
	return string(b), err
}

// Repo is a scratch checkout with a bare origin it has pushed main to and fetched back.
type Repo struct {
	Dir    string
	Origin string
}

// NewRepo makes a scratch repository named name under the root, with one commit on main, a bare origin,
// and a local origin/main.
func (r *Root) NewRepo(name string) *Repo {
	t := r.T
	t.Helper()
	repo := &Repo{Dir: filepath.Join(r.Dir, "repos", name), Origin: filepath.Join(r.Dir, "repos", name+".git")}
	seed(t, repo.Dir, name)
	Git(t, repo.Dir, "init", "-q", "--bare", repo.Origin)
	Git(t, repo.Dir, "remote", "add", "origin", repo.Origin)
	Git(t, repo.Dir, "push", "-q", "origin", "main")
	Git(t, repo.Dir, "fetch", "-q", "origin", "main")
	return repo
}

// NewLocalRepo makes a scratch repository named name with one commit on main and no remote at all,
// as in a repo that lives only on this machine.
func (r *Root) NewLocalRepo(name string) *Repo {
	t := r.T
	t.Helper()
	repo := &Repo{Dir: filepath.Join(r.Dir, "repos", name)}
	seed(t, repo.Dir, name)
	return repo
}

// seed initialises dir as a repository with one commit on main.
func seed(t *testing.T, dir, name string) {
	t.Helper()
	must(t, os.MkdirAll(dir, 0o755))
	Git(t, dir, "init", "-q", "-b", "main")
	must(t, os.WriteFile(filepath.Join(dir, "README.md"), []byte("# "+name+"\n"), 0o644))
	Git(t, dir, "add", "README.md")
	Git(t, dir, "commit", "-q", "-m", "chore: seed")
}

// DropRemoteRef deletes the local origin/<base>, as in a checkout that was never fetched.
func (repo *Repo) DropRemoteRef(t *testing.T, base string) {
	t.Helper()
	Git(t, repo.Dir, "update-ref", "-d", "refs/remotes/origin/"+base)
}

// BreakOrigin points origin at a path that does not exist, so every fetch and push fails fast.
func (repo *Repo) BreakOrigin(t *testing.T) {
	t.Helper()
	Git(t, repo.Dir, "remote", "set-url", "origin", filepath.Join(repo.Dir, "..", "nowhere.git"))
}

// TaskSpec is what a test writes into a task.md.
type TaskSpec struct {
	Repo    *Repo
	Base    string
	Model   string
	Account string
	After   string
	Source  string
	Title   string
	Fake    []string
	Extra   string
}

// Task writes a task record for id and returns its directory.
func (r *Root) Task(id string, spec TaskSpec) string {
	t := r.T
	t.Helper()
	rfc, slug, ok := strings.Cut(id, "/")
	if !ok {
		t.Fatalf("bad id %q", id)
	}
	dir := filepath.Join(r.Dir, "agent-tasks", rfc, "tasks", slug)
	must(t, os.MkdirAll(dir, 0o755))
	var b strings.Builder
	b.WriteString("---\n")
	if spec.Repo != nil {
		fmt.Fprintf(&b, "repo: %s\n", spec.Repo.Dir)
	}
	for _, kv := range [][2]string{{"base", spec.Base}, {"model", spec.Model}, {"account", spec.Account}, {"after", spec.After}, {"source", spec.Source}} {
		if kv[1] != "" {
			fmt.Fprintf(&b, "%s: %s\n", kv[0], kv[1])
		}
	}
	b.WriteString("---\n")
	title := spec.Title
	if title == "" {
		title = "Task " + slug
	}
	fmt.Fprintf(&b, "# %s\n\n## Done looks like\nthe fake did its thing\n\n## Decisions made\n- none\n\n## Do not\n- push\n", title)
	if len(spec.Fake) > 0 {
		b.WriteString("\n## Fake\n")
		for _, a := range spec.Fake {
			fmt.Fprintf(&b, "- %s\n", a)
		}
	}
	b.WriteString(spec.Extra)
	must(t, os.WriteFile(filepath.Join(dir, "task.md"), []byte(b.String()), 0o644))
	return dir
}

// Simple is a task record whose fake commits one file and returns done.
func (r *Root) Simple(id string, repo *Repo) string {
	return r.Task(id, TaskSpec{Repo: repo, Base: "main", Model: "sonnet", Account: "fake", Fake: []string{"commit " + strings.ReplaceAll(id, "/", "-") + ".txt", "emit done"}})
}

// TaskDir is the directory of id under this root.
func (r *Root) TaskDir(id string) string {
	rfc, slug, _ := strings.Cut(id, "/")
	return filepath.Join(r.Dir, "agent-tasks", rfc, "tasks", slug)
}

// Write puts a file into a task directory.
func (r *Root) Write(id, name, content string) {
	r.T.Helper()
	must(r.T, os.WriteFile(filepath.Join(r.TaskDir(id), name), []byte(content), 0o644))
}

// Read returns a file from a task directory, "" when absent.
func (r *Root) Read(id, name string) string {
	b, err := os.ReadFile(filepath.Join(r.TaskDir(id), name))
	if err != nil {
		return ""
	}
	return string(b)
}

// Status is the raw status word of id, `new` when absent.
func (r *Root) Status(id string) string {
	s := strings.TrimSpace(r.Read(id, "status"))
	if s == "" {
		return "new"
	}
	return s
}

// Exists reports whether a file exists in a task directory.
func (r *Root) Exists(id, name string) bool {
	_, err := os.Stat(filepath.Join(r.TaskDir(id), name))
	return err == nil
}

// Ledger writes an rfc's promote.md.
func (r *Root) Ledger(rfc string, lines ...string) {
	r.T.Helper()
	must(r.T, os.WriteFile(filepath.Join(r.Dir, "agent-tasks", rfc, "promote.md"), []byte(strings.Join(lines, "\n")+"\n"), 0o644))
}
