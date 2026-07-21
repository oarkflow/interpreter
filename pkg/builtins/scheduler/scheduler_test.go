package scheduler

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/oarkflow/interpreter/pkg/object"
)

func TestDueJobCannotOverlapItself(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	var calls atomic.Int32

	s := &Scheduler{Jobs: make(map[string]*ScheduledJob), StopCh: make(chan struct{})}
	job := &ScheduledJob{
		ID:       "job_1",
		Active:   true,
		Interval: time.Millisecond,
		NextRun:  time.Now().Add(-time.Second),
		Env:      object.NewEnvironment(),
		Handler: &object.Builtin{Fn: func(args ...object.Object) object.Object {
			calls.Add(1)
			close(started)
			<-release
			return object.NULL
		}},
	}
	s.Jobs[job.ID] = job

	s.tick(time.Now())
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("scheduled job did not start")
	}

	if got := s.runDueJobsBlocking(time.Now().Add(time.Second)); got != 0 {
		t.Fatalf("running job was selected a second time: got %d due jobs", got)
	}
	close(release)

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		s.mu.RLock()
		running := job.running
		s.mu.RUnlock()
		if !running {
			break
		}
		time.Sleep(time.Millisecond)
	}
	s.mu.RLock()
	running := job.running
	s.mu.RUnlock()
	if running {
		t.Fatal("scheduled job did not finish")
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("job overlapped itself: got %d calls", got)
	}
}
