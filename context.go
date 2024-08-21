package bedrocktool

import (
	"context"
	"sync"

	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime/types"
)

type ConverseContext struct {
	mu sync.RWMutex

	inputMessages  []types.Message
	system         []types.SystemContentBlock
	outputMessages []types.Message
	modelID        string
}

func (cc *ConverseContext) ModelID() string {
	cc.mu.RLock()
	defer cc.mu.RUnlock()
	return cc.modelID
}

// SetModelID sets the model ID for the conversation.
// for in tool use. model id upgrade/downgrade.
func (cc *ConverseContext) SetModelID(modelID string) {
	cc.mu.Lock()
	defer cc.mu.Unlock()
	cc.modelID = modelID
}

func (cc *ConverseContext) appendOutputMessages(msgs ...types.Message) {
	cc.mu.Lock()
	defer cc.mu.Unlock()
	cc.outputMessages = append(cc.outputMessages, msgs...)
}

func (cc *ConverseContext) InputMessages() []types.Message {
	cc.mu.RLock()
	defer cc.mu.RUnlock()
	cloned := make([]types.Message, len(cc.inputMessages))
	copy(cloned, cc.inputMessages)
	return cloned
}

func (cc *ConverseContext) OutputMessages() []types.Message {
	cc.mu.RLock()
	defer cc.mu.RUnlock()
	cloned := make([]types.Message, len(cc.outputMessages))
	copy(cloned, cc.outputMessages)
	return cloned
}

func (cc *ConverseContext) System() []types.SystemContentBlock {
	cc.mu.RLock()
	defer cc.mu.RUnlock()
	cloned := make([]types.SystemContentBlock, len(cc.system))
	copy(cloned, cc.system)
	return cloned
}

type key string

var (
	converseContextKey         = key("converseContext")
	toolNameContextKey         = key("toolName")
	toolUseIDContextKey        = key("toolUseID")
	temporaryToolSetContextKey = key("temporaryToolSet")
)

func NewContext(parent context.Context, cc *ConverseContext) context.Context {
	return context.WithValue(parent, converseContextKey, cc)
}

func FromContext(ctx context.Context) (*ConverseContext, bool) {
	cc, ok := ctx.Value(converseContextKey).(*ConverseContext)
	return cc, ok
}

func withToolName(ctx context.Context, name string) context.Context {
	return context.WithValue(ctx, toolNameContextKey, name)
}

func withToolUseID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, toolUseIDContextKey, id)
}

func ToolName(ctx context.Context) string {
	name, ok := ctx.Value(toolNameContextKey).(string)
	if !ok {
		return ""
	}
	return name
}

func ToolUseID(ctx context.Context) string {
	id, ok := ctx.Value(toolUseIDContextKey).(string)
	if !ok {
		return ""
	}
	return id
}

func withTemporaryToolSet(ctx context.Context, ts *ToolSet) context.Context {
	return context.WithValue(ctx, temporaryToolSetContextKey, ts)
}

func temporaryToolSetFromContext(ctx context.Context) (*ToolSet, bool) {
	ts, ok := ctx.Value(temporaryToolSetContextKey).(*ToolSet)
	return ts, ok
}

func WithToolSet(ctx context.Context) (context.Context, *ToolSet) {
	ts, ok := temporaryToolSetFromContext(ctx)
	if !ok {
		ts = newToolSet()
	} else {
		ts = ts.clone()
	}
	return withTemporaryToolSet(ctx, ts), ts
}
