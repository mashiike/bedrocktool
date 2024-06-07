package bedrocktool

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime/types"
)

// BedrockConverseAPIClient is a client for BedrockConverseAPI.
// see: https://pkg.go.dev/github.com/aws/aws-sdk-go-v2/service/bedrockruntime#Client.Converse
type BedrockConverseAPIClient interface {
	Converse(ctx context.Context, params *bedrockruntime.ConverseInput, optFns ...func(*bedrockruntime.Options)) (*bedrockruntime.ConverseOutput, error)
}

// Dispacher is a tool use dispacher. It is used to send messages to the specified Amazon Bedrock model.
type Dispacher struct {
	err               error
	mu                sync.RWMutex
	toolSet           *ToolSet
	client            BedrockConverseAPIClient
	logger            *slog.Logger
	onBeforeModelCall func(context.Context, *bedrockruntime.ConverseInput)
	onAfterModelCall  func(context.Context, *bedrockruntime.ConverseInput, *bedrockruntime.ConverseOutput)
	onBeforeToolUse   func(context.Context, *types.ContentBlockMemberToolUse)
	onAfterToolUse    func(context.Context, *types.ContentBlockMemberToolUse, types.ToolResultBlock)
	toolChoice        types.ToolChoice
}

// New creates a new instance of the Bedrock Tool Use Dispacher.
func New(options bedrockruntime.Options, optFns ...func(*bedrockruntime.Options)) *Dispacher {
	client := bedrockruntime.New(options, optFns...)
	return NewWithClient(client)
}

// NewFromConfig creates a new instance of the Bedrock Tool Use Dispacher.
func NewFromConfig(cfg aws.Config, optFns ...func(*bedrockruntime.Options)) *Dispacher {
	return NewWithClient(bedrockruntime.NewFromConfig(cfg, optFns...))
}

// NewWithClient creates a new instance of the Bedrock Tool Use Dispacher.
func NewWithClient(client BedrockConverseAPIClient) *Dispacher {
	d := &Dispacher{
		client:  client,
		logger:  slog.Default(),
		toolSet: newToolSet(),
	}
	d.SetLogger(slog.Default())
	return d
}

// SetLogger sets the logger for the dispacher.
func (d *Dispacher) SetLogger(logger *slog.Logger) {
	d.logger = logger.With("module", "github.com/mashiike/bedrocktool.Dispacher")
}

// SetToolChoice sets the tool choice for the dispacher.
func (d *Dispacher) SetToolChoice(tc types.ToolChoice) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.toolChoice = tc
}

// OnBeforeModelCall sets the function to be called before the model is called.
// Before Call Bedrock Converse API.
func (d *Dispacher) OnBeforeModelCall(f func(context.Context, *bedrockruntime.ConverseInput)) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.onBeforeModelCall = f
}

// OnAfterModelCall sets the function to be called after the model is called.
// After Call Bedrock Converse API.
func (d *Dispacher) OnAfterModelCall(f func(context.Context, *bedrockruntime.ConverseInput, *bedrockruntime.ConverseOutput)) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.onAfterModelCall = f
}

// OnBeforeToolUse sets the function to be called before the tool is used.
// Before Call Worker.Execute.
func (d *Dispacher) OnBeforeToolUse(f func(context.Context, *types.ContentBlockMemberToolUse)) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.onBeforeToolUse = f
}

// OnAfterToolUse sets the function to be called after the tool is used.
// After Call Worker.Execute.
func (d *Dispacher) OnAfterToolUse(f func(context.Context, *types.ContentBlockMemberToolUse, types.ToolResultBlock)) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.onAfterToolUse = f
}

func (d *Dispacher) handleBeforeModelCall(ctx context.Context, params *bedrockruntime.ConverseInput) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	if d.onBeforeModelCall != nil {
		d.onBeforeModelCall(ctx, params)
	}
}

func (d *Dispacher) handleAfterModelCall(ctx context.Context, params *bedrockruntime.ConverseInput, resp *bedrockruntime.ConverseOutput) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	if d.onAfterModelCall != nil {
		d.onAfterModelCall(ctx, params, resp)
	}
}

func (d *Dispacher) handleBeforeToolUse(ctx context.Context, c *types.ContentBlockMemberToolUse) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	if d.onBeforeToolUse != nil {
		d.onBeforeToolUse(ctx, c)
	}
}

func (d *Dispacher) handleAfterToolUse(ctx context.Context, c *types.ContentBlockMemberToolUse, r types.ToolResultBlock) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	if d.onAfterToolUse != nil {
		d.onAfterToolUse(ctx, c, r)
	}
}

// Converse sends messages to the specified Amazon Bedrock model.
// input same as https://pkg.go.dev/github.com/aws/aws-sdk-go-v2/service/bedrockruntime#Client.Converse
// but output is different.
// because this function is multiple call Converse API.
// if you need track api call, use OnBeforeModelCall and OnAfterModelCall.
func (d *Dispacher) Converse(ctx context.Context, params *bedrockruntime.ConverseInput, optFns ...func(*bedrockruntime.Options)) ([]types.Message, error) {
	if err := d.GetError(); err != nil {
		return nil, fmt.Errorf("registration tool error: %w", err)
	}
	if params == nil {
		return nil, errors.New("params is required")
	}
	if params.ModelId == nil {
		return nil, errors.New("model id is required")
	}
	cc := &ConverseContext{
		inputMessages:  params.Messages,
		modelId:        *params.ModelId,
		outputMessages: make([]types.Message, 0, 1),
	}
	cctx := NewContext(ctx, cc)
	inputMessages := append([]types.Message{}, params.Messages...)
	for {
		select {
		case <-cctx.Done():
			return nil, cctx.Err()
		default:
		}
		params.Messages = inputMessages
		params.ToolConfig = d.NewToolConfiguration(ctx)
		params.ModelId = aws.String(cc.ModelId())
		d.handleBeforeModelCall(cctx, params)
		resp, err := d.client.Converse(ctx, params, optFns...)
		if err != nil {
			return nil, err
		}
		d.handleAfterModelCall(cctx, params, resp)
		var currentMessage types.Message
		switch output := resp.Output.(type) {
		case *types.ConverseOutputMemberMessage:
			currentMessage = output.Value
			inputMessages = append(inputMessages, output.Value)
			cc.appendOutputMessages(output.Value)
		default:
			d.logger.WarnContext(cctx, "unexpected bedrockruntime.ConverseOutput.Output type", "type", fmt.Sprintf("%T", output))
		}
		switch resp.StopReason {
		case types.StopReasonEndTurn, types.StopReasonMaxTokens, types.StopReasonStopSequence, types.StopReasonContentFiltered:
			return cc.OutputMessages(), nil
		case types.StopReasonToolUse:
			msgs, err := d.useTool(cctx, currentMessage)
			if err != nil {
				return cc.OutputMessages(), err
			}
			cc.appendOutputMessages(msgs...)
			inputMessages = append(inputMessages, msgs...)
		default:
			d.logger.WarnContext(cctx, "unexpected stop reason", "reason", resp.StopReason)
			return cc.OutputMessages(), nil
		}
	}
}

func (d *Dispacher) useTool(ctx context.Context, msg types.Message) ([]types.Message, error) {
	var messages []types.Message
	var messageMu sync.Mutex
	var wg sync.WaitGroup
	toolUseCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	for _, c := range msg.Content {
		select {
		case <-toolUseCtx.Done():
			return nil, toolUseCtx.Err()
		default:
		}
		switch c := c.(type) {
		case *types.ContentBlockMemberToolUse:
			if c.Value.Name == nil {
				d.logger.DebugContext(ctx, "tool use content has no name", "content", fmt.Sprintf("%T", c))
				messageMu.Lock()
				messages = append(messages, types.Message{
					Role: types.ConversationRoleUser,
					Content: []types.ContentBlock{
						&types.ContentBlockMemberToolResult{
							Value: types.ToolResultBlock{
								ToolUseId: c.Value.ToolUseId,
								Content: []types.ToolResultContentBlock{
									&types.ToolResultContentBlockMemberText{
										Value: "tool name is required",
									},
								},
								Status: types.ToolResultStatusError,
							},
						},
					},
				})
				messageMu.Unlock()
				continue
			}
			worker, ok := d.Worker(*c.Value.Name)
			if !ok {
				d.logger.WarnContext(ctx, "tool not found", "name", *c.Value.Name)
				messageMu.Lock()
				messages = append(messages, types.Message{
					Role: types.ConversationRoleUser,
					Content: []types.ContentBlock{
						&types.ContentBlockMemberToolResult{
							Value: types.ToolResultBlock{
								ToolUseId: c.Value.ToolUseId,
								Content: []types.ToolResultContentBlock{
									&types.ToolResultContentBlockMemberText{
										Value: "tool not found",
									},
								},
								Status: types.ToolResultStatusError,
							},
						},
					},
				})
				messageMu.Unlock()
				continue
			}
			wg.Add(1)
			go func(c *types.ContentBlockMemberToolUse) {
				defer func() {
					if r := recover(); r != nil {
						messageMu.Lock()
						messages = append(messages, types.Message{
							Role: types.ConversationRoleUser,
							Content: []types.ContentBlock{
								&types.ContentBlockMemberToolResult{
									Value: types.ToolResultBlock{
										ToolUseId: c.Value.ToolUseId,
										Content: []types.ToolResultContentBlock{
											&types.ToolResultContentBlockMemberText{
												Value: fmt.Sprintf("panic: %v", r),
											},
										},
										Status: types.ToolResultStatusError,
									},
								},
							},
						})
						messageMu.Unlock()
					}
					wg.Done()
				}()
				d.handleBeforeToolUse(ctx, c)
				toolResultBlock, err := worker.Execute(toolUseCtx, c.Value.Input)
				if err != nil {
					toolResultBlock = types.ToolResultBlock{
						Content: []types.ToolResultContentBlock{
							&types.ToolResultContentBlockMemberText{
								Value: fmt.Sprintf("worker error: %v", err),
							},
						},
						Status: types.ToolResultStatusError,
					}
				}
				toolResultBlock.ToolUseId = c.Value.ToolUseId
				d.handleAfterToolUse(ctx, c, toolResultBlock)
				messageMu.Lock()
				messages = append(messages, types.Message{
					Role: types.ConversationRoleUser,
					Content: []types.ContentBlock{
						&types.ContentBlockMemberToolResult{Value: toolResultBlock},
					},
				})
				messageMu.Unlock()
			}(c)
		default:
			// pass
		}
	}
	wg.Wait()
	return messages, nil
}

// Worker returns the worker registered with the specified name.
func (d *Dispacher) Worker(name string) (Worker, bool) {
	return d.toolSet.Worker(name)
}

// flag of panic behavior, when the error occurred during the registration of the tool.
var NoPanicOnRegisterError = false

// GetError returns the error that occurred during the registration of the tool.
func (d *Dispacher) GetError() error {
	return d.toolSet.GetError()
}

// Register registers a tool with the specified name and description.
// if occurs error during the registration, it will panic.
// if you want to handle the error, use GetError and NoPanicOnRegisterError=false.
func (d *Dispacher) Register(name string, description string, worker Worker, opts ...RegisterOption) {
	d.toolSet.Register(name, description, worker, opts...)
}

// NewToolConfiguration returns a new ToolConfiguration.
func (d *Dispacher) NewToolConfiguration(ctx context.Context) *types.ToolConfiguration {
	d.mu.RLock()
	defer d.mu.RUnlock()
	cfg := &types.ToolConfiguration{
		Tools:      d.toolSet.Tools(ctx),
		ToolChoice: d.toolChoice,
	}
	if len(cfg.Tools) == 0 {
		return nil
	}
	return cfg
}
