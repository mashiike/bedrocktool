package bedrocktool

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime/document"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime/types"
)

type Middleware interface {
	HandleInputSchema(next Worker) document.Interface
	HandleExecute(ctx context.Context, in types.ToolUseBlock, next Worker) (types.ToolResultBlock, error)
}

type InputSchemaMiddlewareFunc func(Worker) document.Interface

func (f InputSchemaMiddlewareFunc) HandleInputSchema(next Worker) document.Interface {
	return f(next)
}

func (f InputSchemaMiddlewareFunc) HandleExecute(ctx context.Context, in types.ToolUseBlock, next Worker) (types.ToolResultBlock, error) {
	return next.Execute(ctx, in)
}

type ExecuteMiddlewareFunc func(context.Context, types.ToolUseBlock, Worker) (types.ToolResultBlock, error)

func (f ExecuteMiddlewareFunc) HandleInputSchema(next Worker) document.Interface {
	return next.InputSchema()
}

func (f ExecuteMiddlewareFunc) HandleExecute(ctx context.Context, in types.ToolUseBlock, next Worker) (types.ToolResultBlock, error) {
	return f(ctx, in, next)
}

type decoratedWorker struct {
	Next Worker
	With Middleware
}

func (w *decoratedWorker) InputSchema() document.Interface {
	return w.With.HandleInputSchema(w.Next)
}

func (w *decoratedWorker) Execute(ctx context.Context, in types.ToolUseBlock) (types.ToolResultBlock, error) {
	return w.With.HandleExecute(ctx, in, w.Next)
}
