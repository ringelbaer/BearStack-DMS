package photos

import (
	"context"
	"strconv"
	"sync"
	"time"
)

type listTraceContextKey struct{}

type ListTrace struct {
	mu        sync.Mutex
	startedAt time.Time
	steps     []ListTraceStep
}

type ListTraceStep struct {
	Name     string
	Duration time.Duration
	Fields   []ListTraceField
}

type ListTraceField struct {
	Key   string
	Value string
}

type ListTraceSnapshot struct {
	Enabled       bool
	StartedAt     time.Time
	TotalDuration time.Duration
	Steps         []ListTraceStep
}

func NewListTrace() *ListTrace {
	return &ListTrace{startedAt: time.Now()}
}

func ContextWithListTrace(ctx context.Context, trace *ListTrace) context.Context {
	if trace == nil {
		return ctx
	}
	return context.WithValue(ctx, listTraceContextKey{}, trace)
}

func ListTraceFromContext(ctx context.Context) *ListTrace {
	if ctx == nil {
		return nil
	}
	trace, _ := ctx.Value(listTraceContextKey{}).(*ListTrace)
	return trace
}

func StartListTraceStep(ctx context.Context, name string, fields ...ListTraceField) func(...ListTraceField) {
	trace := ListTraceFromContext(ctx)
	if trace == nil {
		return func(...ListTraceField) {}
	}
	return trace.Start(name, fields...)
}

func (t *ListTrace) Start(name string, fields ...ListTraceField) func(...ListTraceField) {
	if t == nil {
		return func(...ListTraceField) {}
	}
	startedAt := time.Now()
	return func(doneFields ...ListTraceField) {
		t.AddStep(name, time.Since(startedAt), append(fields, doneFields...)...)
	}
}

func (t *ListTrace) AddStep(name string, duration time.Duration, fields ...ListTraceField) {
	if t == nil {
		return
	}
	step := ListTraceStep{
		Name:     name,
		Duration: duration,
		Fields:   cleanListTraceFields(fields),
	}
	t.mu.Lock()
	t.steps = append(t.steps, step)
	t.mu.Unlock()
}

func (t *ListTrace) Snapshot() ListTraceSnapshot {
	if t == nil {
		return ListTraceSnapshot{}
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	steps := append([]ListTraceStep(nil), t.steps...)
	for i := range steps {
		steps[i].Fields = append([]ListTraceField(nil), steps[i].Fields...)
	}
	return ListTraceSnapshot{
		Enabled:       true,
		StartedAt:     t.startedAt,
		TotalDuration: time.Since(t.startedAt),
		Steps:         steps,
	}
}

func ListTraceString(key string, value string) ListTraceField {
	return ListTraceField{Key: key, Value: value}
}

func ListTraceInt(key string, value int) ListTraceField {
	return ListTraceField{Key: key, Value: strconv.Itoa(value)}
}

func ListTraceBool(key string, value bool) ListTraceField {
	return ListTraceField{Key: key, Value: strconv.FormatBool(value)}
}

func cleanListTraceFields(fields []ListTraceField) []ListTraceField {
	if len(fields) == 0 {
		return nil
	}
	cleaned := make([]ListTraceField, 0, len(fields))
	for _, field := range fields {
		if field.Key == "" {
			continue
		}
		cleaned = append(cleaned, field)
	}
	return cleaned
}
