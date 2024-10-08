package bedrocktool

import (
	"context"
	"errors"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Songmu/flextime"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime/document"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime/types"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type ClockWorkerInput struct {
	Location string `json:"location,omitempty" jsonschema:"default=Real"`
}

var ToolDescription = `Return current time in RFC3339 format
If the location is different, please call the tool individually. Please do not infer this as it cannot be absorbed by calculations such as time difference.
If you are asked for two or more locations, please call the tool for each location at the same time to reduce time lag.
`

type testDispacherConverseCase struct {
	client BedrockConverseAPIClient
	temp   bool
	mssage string
}

func (tc testDispacherConverseCase) Run(tb testing.TB) {
	require.NotNil(tb, tc.client)
	if tc.mssage == "" {
		tc.mssage = "What time is it now?"
	}
	cleanup := flextime.Set(time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC))
	defer cleanup()
	d := NewWithClient(tc.client)
	var isUse atomic.Bool
	var totalInputTokens, totalOutputTokens atomic.Int64
	d.OnAfterModelCall(func(_ context.Context, _ *bedrockruntime.ConverseInput, output *bedrockruntime.ConverseOutput) {
		if output.Usage != nil {
			tb.Logf("after model call: input tokens: %d, output tokens: %d", *output.Usage.InputTokens, *output.Usage.OutputTokens)
			totalInputTokens.Add(int64(*output.Usage.InputTokens))
			totalOutputTokens.Add(int64(*output.Usage.OutputTokens))
		}
	})
	ctx := context.Background()
	worker := NewWorker(func(ctx context.Context, input ClockWorkerInput) (types.ToolResultBlock, error) {
		isUse.Store(true)
		now := flextime.Now()
		return types.ToolResultBlock{
			Content: []types.ToolResultContentBlock{
				&types.ToolResultContentBlockMemberText{
					Value: now.Format(time.RFC3339),
				},
			},
		}, nil
	})
	if tc.temp {
		var ts *ToolSet
		ctx, ts = WithToolSet(ctx)
		ts.Register(
			"clock",
			ToolDescription,
			worker,
		)
	} else {
		d.Register(
			"clock",
			ToolDescription,
			worker,
		)
	}
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	output, err := d.Converse(ctx, &bedrockruntime.ConverseInput{
		ModelId: aws.String("anthropic.claude-3-haiku-20240307-v1:0"),
		Messages: []types.Message{
			{
				Role: types.ConversationRoleUser,
				Content: []types.ContentBlock{
					&types.ContentBlockMemberText{
						Value: tc.mssage,
					},
				},
			},
		},
	})
	require.NoError(tb, err)
	require.True(tb, isUse.Load())
	tb.Log("total input tokens:", totalInputTokens.Load())
	require.Greater(tb, totalInputTokens.Load(), int64(1))
	tb.Log("total output tokens:", totalOutputTokens.Load())
	require.Greater(tb, totalOutputTokens.Load(), int64(1))

	for _, msg := range output {
		for _, content := range msg.Content {
			switch c := content.(type) {
			case *types.ContentBlockMemberText:
				tb.Logf("[%s]: %s", msg.Role, c.Value)
			case *types.ContentBlockMemberImage:
				tb.Logf("[%s]: <image %s>", msg.Role, c.Value.Format)
			case *types.ContentBlockMemberToolResult:
				tb.Logf("[%s]: <tool result %s>", msg.Role, *c.Value.ToolUseId)
			case *types.ContentBlockMemberToolUse:
				tb.Logf("[%s]: <tool use %s>", msg.Role, *c.Value.ToolUseId)
			default:
				require.Fail(tb, "unexpected content type %T", content)
			}
		}
	}
}

func TestDispacherConverseWithAWS(t *testing.T) {
	if !strings.EqualFold(os.Getenv("TEST_WITH_AWS"), "true") {
		t.Skip("set TEST_WITH_AWS to run this test")
	}
	ctx := context.Background()
	cfg, err := config.LoadDefaultConfig(ctx)
	require.NoError(t, err)
	client := bedrockruntime.NewFromConfig(cfg)
	tc := testDispacherConverseCase{
		client: client,
	}
	tc.Run(t)
}

func TestDispacherConverseWithAWS__WithLocation(t *testing.T) {
	if !strings.EqualFold(os.Getenv("TEST_WITH_AWS"), "true") {
		t.Skip("set TEST_WITH_AWS to run this test")
	}
	ctx := context.Background()
	cfg, err := config.LoadDefaultConfig(ctx)
	require.NoError(t, err)
	client := bedrockruntime.NewFromConfig(cfg)
	tc := testDispacherConverseCase{
		client: client,
		mssage: "What time is it now in Location1 ~ Location10?",
	}
	tc.Run(t)
}

type mockClient struct {
	mock.Mock
	tb testing.TB
}

func (m *mockClient) Converse(ctx context.Context, params *bedrockruntime.ConverseInput, _ ...func(*bedrockruntime.Options)) (*bedrockruntime.ConverseOutput, error) {
	args := m.Called(ctx, params)
	output := args.Get(0)
	if output == nil {
		return nil, args.Error(1)
	}
	if o, ok := output.(*bedrockruntime.ConverseOutput); ok {
		return o, args.Error(1)
	}
	m.tb.Fatalf("unexpected output type %T", output)
	return nil, errors.New("unexpected output type")
}

func newMockClient(tb testing.TB) *mockClient {
	return &mockClient{
		tb: tb,
	}
}

func TestDispacherConverseWithMock(t *testing.T) {
	testDispacherConverseWithMock(t, false)
}

func TestDispacherConverseWithMock__WithTemp(t *testing.T) {
	testDispacherConverseWithMock(t, true)
}

func BenchmarkDispacherConverseWithMock(b *testing.B) {
	b.ReportAllocs()
	b.ReportMetric(0, "ns/op")

	for i := 0; i < b.N; i++ {
		testDispacherConverseWithMock(b, false)
	}
}

func testDispacherConverseWithMock(tb testing.TB, temp bool) {
	client := newMockClient(tb)
	defer client.AssertExpectations(tb)
	//1st-time
	client.On("Converse", mock.Anything, &bedrockruntime.ConverseInput{
		ModelId: aws.String("anthropic.claude-3-haiku-20240307-v1:0"),
		Messages: []types.Message{
			{
				Role: types.ConversationRoleUser,
				Content: []types.ContentBlock{
					&types.ContentBlockMemberText{
						Value: "What time is it now?",
					},
				},
			},
		},
		ToolConfig: &types.ToolConfiguration{
			Tools: []types.Tool{
				&types.ToolMemberToolSpec{
					Value: types.ToolSpecification{
						Name:        aws.String("clock"),
						Description: aws.String(ToolDescription),
						InputSchema: &types.ToolInputSchemaMemberJson{
							Value: document.NewLazyDocument(map[string]interface{}{
								"$id":  "https://github.com/mashiike/bedrocktool/clock-worker-input",
								"type": "object",
								"properties": map[string]interface{}{
									"location": map[string]interface{}{
										"default": "Real",
										"type":    "string",
									},
								},
								"additionalProperties": false,
							}),
						},
					},
				},
			},
		},
	}).Return(&bedrockruntime.ConverseOutput{
		Metrics: &types.ConverseMetrics{
			LatencyMs: aws.Int64(1000),
		},
		Output: &types.ConverseOutputMemberMessage{
			Value: types.Message{
				Role: types.ConversationRoleAssistant,
				Content: []types.ContentBlock{
					&types.ContentBlockMemberToolUse{
						Value: types.ToolUseBlock{
							Name:      aws.String("clock"),
							ToolUseId: aws.String("tooluse_********"),
							Input:     document.NewLazyDocument(map[string]interface{}{"location": "Real"}),
						},
					},
				},
			},
		},
		StopReason: types.StopReasonToolUse,
		Usage: &types.TokenUsage{
			InputTokens:  aws.Int32(1),
			OutputTokens: aws.Int32(1),
		},
	}, nil)
	//2nd-time
	client.On("Converse", mock.Anything, &bedrockruntime.ConverseInput{
		ModelId: aws.String("anthropic.claude-3-haiku-20240307-v1:0"),
		Messages: []types.Message{
			{
				Role: types.ConversationRoleUser,
				Content: []types.ContentBlock{
					&types.ContentBlockMemberText{
						Value: "What time is it now?",
					},
				},
			},
			{
				Role: types.ConversationRoleAssistant,
				Content: []types.ContentBlock{
					&types.ContentBlockMemberToolUse{
						Value: types.ToolUseBlock{
							Name:      aws.String("clock"),
							ToolUseId: aws.String("tooluse_********"),
							Input:     document.NewLazyDocument(map[string]interface{}{"location": "Real"}),
						},
					},
				},
			},
			{
				Role: types.ConversationRoleUser,
				Content: []types.ContentBlock{
					&types.ContentBlockMemberToolResult{
						Value: types.ToolResultBlock{
							ToolUseId: aws.String("tooluse_********"),
							Content: []types.ToolResultContentBlock{
								&types.ToolResultContentBlockMemberText{
									Value: "2020-01-01T00:00:00Z",
								},
							},
						},
					},
				},
			},
		},
		ToolConfig: &types.ToolConfiguration{
			Tools: []types.Tool{
				&types.ToolMemberToolSpec{
					Value: types.ToolSpecification{
						Name:        aws.String("clock"),
						Description: aws.String(ToolDescription),
						InputSchema: &types.ToolInputSchemaMemberJson{
							Value: document.NewLazyDocument(map[string]interface{}{
								"$id":  "https://github.com/mashiike/bedrocktool/clock-worker-input",
								"type": "object",
								"properties": map[string]interface{}{
									"location": map[string]interface{}{
										"default": "Real",
										"type":    "string",
									},
								},
								"additionalProperties": false,
							}),
						},
					},
				},
			},
		},
	}).Return(&bedrockruntime.ConverseOutput{
		Metrics: &types.ConverseMetrics{
			LatencyMs: aws.Int64(1000),
		},
		Output: &types.ConverseOutputMemberMessage{
			Value: types.Message{
				Role: types.ConversationRoleAssistant,
				Content: []types.ContentBlock{
					&types.ContentBlockMemberText{
						Value: "According to the clock function, the current time is 2020-01-01T00:00:01Z",
					},
				},
			},
		},
		StopReason: types.StopReasonEndTurn,
		Usage: &types.TokenUsage{
			InputTokens:  aws.Int32(1),
			OutputTokens: aws.Int32(1),
		},
	}, nil)
	tc := testDispacherConverseCase{
		client: client,
		temp:   temp,
	}
	tc.Run(tb)
}

func TestTemporaryToolSet(t *testing.T) {
	ctx := context.Background()
	client := newMockClient(t)
	defer client.AssertExpectations(t)
	d := NewWithClient(client)
	clockWorkerExpected := NewWorker(func(ctx context.Context, _ EmptyWorkerInput) (types.ToolResultBlock, error) {
		return types.ToolResultBlock{
			Content: []types.ToolResultContentBlock{
				&types.ToolResultContentBlockMemberText{
					Value: flextime.Now().Format(time.RFC3339),
				},
			},
		}, nil
	})
	d.Register(
		"clock",
		ToolDescription,
		clockWorkerExpected,
	)
	ctxWithTs, ts := WithToolSet(ctx)
	clockInJSTWorkerExpected := NewWorker(func(ctx context.Context, _ EmptyWorkerInput) (types.ToolResultBlock, error) {
		return types.ToolResultBlock{
			Content: []types.ToolResultContentBlock{
				&types.ToolResultContentBlockMemberText{
					Value: flextime.Now().In(time.FixedZone("JST", 9*60*60)).Format(time.RFC3339),
				},
			},
		}, nil
	})
	ts.Register(
		"clock_in_jst",
		"Return current time in RFC3339 format in JST",
		clockInJSTWorkerExpected,
	)
	toolConfig := d.NewToolConfiguration(ctxWithTs)
	require.Len(t, toolConfig.Tools, 2)
	toolNames := make([]string, 0, len(toolConfig.Tools))
	for _, tool := range toolConfig.Tools {
		if t, ok := tool.(*types.ToolMemberToolSpec); ok {
			toolNames = append(toolNames, *t.Value.Name)
		}
	}
	require.ElementsMatch(t, []string{"clock", "clock_in_jst"}, toolNames)
	clockWorkerActual, ok := d.ResolveWorker(ctxWithTs, "clock")
	require.True(t, ok)
	require.Same(t, clockWorkerExpected, clockWorkerActual)
	clockInJSTWorkerActual, ok := d.ResolveWorker(ctxWithTs, "clock_in_jst")
	require.True(t, ok)
	require.Same(t, clockInJSTWorkerExpected, clockInJSTWorkerActual)
}
