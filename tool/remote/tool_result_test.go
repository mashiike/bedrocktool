package remote

import (
	"encoding/json"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime/document"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime/types"
	"github.com/stretchr/testify/require"
)

func TestToolResult__Marshal(t *testing.T) {
	toolResult := types.ToolResultBlock{
		Content: []types.ToolResultContentBlock{
			&types.ToolResultContentBlockMemberText{
				Value: "Hello, World!",
			},
			&types.ToolResultContentBlockMemberJson{
				Value: document.NewLazyDocument(map[string]interface{}{
					"key": "value",
				}),
			},
			&types.ToolResultContentBlockMemberDocument{
				Value: types.DocumentBlock{
					Format: types.DocumentFormatCsv,
					Name:   aws.String("example"),
					Source: &types.DocumentSourceMemberBytes{
						Value: []byte("a,b,c\n1,2,3"),
					},
				},
			},
			&types.ToolResultContentBlockMemberImage{
				Value: types.ImageBlock{
					Format: types.ImageFormatPng,
					Source: &types.ImageSourceMemberBytes{
						Value: []byte("image data"),
					},
				},
			},
		},
		Status: types.ToolResultStatusSuccess,
	}
	var tr ToolResult
	err := tr.UnmarshalTypes(toolResult)
	require.NoError(t, err)
	bs, err := json.MarshalIndent(tr, "", "  ")
	require.NoError(t, err)
	expected := `{
	"content": [
		{
			"type": "text",
			"text": "Hello, World!"
		},
		{
			"type": "json",
			"json": "{\"key\":\"value\"}"
		},
		{
			"type": "document",
			"format": "csv",
			"name": "example",
			"source": "YSxiLGMKMSwyLDM="
		},
		{
			"type": "image",
			"format": "png",
			"source": "aW1hZ2UgZGF0YQ=="
		}
	],
	"status": "success"
}
`
	t.Log(string(bs))
	require.JSONEq(t, expected, string(bs))
	var tr2 ToolResult
	err = json.Unmarshal(bs, &tr2)
	require.NoError(t, err)
	require.EqualValues(t, tr, tr2)
	acutal, err := tr2.MarshalTypes()
	require.NoError(t, err)
	require.EqualValues(t, toolResult, acutal)
}
