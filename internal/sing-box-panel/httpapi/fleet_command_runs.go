// SPDX-License-Identifier: GPL-3.0-or-later

package httpapi

import (
	"crypto/rand"
	"slices"
	"sync"
	"time"
)

// Bounds of the fleet commands kept to follow again once they end.
const (
	fleetCommandRetention        = 15 * time.Minute
	maximumFinishedFleetCommands = 32
)

// fleetCommandRun is one fleet command's outcomes in the order they
// arrived. It outlives the request that started it, so a client whose stream
// broke, as one reaching the panel through a node the command restarts does,
// follows it again from the last outcome it read.
type fleetCommandRun struct {
	id    string
	total int

	mu      sync.Mutex
	results []fleetCommandResult
	summary *fleetCommandSummary
	ended   time.Time
	// changed is closed and replaced whenever an outcome arrives or the
	// command ends.
	changed chan struct{}
}

func (run *fleetCommandRun) add(result fleetCommandResult) {
	run.mu.Lock()
	defer run.mu.Unlock()
	run.results = append(run.results, result)
	close(run.changed)
	run.changed = make(chan struct{})
}

func (run *fleetCommandRun) finish(summary fleetCommandSummary, now time.Time) {
	run.mu.Lock()
	defer run.mu.Unlock()
	run.summary, run.ended = &summary, now
	close(run.changed)
	run.changed = make(chan struct{})
}

// since returns the outcomes after the first after, the summary once the
// command has ended, and a channel closed by the next change.
func (run *fleetCommandRun) since(after int) ([]fleetCommandResult, *fleetCommandSummary, <-chan struct{}) {
	run.mu.Lock()
	defer run.mu.Unlock()
	return run.results[min(after, len(run.results)):], run.summary, run.changed
}

func (run *fleetCommandRun) endedAt() (time.Time, bool) {
	run.mu.Lock()
	defer run.mu.Unlock()
	return run.ended, run.summary != nil
}

// fleetCommandRuns holds the running fleet commands and, for a while, the
// ones that ended. A panel restart forgets them.
type fleetCommandRuns struct {
	mu   sync.Mutex
	runs map[string]*fleetCommandRun
	now  func() time.Time
}

func newFleetCommandRuns(now func() time.Time) *fleetCommandRuns {
	return &fleetCommandRuns{runs: map[string]*fleetCommandRun{}, now: now}
}

// start registers a command for total nodes.
func (runs *fleetCommandRuns) start(total int) *fleetCommandRun {
	run := &fleetCommandRun{id: rand.Text(), total: total, changed: make(chan struct{})}
	runs.mu.Lock()
	defer runs.mu.Unlock()
	runs.prune()
	runs.runs[run.id] = run
	return run
}

// get returns the command with id, or nil when it is unknown or ended too
// long ago.
func (runs *fleetCommandRuns) get(id string) *fleetCommandRun {
	runs.mu.Lock()
	defer runs.mu.Unlock()
	runs.prune()
	return runs.runs[id]
}

// prune drops the commands that ended before the retention and then the
// oldest ended ones beyond the bound; running commands stay.
func (runs *fleetCommandRuns) prune() {
	type finishedRun struct {
		id    string
		ended time.Time
	}
	cutoff := runs.now().Add(-fleetCommandRetention)
	var finished []finishedRun
	for id, run := range runs.runs {
		ended, done := run.endedAt()
		switch {
		case !done:
		case ended.Before(cutoff):
			delete(runs.runs, id)
		default:
			finished = append(finished, finishedRun{id, ended})
		}
	}
	if len(finished) <= maximumFinishedFleetCommands {
		return
	}
	slices.SortFunc(finished, func(a, b finishedRun) int { return a.ended.Compare(b.ended) })
	for _, run := range finished[:len(finished)-maximumFinishedFleetCommands] {
		delete(runs.runs, run.id)
	}
}
