package interpreter

import (
	"testing"
	"time"
)

func TestScheduleRunExecutesDueJobsSynchronously(t *testing.T) {
	globalScheduler = &Scheduler{jobs: make(map[string]*ScheduledJob), stopCh: make(chan struct{})}
	time.Sleep(2 * time.Millisecond)
	res, err := ExecWithOptions(`
let hits = 0;
let id = schedule_interval(1, "tick", function() {
    hits = hits + 1;
});
sleep(5);
schedule_run(2);
schedule_cancel(id);
hits;
`, nil, ExecOptions{})
	if err != nil {
		t.Fatal(err)
	}
	count, ok := res.(*Integer)
	if !ok {
		t.Fatalf("expected integer result, got %T", res)
	}
	if count.Value < 1 {
		t.Fatalf("expected at least one synchronous run, got %d", count.Value)
	}
}
