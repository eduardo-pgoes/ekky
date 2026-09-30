// Package render prints the model for people: the backlog table, the show card, and result.md.
package render

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/eduardo-pgoes/ekky/internal/model"
)

// Groups is the order the backlog prints its owner groups in.
var Groups = []model.Owner{model.OwnerMe, model.OwnerMachine, model.OwnerFinished}

// Filter is the tasks backlog prints: every task, or only the `me` group.
func Filter(tree *model.Tree, needsMe bool) []*model.Task {
	if !needsMe {
		return tree.Tasks
	}
	var out []*model.Task
	for _, t := range tree.Tasks {
		if t.Owner == model.OwnerMe {
			out = append(out, t)
		}
	}
	return out
}

// JSON writes tasks as a JSON array; this is the model's shape and a UI reads it.
func JSON(w io.Writer, tasks []*model.Task) error {
	if tasks == nil {
		tasks = []*model.Task{}
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(tasks)
}

// JSONOne writes one task object.
func JSONOne(w io.Writer, t *model.Task) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(t)
}

// Backlog writes the table: one section per owner group, in Groups order, with the columns
// rfc, task, status, upstream, branch, result.
func Backlog(w io.Writer, tree *model.Tree, needsMe bool) {
	groups := Groups
	if needsMe {
		groups = Groups[:1]
	}
	for i, owner := range groups {
		if i > 0 {
			fmt.Fprintln(w)
		}
		fmt.Fprintf(w, "%s\n", owner)
		rows := [][]string{{"rfc", "task", "status", "upstream", "branch", "result"}}
		for _, t := range tree.Tasks {
			if t.Owner != owner {
				continue
			}
			rows = append(rows, []string{t.RFC, t.Slug, string(t.Shown), dash(t.After), t.Branch, dash(summary(t))})
		}
		if len(rows) == 1 {
			fmt.Fprintln(w, "  none")
			continue
		}
		table(w, "  ", rows)
	}
}

func summary(t *model.Task) string {
	if t.Result == nil {
		return ""
	}
	return truncate(oneLine(t.Result.Summary), 80)
}

func dash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func oneLine(s string) string { return strings.Join(strings.Fields(s), " ") }

func truncate(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	r := []rune(s)
	return string(r[:n])
}

func table(w io.Writer, indent string, rows [][]string) {
	widths := make([]int, len(rows[0]))
	for _, row := range rows {
		for i, cell := range row {
			if n := utf8.RuneCountInString(cell); n > widths[i] {
				widths[i] = n
			}
		}
	}
	for _, row := range rows {
		var b strings.Builder
		b.WriteString(indent)
		for i, cell := range row {
			b.WriteString(cell)
			if i < len(row)-1 {
				b.WriteString(strings.Repeat(" ", widths[i]-utf8.RuneCountInString(cell)+2))
			}
		}
		fmt.Fprintln(w, strings.TrimRight(b.String(), " "))
	}
}

// Envelope is the header line of a result: status · $cost · duration · turns, as the script rendered it.
func Envelope(r *model.Result) string {
	secs := (r.DurationMS + 500) / 1000
	dur := fmt.Sprintf("%ds", secs)
	if secs >= 60 {
		dur = fmt.Sprintf("%dm", (secs+30)/60)
	}
	return fmt.Sprintf("%s · $%s · %s · %d turns", r.Status, money(r.CostUSD), dur, r.Turns)
}

// money renders a cost the way jq's `. * 100 | round / 100` printed it: 0.15, 1.01, 2, 0.5.
func money(usd float64) string {
	return strconv.FormatFloat(math.Round(usd*100)/100, 'f', -1, 64)
}

// Card writes the compressed result card review reads: summary, decisions as fork → chosen, deviations,
// verify, and the question on needs-input.
func Card(w io.Writer, r *model.Result) {
	fmt.Fprintln(w, Envelope(r))
	fmt.Fprintln(w)
	fmt.Fprintln(w, r.Summary)
	fmt.Fprintln(w)
	if len(r.Decisions) == 0 {
		fmt.Fprintln(w, "decisions: none recorded")
	} else {
		fmt.Fprintln(w, "decisions:")
		for _, d := range r.Decisions {
			fmt.Fprintf(w, "- %s → %s\n    rejected: %s\n    why: %s\n", d.Fork, d.Chosen, d.Rejected, d.Why)
		}
	}
	fmt.Fprintf(w, "deviations: %s\n", dashWord(r.Deviations, "none"))
	fmt.Fprintln(w, "verify:")
	for _, line := range strings.Split(strings.TrimRight(r.Verify, "\n"), "\n") {
		fmt.Fprintf(w, "    %s\n", line)
	}
	if r.Status == string(model.NeedsInput) {
		fmt.Fprintf(w, "question: %s\n", r.Question)
	}
}

func dashWord(s, empty string) string {
	if strings.TrimSpace(s) == "" {
		return empty
	}
	return s
}

// Show writes one task: the record header, shown status and owner, descendants, the card, and the tail of run.log.
func Show(w io.Writer, t *model.Task, logTail int) {
	fmt.Fprintf(w, "%s — %s\n", t.ID, dash(t.Title))
	rows := [][]string{
		{"repo:", dash(t.Repo)}, {"base:", dash(t.Base)}, {"model:", dash(t.Model)},
		{"account:", dash(t.Account)}, {"source:", dash(t.Source)}, {"after:", dash(t.After)},
		{"status:", fmt.Sprintf("%s (%s)", t.Shown, t.Owner)}, {"branch:", t.Branch},
	}
	if t.PR != "" {
		rows = append(rows, []string{"pr:", t.PR})
	}
	rows = append(rows, []string{"descendants:", dashWord(strings.Join(t.Descendants, ", "), "none")})
	table(w, "", rows)
	if t.Result != nil {
		fmt.Fprintln(w)
		fmt.Fprintln(w, "## Result")
		fmt.Fprintln(w)
		Card(w, t.Result)
	}
	tail := Tail(filepath.Join(t.Dir, "run.log"), logTail)
	if tail != "" {
		fmt.Fprintln(w)
		fmt.Fprintf(w, "## run.log (last %d lines)\n\n", logTail)
		fmt.Fprint(w, tail)
	}
}

// Tail is the last n lines of a file, "" when it is absent or empty.
func Tail(path string, n int) string {
	b, err := os.ReadFile(path)
	if err != nil || len(b) == 0 || n <= 0 {
		return ""
	}
	s := strings.TrimRight(string(b), "\n")
	lines := strings.Split(s, "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n") + "\n"
}

// Result writes result.md in the script's shape: title, the header line, summary, decisions, deviations,
// verify, and the question on needs-input.
func Result(w io.Writer, title, branch, modelName string, r *model.Result) {
	fmt.Fprintf(w, "# %s\n\n", title)
	fmt.Fprintf(w, "%s · `%s` · %s · %s\n\n", r.Status, branch, modelName, strings.TrimPrefix(Envelope(r), r.Status+" · "))
	fmt.Fprintf(w, "%s\n\n", r.Summary)
	fmt.Fprint(w, "## Decisions\n\n")
	if len(r.Decisions) == 0 {
		fmt.Fprint(w, "none recorded\n\n")
	} else {
		for _, d := range r.Decisions {
			fmt.Fprintf(w, "- %s\n  chose: %s\n  rejected: %s\n  why: %s\n\n", d.Fork, d.Chosen, d.Rejected, d.Why)
		}
	}
	fmt.Fprint(w, "## Deviations\n\n")
	if r.Deviations == "" {
		fmt.Fprint(w, "none\n\n")
	} else {
		fmt.Fprintf(w, "%s\n\n", r.Deviations)
	}
	fmt.Fprintf(w, "## Verify\n\n%s\n\n", r.Verify)
	if r.Status == string(model.NeedsInput) {
		fmt.Fprintf(w, "## Question\n\n%s\n\n", r.Question)
	}
}
