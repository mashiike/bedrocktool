package bedrocktool

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime/document"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime/types"
	"github.com/invopop/jsonschema"
)

type Worker interface {
	InputSchema() document.Interface
	Execute(context.Context, document.Interface) (types.ToolResultBlock, error)
}

type reflectWorker struct {
	inputSchema document.Interface
	execFunc    func(context.Context, document.Interface) (types.ToolResultBlock, error)
}

func (w *reflectWorker) InputSchema() document.Interface {
	return w.inputSchema
}

func (w *reflectWorker) Execute(ctx context.Context, input document.Interface) (types.ToolResultBlock, error) {
	return w.execFunc(ctx, input)
}

type EmptyWorkerInput struct{}

func NewWorker[T any](f func(context.Context, T) (types.ToolResultBlock, error)) Worker {
	var v T
	r := jsonschema.Reflector{
		DoNotReference: true,
		ExpandedStruct: true,
	}
	schema := r.Reflect(v)
	bs, err := json.Marshal(schema)
	if err != nil {
		panic(fmt.Errorf("bedrock tool: failed to marshal schema: %w", err))
	}
	var m map[string]interface{}
	if err := json.Unmarshal(bs, &m); err != nil {
		panic(fmt.Errorf("bedrock tool: failed to unmarshal schema: %w", err))
	}
	delete(m, "$schema")
	return &reflectWorker{
		inputSchema: document.NewLazyDocument(m),
		execFunc: func(ctx context.Context, input document.Interface) (types.ToolResultBlock, error) {
			var value T
			if err := input.UnmarshalSmithyDocument(&value); err != nil {
				return types.ToolResultBlock{}, err
			}
			return f(ctx, value)
		},
	}
}
