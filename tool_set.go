package bedrocktool

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime/types"
)

type ToolSet struct {
	mu          sync.RWMutex
	subToolSets []*ToolSet
	entries     map[string]workerEntry
	middlewares []Middleware
	err         error
}

func (ts *ToolSet) SubToolSet() *ToolSet {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	sub := newToolSet()
	ts.subToolSets = append(ts.subToolSets, sub)
	return sub
}

func newToolSet() *ToolSet {
	return &ToolSet{
		entries:     make(map[string]workerEntry),
		subToolSets: make([]*ToolSet, 0),
		middlewares: make([]Middleware, 0),
	}
}

type workerEntry struct {
	tool    types.Tool
	worker  Worker
	enabler func(context.Context) bool
}

// RegisterOption is an option for registering a tool.
type RegisterOption func(*workerEntry)

// WithToolEnabler sets the function to determine whether the tool is enabled.
// this function is called before the first time Bedorck Converse API
// If return false, not enabled the tool in this conversation.
func WithToolEnabler(f func(context.Context) bool) RegisterOption {
	return func(e *workerEntry) {
		e.enabler = f
	}
}

func (ts *ToolSet) Use(middlewares ...Middleware) {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	ts.middlewares = append(ts.middlewares, middlewares...)
}

func (ts *ToolSet) Register(name string, description string, worker Worker, opts ...RegisterOption) {
	if err := ts.register(name, description, worker, opts...); err != nil {
		if !NoPanicOnRegisterError {
			panic(fmt.Errorf("bedrock tool: %w", err))
		}
	}
}

func (ts *ToolSet) register(name string, description string, worker Worker, opts ...RegisterOption) error {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	if name == "" {
		ts.err = errors.New("tool name is required")
		return ts.err
	}
	if worker == nil {
		ts.err = errors.New("worker is required")
		return ts.err
	}
	for _, sub := range ts.subToolSets {
		if _, ok := sub.Worker(name); ok {
			ts.err = fmt.Errorf("multiple registrations for tool %s", name)
			return ts.err
		}
	}
	if _, ok := ts.entries[name]; ok {
		ts.err = fmt.Errorf("multiple registrations for tool %s", name)
		return ts.err
	}
	if ts.entries == nil {
		ts.entries = make(map[string]workerEntry)
	}
	var desc *string
	if description != "" {
		desc = &description
	}
	entry := workerEntry{
		tool: &types.ToolMemberToolSpec{
			Value: types.ToolSpecification{
				Name:        &name,
				Description: desc,
				InputSchema: &types.ToolInputSchemaMemberJson{
					Value: worker.InputSchema(),
				},
			},
		},
		worker: worker,
	}
	for _, opt := range opts {
		opt(&entry)
	}
	ts.entries[name] = entry
	return nil
}

func (ts *ToolSet) Exists(name string) bool {
	ts.mu.RLock()
	defer ts.mu.RUnlock()
	if _, ok := ts.entries[name]; ok {
		return true
	}
	for _, sub := range ts.subToolSets {
		if sub.Exists(name) {
			return true
		}
	}
	return false
}

func (ts *ToolSet) worker(name string) (Worker, bool) {
	entry, ok := ts.entries[name]
	if ok {

		return entry.worker, true
	}
	for _, sub := range ts.subToolSets {
		worker, ok := sub.Worker(name)
		if ok {
			return worker, true
		}
	}
	return nil, false
}

func (ts *ToolSet) Worker(name string) (Worker, bool) {
	w, ok := ts.worker(name)
	if !ok {
		return nil, false
	}
	for _, middleware := range ts.middlewares {
		w = &decoratedWorker{
			Next: w,
			With: middleware,
		}
	}
	return w, true
}

func (ts *ToolSet) GetError() error {
	ts.mu.RLock()
	defer ts.mu.RUnlock()
	if ts.err != nil {
		return ts.err
	}
	for _, sub := range ts.subToolSets {
		if err := sub.GetError(); err != nil {
			return err
		}
	}
	return nil
}

func (ts *ToolSet) Tools(ctx context.Context) []types.Tool {
	tools := make([]types.Tool, 0, len(ts.subToolSets))
	for _, entry := range ts.entries {
		if entry.enabler != nil && !entry.enabler(ctx) {
			continue
		}
		tools = append(tools, entry.tool)
	}
	for _, sub := range ts.subToolSets {
		tools = append(tools, sub.Tools(ctx)...)
	}
	return tools
}
