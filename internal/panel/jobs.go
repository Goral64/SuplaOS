package panel

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"
)

// Job is a long operation (install, update...) whose output the browser
// follows live. One job runs at a time.
type Job struct {
	ID      int       `json:"id"`
	Title   string    `json:"title"`
	State   string    `json:"state"` // running, done, failed
	Error   string    `json:"error,omitempty"`
	Started time.Time `json:"started"`

	mu      sync.Mutex
	changed *sync.Cond
	lines   []string
	partial string
}

// ansi matches terminal control sequences docker compose prints even
// without a terminal.
var ansi = regexp.MustCompile(`\x1b\[[0-9;?]*[A-Za-z]`)

func stripANSI(s string) string { return ansi.ReplaceAllString(s, "") }

// Write collects the output in lines; progress output with \r counts as
// separate lines.
func (j *Job) Write(p []byte) (int, error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	text := j.partial + strings.ReplaceAll(stripANSI(string(p)), "\r", "\n")
	parts := strings.Split(text, "\n")
	j.partial = parts[len(parts)-1]
	for _, l := range parts[:len(parts)-1] {
		if strings.TrimSpace(l) != "" {
			j.lines = append(j.lines, l)
		}
	}
	j.changed.Broadcast()
	return len(p), nil
}

// Logf adds a line of the panel's own.
func (j *Job) Logf(format string, args ...any) {
	fmt.Fprintf(j, format+"\n", args...)
}

func (j *Job) finish(err error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.partial != "" {
		j.lines = append(j.lines, j.partial)
		j.partial = ""
	}
	j.State = "done"
	if err != nil {
		j.State = "failed"
		j.Error = err.Error()
	}
	j.changed.Broadcast()
}

// follow calls send with the lines from index from on until the job ends
// or the context is cancelled; it returns false in the latter case.
func (j *Job) follow(ctx context.Context, send func(lines []string)) bool {
	stop := context.AfterFunc(ctx, func() {
		j.mu.Lock()
		j.changed.Broadcast()
		j.mu.Unlock()
	})
	defer stop()
	sent := 0
	j.mu.Lock()
	defer j.mu.Unlock()
	for {
		if ctx.Err() != nil {
			return false
		}
		if sent < len(j.lines) {
			lines := append([]string(nil), j.lines[sent:]...)
			sent = len(j.lines)
			j.mu.Unlock()
			send(lines)
			j.mu.Lock()
			continue
		}
		if j.State != "running" {
			return true
		}
		j.changed.Wait()
	}
}

func (j *Job) snapshot() Job {
	j.mu.Lock()
	defer j.mu.Unlock()
	return Job{ID: j.ID, Title: j.Title, State: j.State, Error: j.Error, Started: j.Started}
}

type jobs struct {
	mu      sync.Mutex
	next    int
	list    []*Job
	running *Job
}

// errBusy is returned when another job is running.
type errBusy struct{ job *Job }

func (e errBusy) Error() string { return "another operation is running: " + e.job.Title }

// start runs fn in the background as a new job.
func (js *jobs) start(title string, fn func(j *Job) error) (*Job, error) {
	js.mu.Lock()
	defer js.mu.Unlock()
	if js.running != nil {
		return nil, errBusy{js.running}
	}
	js.next++
	j := &Job{ID: js.next, Title: title, State: "running", Started: time.Now()}
	j.changed = sync.NewCond(&j.mu)
	js.list = append(js.list, j)
	if len(js.list) > 20 {
		js.list = js.list[len(js.list)-20:]
	}
	js.running = j
	go func() {
		err := fn(j)
		j.finish(err)
		js.mu.Lock()
		js.running = nil
		js.mu.Unlock()
	}()
	return j, nil
}

func (js *jobs) get(id int) *Job {
	js.mu.Lock()
	defer js.mu.Unlock()
	for _, j := range js.list {
		if j.ID == id {
			return j
		}
	}
	return nil
}

func (js *jobs) current() *Job {
	js.mu.Lock()
	defer js.mu.Unlock()
	return js.running
}
