package urlreader

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime/document"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime/types"
	"github.com/mashiike/bedrocktool"
)

type ToolInput struct {
	URL string `json:"url" jsonschema:"format=uri,required=true,description=read content from this URL."`
}

type Fetcher interface {
	Fetch(ctx context.Context, u *url.URL) (<-chan string, error)
}

type FetcherFunc func(ctx context.Context, u *url.URL) (<-chan string, error)

func (f FetcherFunc) Fetch(ctx context.Context, u *url.URL) (<-chan string, error) {
	return f(ctx, u)
}

// New creates a new instance of the URL Reader Tool.
func New(options bedrockruntime.Options, optFns ...func(*bedrockruntime.Options)) *Worker {
	client := bedrockruntime.New(options, optFns...)
	return NewWithClient(client)
}

// NewFromConfig creates a new instance of the URL Reader Tool.
func NewFromConfig(cfg aws.Config, optFns ...func(*bedrockruntime.Options)) *Worker {
	return NewWithClient(bedrockruntime.NewFromConfig(cfg, optFns...))
}

// NewWithClient creates a new instance of the URL Reader Tool.
func NewWithClient(client bedrocktool.BedrockConverseAPIClient) *Worker {
	w := &Worker{
		logger: slog.Default().With("tool", "urlReader"),
		client: client,
	}
	w.worker = bedrocktool.NewWorker(w.execute)
	w.SetFetcher(nil)
	return w
}

const (
	ToolName        = "url_reader"
	ToolDescription = "Read a URL, can not read the file content, only web content."
)

func Register(r bedrocktool.Registory, options bedrockruntime.Options, optFns ...func(*bedrockruntime.Options)) {
	New(options, optFns...).RegisterTo(r)
}

func RegisterFromConfig(r bedrocktool.Registory, cfg aws.Config, optFns ...func(*bedrockruntime.Options)) {
	NewFromConfig(cfg, optFns...).RegisterTo(r)
}

func RegisterWithClient(r bedrocktool.Registory, client bedrocktool.BedrockConverseAPIClient) {
	NewWithClient(client).RegisterTo(r)
}

type Worker struct {
	worker  bedrocktool.Worker
	logger  *slog.Logger
	fetcher Fetcher
	client  bedrocktool.BedrockConverseAPIClient
}

func (w *Worker) RegisterTo(r bedrocktool.Registory) {
	r.Register(ToolName, ToolDescription, w)
}

func (w *Worker) InputSchema() document.Interface {
	return w.worker.InputSchema()
}

func (w *Worker) Execute(ctx context.Context, toolUse types.ToolUseBlock) (types.ToolResultBlock, error) {
	return w.worker.Execute(ctx, toolUse)
}

func (w *Worker) Fetcher() Fetcher {
	return w.fetcher
}

func (w *Worker) SetFetcher(f Fetcher) {
	if f == nil {
		f = FetcherFunc(func(ctx context.Context, u *url.URL) (<-chan string, error) {
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
			if err != nil {
				return nil, fmt.Errorf("failed to create request; %w", err)
			}
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				return nil, fmt.Errorf("failed to fetch url; %w", err)
			}
			if resp.StatusCode >= 300 {
				return nil, fmt.Errorf("failed to fetch url; %s", resp.Status)
			}
			defer resp.Body.Close()
			bs, err := io.ReadAll(resp.Body)
			if err != nil {
				return nil, fmt.Errorf("failed to read response body; %w", err)
			}
			ret := make(chan string, 1)
			ret <- string(bs)
			close(ret)
			return ret, nil
		})
	}
	w.fetcher = f
}

func (w *Worker) execute(ctx context.Context, input ToolInput) (types.ToolResultBlock, error) {
	w.logger.DebugContext(ctx, "call url reader tool", "url", input.URL)
	u, err := url.Parse(input.URL)
	if err != nil {
		return types.ToolResultBlock{
			Content: []types.ToolResultContentBlock{
				&types.ToolResultContentBlockMemberText{
					Value: fmt.Sprintf("failed to parse url; %s", err),
				},
			},
			Status: types.ToolResultStatusError,
		}, nil
	}
	contents, err := w.fetcher.Fetch(ctx, u)
	if err != nil {
		return types.ToolResultBlock{
			Content: []types.ToolResultContentBlock{
				&types.ToolResultContentBlockMemberText{
					Value: fmt.Sprintf("failed to fetch url content; %s", err),
				},
			},
			Status: types.ToolResultStatusError,
		}, nil
	}
	var builder strings.Builder
	for content := range contents {
		select {
		case <-ctx.Done():
			return types.ToolResultBlock{
				Status: types.ToolResultStatusError,
			}, ctx.Err()
		default:
		}

		if err := w.extructContent(ctx, input, content, &builder); err != nil {
			return types.ToolResultBlock{
				Content: []types.ToolResultContentBlock{
					&types.ToolResultContentBlockMemberText{
						Value: fmt.Sprintf("failed to read content; %s", err),
					},
				},
				Status: types.ToolResultStatusError,
			}, nil
		}
	}
	return types.ToolResultBlock{
		Content: []types.ToolResultContentBlock{
			&types.ToolResultContentBlockMemberText{
				Value: builder.String(),
			},
		},
	}, nil
}

var ModelID = "anthropic.claude-3-haiku-20240307-v1:0"

const (
	systemPromptTemplate = `
You are a sophisticated AI to retrieve web content.
Your job is to retrieve the body content from the given HTML.
 Please retrieve the body of the content while preserving the original meaning and context.
Please follow the guidelines below when retrieving.
<guidelines>
1. remove HTML tags.
2. remove any elements that appear to be sidebars, navigation bars, etc.
3. maintain the original content of the text, without guesswork or completion.
</guidelines>
Here is a portion of the HTML content for a site called "%s". Put the body content inside <output></output> XML tags.
`
)

func (w *Worker) extructContent(ctx context.Context, input ToolInput, content string, builder *strings.Builder) error {
	output, err := w.client.Converse(ctx, &bedrockruntime.ConverseInput{
		ModelId: aws.String(ModelID),
		System: []types.SystemContentBlock{
			&types.SystemContentBlockMemberText{
				Value: fmt.Sprintf(systemPromptTemplate, input.URL),
			},
		},
		Messages: []types.Message{
			{
				Role: types.ConversationRoleUser,
				Content: []types.ContentBlock{
					&types.ContentBlockMemberText{
						Value: content,
					},
				},
			},
			{
				Role: types.ConversationRoleAssistant,
				Content: []types.ContentBlock{
					&types.ContentBlockMemberText{
						Value: "<output>",
					},
				},
			},
		},
		InferenceConfig: &types.InferenceConfiguration{
			MaxTokens:     aws.Int32(3000),
			StopSequences: []string{"</output>"},
		},
	})
	if err != nil {
		return fmt.Errorf("failed to converse; %w", err)
	}
	msg, ok := output.Output.(*types.ConverseOutputMemberMessage)
	if !ok {
		return fmt.Errorf("unexpected output type; %T", output.Output)
	}
	for _, content := range msg.Value.Content {
		switch c := content.(type) {
		case *types.ContentBlockMemberText:
			builder.WriteString(c.Value)
		}
	}
	return nil
}
