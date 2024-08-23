package remote

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/Songmu/flextime"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime/document"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime/types"
	"github.com/mashiike/bedrocktool"
	"github.com/stretchr/testify/require"
)

func TestRemoteTool(t *testing.T) {
	var h http.Handler
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Logf("request: %s %s", r.Method, r.URL)
		h.ServeHTTP(w, r)
	}))
	defer server.Close()
	u, err := url.Parse(server.URL)
	require.NoError(t, err)
	h, err = NewHandler(HandlerConfig{
		Endpoint:        u,
		WorkerPath:      "/worker/execute",
		ToolName:        "weather",
		ToolDescription: "return weather",
		Worker: bedrocktool.NewWorker(func(ctx context.Context, input weatherInput) (types.ToolResultBlock, error) {
			require.EqualValues(t, "東京", input.City)
			require.EqualValues(t, "2022-01-01T00:00:00Z", input.When)
			require.EqualValues(t, "weather", bedrocktool.ToolName(ctx))
			require.EqualValues(t, "test", bedrocktool.ToolUseID(ctx))
			return types.ToolResultBlock{
				Content: []types.ToolResultContentBlock{
					&types.ToolResultContentBlockMemberText{
						Value: "sunny",
					},
				},
				Status: types.ToolResultStatusSuccess,
			}, nil
		}),
	})
	require.NoError(t, err)

	ctx := context.Background()
	tool, err := NewTool(ctx, ToolConfig{
		Endpoint: server.URL,
	})
	require.NoError(t, err)
	require.Equal(t, "weather", tool.Name())
	require.Equal(t, "return weather", tool.Description())
	result, err := tool.Worker().Execute(ctx, types.ToolUseBlock{
		Name:      aws.String("weather"),
		Input:     document.NewLazyDocument(weatherInput{City: "東京", When: "2022-01-01T00:00:00Z"}),
		ToolUseId: aws.String("test"),
	})
	require.NoError(t, err)
	require.Equal(t, types.ToolResultStatusSuccess, result.Status)
	require.Len(t, result.Content, 1)
	require.Equal(t, "sunny", result.Content[0].(*types.ToolResultContentBlockMemberText).Value)
}

func TestSpecificationCache(t *testing.T) {
	now := time.Now()
	restore := flextime.Fix(now.AddDate(0, 0, -1))
	defer restore()
	cache := NewSpecificationCache(1 * time.Hour)

	spec := Specification{Name: "test"}

	// Set the specification in the cache
	cache.Set("https://example.com/", spec)

	// Get the specification from the cache
	retrievedSpec, ok := cache.Get("https://example.com/")
	require.True(t, ok)
	require.Equal(t, spec, retrievedSpec)

	flextime.Fix(now)

	// Try to get the specification from the cache after expiration
	_, ok = cache.Get("https://example.com/")
	require.False(t, ok)
	// Set the specification in the cache again
	cache.Set("http://www.example.com/", spec)

	// Delete the specification from the cache
	cache.Delete("http://www.example.com/")

	// Try to get the specification from the cache after deletion
	_, ok = cache.Get("http://www.example.com/")
	require.False(t, ok)
}
