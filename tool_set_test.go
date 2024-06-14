package bedrocktool

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime/document"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime/types"
	"github.com/stretchr/testify/require"
)

func TestToolSetUse(t *testing.T) {
	var called atomic.Int64
	var subCalled atomic.Int64
	ts := newToolSet()
	ts.Use(
		ExecuteMiddlewareFunc(func(ctx context.Context, in types.ToolUseBlock, next Worker) (types.ToolResultBlock, error) {
			called.Add(1)
			return next.Execute(ctx, in)
		}),
	)
	sub := ts.SubToolSet()
	sub.Use(
		ExecuteMiddlewareFunc(func(ctx context.Context, in types.ToolUseBlock, next Worker) (types.ToolResultBlock, error) {
			subCalled.Add(1)
			return next.Execute(ctx, in)
		}),
	)
	sub.Register("test", "", NewWorker(func(context.Context, EmptyWorkerInput) (types.ToolResultBlock, error) {
		return types.ToolResultBlock{}, nil
	}))
	w, ok := ts.Worker("test")
	require.True(t, ok)
	w.Execute(context.Background(), types.ToolUseBlock{
		Name:      aws.String("test"),
		Input:     document.NewLazyDocument(nil),
		ToolUseId: aws.String("test"),
	})
	require.EqualValues(t, 1, called.Load())
	require.EqualValues(t, 1, subCalled.Load())
}
