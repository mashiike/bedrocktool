package urlreader_test

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime/document"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime/types"
	"github.com/mashiike/bedrocktool/tool/urlreader"
	"github.com/stretchr/testify/require"
)

func TestUrlSummarizerWithAWS(t *testing.T) {
	if !strings.EqualFold(os.Getenv("TEST_WITH_AWS"), "true") {
		t.Skip("set TEST_WITH_AWS to run this test")
	}
	ctx := context.Background()
	cfg, err := config.LoadDefaultConfig(ctx)
	require.NoError(t, err)
	tool := urlreader.NewFromConfig(cfg)
	result, err := tool.Execute(ctx, types.ToolUseBlock{
		Input: document.NewLazyDocument(map[string]interface{}{
			"url":      "https://aws.amazon.com/jp/what-is-aws/",
			"context":  "What is this page?",
			"language": "english",
		}),
		Name:      aws.String(urlreader.ToolName),
		ToolUseId: aws.String("test"),
	})
	require.NoError(t, err)
	require.Len(t, result.Content, 1)
	for _, content := range result.Content {
		switch c := content.(type) {
		case *types.ToolResultContentBlockMemberText:
			t.Logf("summary: %s", c.Value)
		default:
			require.Fail(t, "unexpected content type %T", content)
		}
	}
}
