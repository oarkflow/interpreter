package interpreter

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ── Cron Parser ────────────────────────────────────────────────────

type CronSchedule struct {
	Minutes     []int // 0-59
	Hours       []int // 0-23
	DaysOfMonth []int // 1-31
	Months      []int // 1-12
	DaysOfWeek  []int // 0-6 (0=Sunday)
	Raw         string
}

func parseCronExpression(expr string) (*CronSchedule, error) {
	fields := strings.Fields(expr)
	if len(fields) != 5 {
		return nil, fmt.Errorf("cron expression must have 5 fields, got %d", len(fields))
	}
	minutes, err := parseCronField(fields[0], 0, 59)
	if err != nil {
		return nil, fmt.Errorf("minute field: %w", err)
	}
	hours, err := parseCronField(fields[1], 0, 23)
	if err != nil {
		return nil, fmt.Errorf("hour field: %w", err)
	}
	dom, err := parseCronField(fields[2], 1, 31)
	if err != nil {
		return nil, fmt.Errorf("day-of-month field: %w", err)
	}
	months, err := parseCronField(fields[3], 1, 12)
	if err != nil {
		return nil, fmt.Errorf("month field: %w", err)
	}
	dow, err := parseCronField(fields[4], 0, 6)
	if err != nil {
		return nil, fmt.Errorf("day-of-week field: %w", err)
	}
	return &CronSchedule{
		Minutes:     minutes,
		Hours:       hours,
		DaysOfMonth: dom,
		Months:      months,
		DaysOfWeek:  dow,
		Raw:         expr,
	}, nil
}

func parseCronField(field string, min, max int) ([]int, error) {
	if field == "*" {
		r := make([]int, max-min+1)
		for i := range r {
			r[i] = min + i
		}
		return r, nil
	}

	// Handle */N
	if strings.HasPrefix(field, "*/") {
		step, err := strconv.Atoi(field[2:])
		if err != nil || step <= 0 {
			return nil, fmt.Errorf("invalid step: %s", field)
		}
		var r []int
		for i := min; i <= max; i += step {
			r = append(r, i)
		}
		return r, nil
	}

	var result []int
	parts := strings.Split(field, ",")
	for _, part := range parts {
		// Handle ranges
		if strings.Contains(part, "-") {
			rangeParts := strings.SplitN(part, "-", 2)
			start, err := strconv.Atoi(rangeParts[0])
			if err != nil {
				return nil, fmt.Errorf("invalid range start: %s", part)
			}
			end, err := strconv.Atoi(rangeParts[1])
			if err != nil {
				return nil, fmt.Errorf("invalid range end: %s", part)
			}
			if start < min || end > max || start > end {
				return nil, fmt.Errorf("range out of bounds: %s", part)
			}
			for i := start; i <= end; i++ {
				result = append(result, i)
			}
		} else {
			val, err := strconv.Atoi(part)
			if err != nil {
				return nil, fmt.Errorf("invalid value: %s", part)
			}
			if val < min || val > max {
				return nil, fmt.Errorf("value %d out of range [%d-%d]", val, min, max)
			}
			result = append(result, val)
		}
	}
	return result, nil
}

func (cs *CronSchedule) Matches(t time.Time) bool {
	return intSliceContains(cs.Minutes, t.Minute()) &&
		intSliceContains(cs.Hours, t.Hour()) &&
		intSliceContains(cs.DaysOfMonth, t.Day()) &&
		intSliceContains(cs.Months, int(t.Month())) &&
		intSliceContains(cs.DaysOfWeek, int(t.Weekday()))
}

func (cs *CronSchedule) Next(from time.Time) time.Time {
	// Truncate to the current minute, then step forward
	t := from.Truncate(time.Minute).Add(time.Minute)
	for i := 0; i < 525960; i++ { // max ~1 year of minutes
		if cs.Matches(t) {
			return t
		}
		t = t.Add(time.Minute)
	}
	return time.Time{} // no match found within a year
}

func schedulerNow() time.Time {
	globalScheduler.mu.RLock()
	loc := globalScheduler.timezone
	globalScheduler.mu.RUnlock()
	if loc != nil {
		return time.Now().In(loc)
	}
	return time.Now()
}

func intSliceContains(s []int, v int) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}

// ── Scheduler Engine ───────────────────────────────────────────────

type ScheduledJob struct {
	ID       string
	Name     string
	Schedule *CronSchedule
	Handler  Object
	Env      *Environment
	Once     bool
	Interval time.Duration // for interval-based jobs
	NextRun  time.Time
	LastRun  time.Time
	RunCount int64
	Active   bool
}

type Scheduler struct {
	mu          sync.RWMutex
	jobs        map[string]*ScheduledJob
	nextID      int
	stopCh      chan struct{}
	running     bool
	persistPath string
	timezone    *time.Location
}

var globalScheduler = &Scheduler{
	jobs:   make(map[string]*ScheduledJob),
	stopCh: make(chan struct{}),
}

func (s *Scheduler) start() {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return
	}
	s.running = true
	s.mu.Unlock()

	go s.loop()
}

func (s *Scheduler) loop() {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-s.stopCh:
			return
		case now := <-ticker.C:
			s.tick(now)
		}
	}
}

func (s *Scheduler) tick(now time.Time) {
	s.mu.RLock()
	var toRun []*ScheduledJob
	for _, job := range s.jobs {
		if !job.Active {
			continue
		}
		if job.Interval > 0 {
			if now.After(job.NextRun) || now.Equal(job.NextRun) {
				toRun = append(toRun, job)
			}
		} else if job.Schedule != nil {
			if now.After(job.NextRun) || now.Equal(job.NextRun) {
				if job.Schedule.Matches(now.Truncate(time.Minute)) {
					toRun = append(toRun, job)
				}
			}
		}
	}
	s.mu.RUnlock()

	for _, job := range toRun {
		go s.executeJob(job, now)
	}
}

func (s *Scheduler) executeJob(job *ScheduledJob, now time.Time) {
	s.mu.Lock()
	job.LastRun = now
	job.RunCount++
	if job.Once {
		job.Active = false
	} else if job.Interval > 0 {
		job.NextRun = now.Add(job.Interval)
	} else if job.Schedule != nil {
		job.NextRun = job.Schedule.Next(now)
	}
	handler := job.Handler
	env := job.Env
	s.mu.Unlock()

	switch fn := handler.(type) {
	case *Function:
		extEnv := extendFunctionEnv(fn, nil, env, nil)
		Eval(fn.Body, extEnv)
	case *Builtin:
		if fn.FnWithEnv != nil {
			fn.FnWithEnv(env)
		} else {
			fn.Fn()
		}
	}
}

func (s *Scheduler) runDueJobsBlocking(now time.Time) int {
	s.mu.RLock()
	var toRun []*ScheduledJob
	for _, job := range s.jobs {
		if !job.Active {
			continue
		}
		if job.Interval > 0 {
			if now.After(job.NextRun) || now.Equal(job.NextRun) {
				toRun = append(toRun, job)
			}
		} else if job.Schedule != nil {
			if now.After(job.NextRun) || now.Equal(job.NextRun) {
				if job.Schedule.Matches(now.Truncate(time.Minute)) {
					toRun = append(toRun, job)
				}
			}
		}
	}
	s.mu.RUnlock()
	for _, job := range toRun {
		s.executeJob(job, now)
	}
	return len(toRun)
}

func (s *Scheduler) addJob(job *ScheduledJob) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nextID++
	job.ID = fmt.Sprintf("job_%d", s.nextID)
	job.Active = true
	s.jobs[job.ID] = job
	return job.ID
}

func (s *Scheduler) restore(path string) error {
	type savedJob struct {
		ID       string `json:"id"`
		Name     string `json:"name"`
		Cron     string `json:"cron,omitempty"`
		Interval string `json:"interval,omitempty"`
		Once     bool   `json:"once,omitempty"`
		Active   bool   `json:"active"`
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var saved []savedJob
	if err := json.Unmarshal(data, &saved); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, item := range saved {
		job := &ScheduledJob{ID: item.ID, Name: item.Name, Once: item.Once, Active: item.Active}
		if item.Cron != "" {
			sched, err := parseCronExpression(item.Cron)
			if err != nil {
				continue
			}
			job.Schedule = sched
			job.NextRun = sched.Next(time.Now())
		}
		if item.Interval != "" {
			interval, err := time.ParseDuration(item.Interval)
			if err != nil {
				continue
			}
			job.Interval = interval
			job.NextRun = time.Now().Add(interval)
		}
		if job.ID == "" {
			s.nextID++
			job.ID = fmt.Sprintf("job_%d", s.nextID)
		}
		if !job.NextRun.IsZero() {
			s.jobs[job.ID] = job
		}
	}
	return nil
}

func (s *Scheduler) cancelJob(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if job, ok := s.jobs[id]; ok {
		job.Active = false
		delete(s.jobs, id)
		return true
	}
	return false
}

func (s *Scheduler) listJobs() []*ScheduledJob {
	s.mu.RLock()
	defer s.mu.RUnlock()
	jobs := make([]*ScheduledJob, 0, len(s.jobs))
	for _, job := range s.jobs {
		jobs = append(jobs, job)
	}
	sort.Slice(jobs, func(i, j int) bool { return jobs[i].ID < jobs[j].ID })
	return jobs
}

func (s *Scheduler) persist() error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.persistPath == "" {
		return fmt.Errorf("no persist path configured")
	}
	type savedJob struct {
		ID       string `json:"id"`
		Name     string `json:"name"`
		Cron     string `json:"cron,omitempty"`
		Interval string `json:"interval,omitempty"`
		Once     bool   `json:"once,omitempty"`
		Active   bool   `json:"active"`
	}
	var saved []savedJob
	for _, job := range s.jobs {
		sj := savedJob{
			ID:     job.ID,
			Name:   job.Name,
			Active: job.Active,
			Once:   job.Once,
		}
		if job.Schedule != nil {
			sj.Cron = job.Schedule.Raw
		}
		if job.Interval > 0 {
			sj.Interval = job.Interval.String()
		}
		saved = append(saved, sj)
	}
	data, err := json.MarshalIndent(saved, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.persistPath, data, 0644)
}

// ── Builtins ───────────────────────────────────────────────────────

func init() {
	registerBuiltins(map[string]*Builtin{
		"schedule":          {FnWithEnv: builtinSchedule},
		"schedule_once":     {FnWithEnv: builtinScheduleOnce},
		"schedule_interval": {FnWithEnv: builtinScheduleInterval},
		"schedule_cancel":   {Fn: builtinScheduleCancel},
		"schedule_list":     {Fn: builtinScheduleList},
		"schedule_persist":  {Fn: builtinSchedulePersist},
		"schedule_restore":  {Fn: builtinScheduleRestore},
		"schedule_now":      {FnWithEnv: builtinScheduleNow},
		"schedule_run":      {Fn: builtinScheduleRun},
		"schedule_worker":   {Fn: builtinScheduleWorker},
		"schedule_timezone": {Fn: builtinScheduleTimezone},
		"background":        {FnWithEnv: builtinBackground},
	})
}

// schedule(cron_expr, handler) or schedule(cron_expr, name, handler)
func builtinSchedule(env *Environment, args ...Object) Object {
	if len(args) < 2 {
		return newError("schedule() requires (cron_expr, handler)")
	}
	cronStr, ok := args[0].(*String)
	if !ok {
		return newError("schedule() first argument must be a cron expression string")
	}
	sched, err := parseCronExpression(cronStr.Value)
	if err != nil {
		return newError("invalid cron expression: %s", err)
	}

	var name string
	var handler Object
	if len(args) >= 3 {
		if n, ok := args[1].(*String); ok {
			name = n.Value
		}
		handler = args[2]
	} else {
		handler = args[1]
		name = cronStr.Value
	}

	job := &ScheduledJob{
		Name:     name,
		Schedule: sched,
		Handler:  handler,
		Env:      env,
		NextRun:  sched.Next(schedulerNow()),
	}

	globalScheduler.start()
	id := globalScheduler.addJob(job)
	return &String{Value: id}
}

// schedule_once(cron_expr, handler)
func builtinScheduleOnce(env *Environment, args ...Object) Object {
	if len(args) < 2 {
		return newError("schedule_once() requires (cron_expr, handler)")
	}
	cronStr, ok := args[0].(*String)
	if !ok {
		return newError("schedule_once() first argument must be a cron expression string")
	}
	sched, err := parseCronExpression(cronStr.Value)
	if err != nil {
		return newError("invalid cron expression: %s", err)
	}

	var name string
	if len(args) >= 3 {
		if n, ok := args[1].(*String); ok {
			name = n.Value
		}
	}

	handler := args[len(args)-1]
	job := &ScheduledJob{
		Name:     name,
		Schedule: sched,
		Handler:  handler,
		Env:      env,
		Once:     true,
		NextRun:  sched.Next(schedulerNow()),
	}

	globalScheduler.start()
	id := globalScheduler.addJob(job)
	return &String{Value: id}
}

// schedule_interval(duration_ms, handler) or schedule_interval(duration_str, handler)
func builtinScheduleInterval(env *Environment, args ...Object) Object {
	if len(args) < 2 {
		return newError("schedule_interval() requires (duration, handler)")
	}

	var interval time.Duration
	switch v := args[0].(type) {
	case *Integer:
		interval = time.Duration(v.Value) * time.Millisecond
	case *String:
		d, err := time.ParseDuration(v.Value)
		if err != nil {
			return newError("invalid duration: %s", err)
		}
		interval = d
	default:
		return newError("schedule_interval() duration must be integer (ms) or string")
	}

	if interval <= 0 {
		return newError("schedule_interval() duration must be positive")
	}

	var name string
	var handler Object
	if len(args) >= 3 {
		if n, ok := args[1].(*String); ok {
			name = n.Value
		}
		handler = args[2]
	} else {
		handler = args[1]
		name = fmt.Sprintf("every %s", interval)
	}

	job := &ScheduledJob{
		Name:     name,
		Handler:  handler,
		Env:      env,
		Interval: interval,
		NextRun:  schedulerNow().Add(interval),
	}

	globalScheduler.start()
	id := globalScheduler.addJob(job)
	return &String{Value: id}
}

// schedule_cancel(job_id)
func builtinScheduleCancel(args ...Object) Object {
	if len(args) < 1 {
		return newError("schedule_cancel() requires a job ID")
	}
	id, ok := args[0].(*String)
	if !ok {
		return newError("schedule_cancel() argument must be a string")
	}
	if globalScheduler.cancelJob(id.Value) {
		return TRUE
	}
	return FALSE
}

// schedule_list() -> array of job info hashes
func builtinScheduleList(args ...Object) Object {
	jobs := globalScheduler.listJobs()
	elems := make([]Object, len(jobs))
	for i, job := range jobs {
		h := &Hash{Pairs: make(map[HashKey]HashPair)}
		setHashStr := func(k, v string) {
			key := &String{Value: k}
			h.Pairs[key.HashKey()] = HashPair{Key: key, Value: &String{Value: v}}
		}
		setHashStr("id", job.ID)
		setHashStr("name", job.Name)
		ak := &String{Value: "active"}
		if job.Active {
			h.Pairs[ak.HashKey()] = HashPair{Key: ak, Value: TRUE}
		} else {
			h.Pairs[ak.HashKey()] = HashPair{Key: ak, Value: FALSE}
		}
		rk := &String{Value: "run_count"}
		h.Pairs[rk.HashKey()] = HashPair{Key: rk, Value: &Integer{Value: job.RunCount}}
		if !job.NextRun.IsZero() {
			setHashStr("next_run", job.NextRun.Format(time.RFC3339))
		}
		if !job.LastRun.IsZero() {
			setHashStr("last_run", job.LastRun.Format(time.RFC3339))
		}
		if job.Schedule != nil {
			setHashStr("cron", job.Schedule.Raw)
		}
		if job.Interval > 0 {
			setHashStr("interval", job.Interval.String())
		}
		elems[i] = h
	}
	return &Array{Elements: elems}
}

// schedule_persist(path)
func builtinSchedulePersist(args ...Object) Object {
	if len(args) < 1 {
		return newError("schedule_persist() requires a file path")
	}
	path, ok := args[0].(*String)
	if !ok {
		return newError("schedule_persist() argument must be a string")
	}
	globalScheduler.mu.Lock()
	globalScheduler.persistPath = path.Value
	globalScheduler.mu.Unlock()
	if err := globalScheduler.persist(); err != nil {
		return newError("persist error: %s", err)
	}
	return TRUE
}

// schedule_restore(path)
func builtinScheduleRestore(args ...Object) Object {
	if len(args) < 1 {
		return newError("schedule_restore() requires a file path")
	}
	path, ok := args[0].(*String)
	if !ok {
		return newError("schedule_restore() argument must be a string")
	}
	if err := globalScheduler.restore(path.Value); err != nil {
		return newError("restore error: %s", err)
	}
	globalScheduler.start()
	return TRUE
}

// schedule_now(handler)
func builtinScheduleNow(env *Environment, args ...Object) Object {
	if len(args) < 1 {
		return newError("schedule_now() requires a handler")
	}
	go globalScheduler.executeJob(&ScheduledJob{Handler: args[0], Env: env, Active: true}, time.Now())
	return TRUE
}

// schedule_run() or schedule_run(limit)
func builtinScheduleRun(args ...Object) Object {
	limit := 1
	if len(args) >= 1 {
		if n, ok := args[0].(*Integer); ok {
			if n.Value <= 0 {
				return newError("schedule_run() limit must be positive")
			}
			limit = int(n.Value)
		}
	}
	runs := int64(0)
	for i := 0; i < limit; i++ {
		runs += int64(globalScheduler.runDueJobsBlocking(schedulerNow()))
		if i+1 < limit {
			time.Sleep(time.Second)
		}
	}
	return &Integer{Value: runs}
}

// schedule_worker() or schedule_worker(duration_ms|duration_str)
func builtinScheduleWorker(args ...Object) Object {
	var stopAt time.Time
	if len(args) >= 1 {
		switch v := args[0].(type) {
		case *Integer:
			if v.Value <= 0 {
				return newError("schedule_worker() duration must be positive")
			}
			stopAt = time.Now().Add(time.Duration(v.Value) * time.Millisecond)
		case *String:
			d, err := time.ParseDuration(v.Value)
			if err != nil {
				return newError("schedule_worker() invalid duration: %s", err)
			}
			stopAt = time.Now().Add(d)
		default:
			return newError("schedule_worker() duration must be integer (ms) or string")
		}
	}
	runs := int64(0)
	for {
		now := schedulerNow()
		runs += int64(globalScheduler.runDueJobsBlocking(now))
		if !stopAt.IsZero() && (now.After(stopAt) || now.Equal(stopAt)) {
			break
		}
		time.Sleep(time.Second)
	}
	return &Integer{Value: runs}
}

// schedule_timezone(name)
func builtinScheduleTimezone(args ...Object) Object {
	if len(args) < 1 {
		return newError("schedule_timezone() requires a timezone name")
	}
	name, ok := args[0].(*String)
	if !ok {
		return newError("schedule_timezone() argument must be a string")
	}
	loc, err := time.LoadLocation(name.Value)
	if err != nil {
		return newError("invalid timezone: %s", err)
	}
	globalScheduler.mu.Lock()
	globalScheduler.timezone = loc
	globalScheduler.mu.Unlock()
	return TRUE
}

// background(handler) — runs function in goroutine, returns future
func builtinBackground(env *Environment, args ...Object) Object {
	if len(args) < 1 {
		return newError("background() requires a function")
	}
	handler := args[0]
	ch := make(chan Object, 1)
	go func() {
		var result Object
		switch fn := handler.(type) {
		case *Function:
			extEnv := extendFunctionEnv(fn, args[1:], env, nil)
			result = Eval(fn.Body, extEnv)
			result = unwrapReturnValue(result)
		case *Builtin:
			if fn.FnWithEnv != nil {
				result = fn.FnWithEnv(env, args[1:]...)
			} else {
				result = fn.Fn(args[1:]...)
			}
		default:
			result = newError("background() argument must be a function")
		}
		ch <- result
	}()
	return &Future{ch: ch}
}
