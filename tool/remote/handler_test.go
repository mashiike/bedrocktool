package remote

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime/types"
	"github.com/mashiike/bedrocktool"
	"github.com/stretchr/testify/require"
)

type weatherInput struct {
	City string `json:"city" jsonschema:"description=都市名 (例: 横浜,東京),default=東京, required=true"`
	When string `json:"when" jsonschema:"description=日時 RFC3339 (例: 2022-01-01T00:00:00Z), required=false"`
}

func TestHandler(t *testing.T) {
	h, err := NewHandler(HandlerConfig{
		WorkerPath:      "/worker/execute",
		ToolName:        "weather",
		ToolDescription: "return weather",
		Worker: bedrocktool.NewWorker(func(ctx context.Context, input weatherInput) (types.ToolResultBlock, error) {
			require.EqualValues(t, "東京", input.City)
			require.EqualValues(t, "2022-01-01T00:00:00Z", input.When)
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
	specReq := httptest.NewRequest(http.MethodGet, "http://localhost:8080/.well-known/bedrock-tool-specification", nil)
	specResp := httptest.NewRecorder()
	h.ServeHTTP(specResp, specReq)
	require.Equal(t, http.StatusOK, specResp.Code)
	t.Log(specResp.Body.String())
	expected := `{
  "name": "weather",
  "description": "return weather",
  "input_schema": {
    "$id": "https://github.com/mashiike/bedrocktool/tool/remote/weather-input",
    "properties": {
      "city": {
        "default": "東京",
        "type": "string",
        "description": "都市名 (例: 横浜"
      },
      "when": {
        "type": "string",
        "description": "日時 RFC3339 (例: 2022-01-01T00:00:00Z)"
      }
    },
    "additionalProperties": false,
    "type": "object",
    "required": [
      "city",
      "when"
    ]
  },
  "worker_endpoint": "/worker/execute"
}`

	require.JSONEq(t, expected, specResp.Body.String())
	input := `{"city":"東京","when":"2022-01-01T00:00:00Z"}`
	workerReq := httptest.NewRequest(http.MethodPost, "http://localhost:8080/worker/execute", strings.NewReader(input))
	workerResp := httptest.NewRecorder()
	h.ServeHTTP(workerResp, workerReq)
	require.Equal(t, http.StatusOK, workerResp.Code)
	t.Log(workerResp.Body.String())
	expected = `{"content":[{"type":"text","text":"sunny"}],"status":"success"}`
	require.JSONEq(t, expected, workerResp.Body.String())
}

func TestHandler__NotFound(t *testing.T) {
	h, err := NewHandler(HandlerConfig{
		ToolName:   "weather",
		WorkerPath: "/worker/execute",
		Worker: bedrocktool.NewWorker(func(ctx context.Context, input weatherInput) (types.ToolResultBlock, error) {
			return types.ToolResultBlock{}, nil
		}),
	})
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodGet, "http://localhost:8080/notfound", nil)
	resp := httptest.NewRecorder()
	h.ServeHTTP(resp, req)
	require.Equal(t, http.StatusNotFound, resp.Code)
	require.JSONEq(t, `{"error":"Not Found", "message":"the requested resource \"/notfound\" was not found", "status":404}`, resp.Body.String())
}
