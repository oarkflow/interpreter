//go:build !js

package interpreter

import "github.com/oarkflow/interpreter/pkg/builtins/scheduler"

type Scheduler = scheduler.Scheduler
type ScheduledJob = scheduler.ScheduledJob
