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
	Execute(context.Context, types.ToolUseBlock) (types.ToolResultBlock, error)
}

type reflectWorker struct {
	inputSchema document.Interface
	execFunc    func(context.Context, types.ToolUseBlock) (types.ToolResultBlock, error)
}

func (w *reflectWorker) InputSchema() document.Interface {
	return w.inputSchema
}

func (w *reflectWorker) Execute(ctx context.Context, toolUse types.ToolUseBlock) (types.ToolResultBlock, error) {
	return w.execFunc(ctx, toolUse)
}

type EmptyWorkerInput struct{}

func MarshalJSONSchema(v any) ([]byte, error) {
	r := jsonschema.Reflector{
		DoNotReference: true,
		ExpandedStruct: true,
	}
	schema := r.Reflect(v)
	bs, err := json.Marshal(schema)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal schema: %w", err)
	}
	return bs, nil
}

func GenerateInputSchemaDocument[T any]() (document.Interface, error) {
	var v T
	bs, err := MarshalJSONSchema(v)
	if err != nil {
		return nil, fmt.Errorf("failed to generate schema: %w", err)
	}
	var m map[string]interface{}
	if err := json.Unmarshal(bs, &m); err != nil {
		return nil, fmt.Errorf("failed to unmarshal schema: %w", err)
	}
	delete(m, "$schema")
	return document.NewLazyDocument(m), nil
}

func NewWorker[T any](f func(context.Context, T) (types.ToolResultBlock, error)) Worker {
	inputSchema, err := GenerateInputSchemaDocument[T]()
	if err != nil {
		panic(fmt.Errorf("bedrock tool: %w", err))
	}
	return &reflectWorker{
		inputSchema: inputSchema,
		execFunc: func(ctx context.Context, toolUse types.ToolUseBlock) (types.ToolResultBlock, error) {
			ctx = withToolName(ctx, *toolUse.Name)
			ctx = withToolUseID(ctx, *toolUse.ToolUseId)
			var value T
			bs, err := toolUse.Input.MarshalSmithyDocument()
			if err != nil {
				return types.ToolResultBlock{}, err
			}
			if err := json.Unmarshal(bs, &value); err != nil {
				return types.ToolResultBlock{}, err
			}
			return f(ctx, value)
		},
	}
}
