// Package serve is the read-only HTTP view over agent-tasks/: the model as JSON for the UI, plus
// ui/dist as static files. Every request rebuilds its view from the files, like every other verb;
// nothing here writes and nothing is held in memory between requests.
package serve

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/eduardo-pgoes/ekky/internal/config"
	"github.com/eduardo-pgoes/ekky/internal/model"
	"github.com/eduardo-pgoes/ekky/internal/render"
)

// logTail is how many run.log lines /api/task returns; `ekky show <id>` has the rest.
const logTail = 120

// detail is what the UI's task pane reads: the task object plus the human-readable files.
type detail struct {
	Task     *model.Task `json:"task"`
	Record   string      `json:"record"`
	Log      string      `json:"log"`
	RFCTitle string      `json:"rfcTitle"`
}

// Handler serves /api/backlog, /api/task/<rfc>/<slug>, and ui/dist under root.
func Handler(root string) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/backlog", func(w http.ResponseWriter, r *http.Request) {
		tree, err := model.Load(root)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		jsonHeaders(w)
		render.JSON(w, render.Filter(tree, false))
	})
	mux.HandleFunc("GET /api/task/{rfc}/{slug}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("rfc") + "/" + r.PathValue("slug")
		if _, _, err := model.ParseID(id); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		tree, err := model.Load(root)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		t := tree.Get(id)
		if t == nil {
			http.Error(w, fmt.Sprintf("no task %s under %s", id, model.TasksDir(root)), http.StatusNotFound)
			return
		}
		jsonHeaders(w)
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		enc.Encode(detail{
			Task:     t,
			Record:   readFile(filepath.Join(t.Dir, "task.md")),
			Log:      tail(readFile(filepath.Join(t.Dir, "run.log")), logTail),
			RFCTitle: rfcTitle(filepath.Join(model.TasksDir(root), t.RFC, "rfc.md")),
		})
	})
	mux.Handle("GET /", http.FileServer(http.Dir(filepath.Join(root, "ui", "dist"))))
	return mux
}

// Serve listens on cfg.UIAddr until the process is stopped: the always-on half of the UI,
// run by ekky-ui.service. The vite dev server proxies /api here.
func Serve(cfg config.Config, out io.Writer) error {
	if !exists(filepath.Join(cfg.Root, "ui", "dist", "index.html")) {
		fmt.Fprintf(out, "ekky: no ui/dist under %s — run `pnpm build` in ui/; serving /api only\n", cfg.Root)
	}
	fmt.Fprintf(out, "ekky: ui on http://%s (root %s)\n", cfg.UIAddr, cfg.Root)
	srv := &http.Server{Addr: cfg.UIAddr, Handler: Handler(cfg.Root), ReadHeaderTimeout: 5 * time.Second}
	return srv.ListenAndServe()
}

func jsonHeaders(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
}

func readFile(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(b)
}

// tail is the last n non-empty lines, no trailing newline — the shape the UI's log pane was built against.
func tail(s string, n int) string {
	var lines []string
	for _, l := range strings.Split(s, "\n") {
		if l != "" {
			lines = append(lines, l)
		}
	}
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}

// rfcTitle is the first `# ` heading of an rfc.md, its `RFC: ` prefix stripped, "" when absent.
func rfcTitle(path string) string {
	for _, l := range strings.Split(readFile(path), "\n") {
		if t, ok := strings.CutPrefix(l, "# "); ok {
			return strings.TrimPrefix(strings.TrimSpace(t), "RFC: ")
		}
	}
	return ""
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
