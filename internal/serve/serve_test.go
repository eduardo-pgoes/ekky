package serve_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/eduardo-pgoes/ekky/internal/harness"
	"github.com/eduardo-pgoes/ekky/internal/model"
	"github.com/eduardo-pgoes/ekky/internal/serve"
)

func TestMain(m *testing.M) {
	harness.MaybeFake()
	os.Exit(m.Run())
}

func get(t *testing.T, srv *httptest.Server, path string) (int, string) {
	t.Helper()
	res, err := http.Get(srv.URL + path)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	b, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	return res.StatusCode, string(b)
}

func TestHandler(t *testing.T) {
	root := harness.NewRoot(t)
	repo := root.NewRepo("r")
	root.Simple("r/01-a", repo)
	root.Write("r/01-a", "run.log", "ekky: cut worktree\n\none\ntwo\n")
	must(t, os.WriteFile(filepath.Join(root.Dir, "agent-tasks", "r", "rfc.md"), []byte("# RFC: Bot routing\n\nprose\n"), 0o644))
	dist := filepath.Join(root.Dir, "ui", "dist")
	must(t, os.MkdirAll(dist, 0o755))
	must(t, os.WriteFile(filepath.Join(dist, "index.html"), []byte("<title>ekky</title>"), 0o644))

	srv := httptest.NewServer(serve.Handler(root.Dir))
	defer srv.Close()

	code, body := get(t, srv, "/api/backlog")
	if code != 200 {
		t.Fatalf("backlog: %d %s", code, body)
	}
	var tasks []model.Task
	must(t, json.Unmarshal([]byte(body), &tasks))
	if len(tasks) != 1 || tasks[0].ID != "r/01-a" {
		t.Fatalf("backlog: %s", body)
	}

	code, body = get(t, srv, "/api/task/r/01-a")
	if code != 200 {
		t.Fatalf("task: %d %s", code, body)
	}
	var d struct {
		Task     model.Task `json:"task"`
		Record   string     `json:"record"`
		Log      string     `json:"log"`
		RFCTitle string     `json:"rfcTitle"`
	}
	must(t, json.Unmarshal([]byte(body), &d))
	if d.Task.ID != "r/01-a" || !strings.Contains(d.Record, "# Task 01-a") {
		t.Errorf("detail task/record: %+v", d)
	}
	if d.Log != "ekky: cut worktree\none\ntwo" {
		t.Errorf("log %q: want empty lines dropped, no trailing newline", d.Log)
	}
	if d.RFCTitle != "Bot routing" {
		t.Errorf("rfcTitle %q", d.RFCTitle)
	}

	if code, body = get(t, srv, "/api/task/r/99-x"); code != 404 {
		t.Errorf("missing task: %d %s", code, body)
	}
	if code, body = get(t, srv, "/"); code != 200 || !strings.Contains(body, "<title>ekky</title>") {
		t.Errorf("index: %d %s", code, body)
	}
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
