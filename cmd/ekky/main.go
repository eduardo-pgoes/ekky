// ekky — dispatcher for the private AI task daemon. tackle | drain | backlog | show | cancel
package main

import (
	"fmt"
	"os"
)

const usage = `usage: ekky tackle <id>... | all | <rfc>     mark ready and start the drain
       ekky drain                             run every ready task, SLOTS at a time
       ekky backlog [--needs-me] [--json]     the table, grouped by who the task waits on
       ekky show <id> [--json]                one task: record, status, result card, log tail
       ekky cancel <id>                       stop a running attempt; it lands failed / cancelled
       ekky serve                             the ui: /api + ui/dist over http, loopback only

<id> is <rfc>/<nn-slug>; stray tasks live under the null rfc, e.g. _/03-fix-cors
a task with ` + "`after: <nn-slug>`" + ` is cut from that sibling's ekky/ branch once it is done;
tackling a task re-readies every task downstream of it
env:   EKKY_ROOT (~/ekky)  SLOTS (2)  EKKY_BUDGET (15)  EKKY_TIMEOUT (90m)  EKKY_CLAUDE
       EKKY_LOG_TAIL (40)  EKKY_NO_SYSTEMD (drain in the foreground)  EKKY_UI_ADDR (127.0.0.1:5178)
       a task's ` + "`account:`" + ` picks CLAUDE_CONFIG_DIR: perso → ~/.claude, <name> → ~/.claude-<name>
`

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	verb, args := os.Args[1], os.Args[2:]
	run, ok := verbs[verb]
	if !ok {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	if err := run(args); err != nil {
		fmt.Fprintf(os.Stderr, "ekky: %v\n", err)
		os.Exit(1)
	}
}

var verbs = map[string]func(args []string) error{
	"tackle":  tackle,
	"drain":   drain,
	"backlog": backlog,
	"show":    show,
	"cancel":  cancel,
	"serve":   serveUI,
}
