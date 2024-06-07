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
	modelId        string
}

func (cc *ConverseContext) ModelId() string {
	cc.mu.RLock()
	defer cc.mu.RUnlock()
	return cc.modelId
}

// SetModelId sets the model ID for the conversation.
// for in tool use. model id upgrade/downgrade.
func (cc *ConverseContext) SetModelId(modelId string) {
	cc.mu.Lock()
	defer cc.mu.Unlock()
	cc.modelId = modelId
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

type key struct{}

var contextKey = &key{}

func NewContext(parent context.Context, cc *ConverseContext) context.Context {
	return context.WithValue(parent, contextKey, cc)
}

func FromContext(ctx context.Context) (*ConverseContext, bool) {
	cc, ok := ctx.Value(contextKey).(*ConverseContext)
	return cc, ok
}

func withToolName(ctx context.Context, name string) context.Context {
	return context.WithValue(ctx, contextKey, name)
}

func withToolUseID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, contextKey, id)
}

func ToolName(ctx context.Context) string {
	name, _ := ctx.Value(contextKey).(string)
	return name
}

func ToolUseID(ctx context.Context) string {
	id, _ := ctx.Value(contextKey).(string)
	return id
}
