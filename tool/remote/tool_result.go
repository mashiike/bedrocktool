package remote

import (
	"encoding/json"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime/document"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime/types"
)

type ToolResult struct {
	Content []ToolResultContent `json:"content,omitempty"`
	Status  string              `json:"status,omitempty"`
}

func (tr ToolResult) MarshalTypes() (types.ToolResultBlock, error) {
	content := make([]types.ToolResultContentBlock, 0, len(tr.Content))
	for _, c := range tr.Content {
		tc, err := c.MarshalTypes()
		if err != nil {
			return types.ToolResultBlock{}, err
		}
		content = append(content, tc)
	}
	return types.ToolResultBlock{
		Content: content,
		Status:  types.ToolResultStatus(tr.Status),
	}, nil
}

func (tr *ToolResult) UnmarshalTypes(t types.ToolResultBlock) error {
	tr.Status = string(t.Status)
	tr.Content = make([]ToolResultContent, 0, len(t.Content))
	for _, c := range t.Content {
		var tc ToolResultContent
		if err := tc.UnmarshalTypes(c); err != nil {
			return err
		}
		tr.Content = append(tr.Content, tc)
	}
	return nil
}

type ToolResultContent struct {
	Type   string `json:"type"`
	Text   string `json:"text,omitempty"`
	Format string `json:"format,omitempty"`
	JSON   string `json:"json,omitempty"`
	Name   string `json:"name,omitempty"`
	Source []byte `json:"source,omitempty"`
}

func (trc ToolResultContent) MarshalTypes() (types.ToolResultContentBlock, error) {
	switch trc.Type {
	case "text":
		return &types.ToolResultContentBlockMemberText{
			Value: trc.Text,
		}, nil
	case "json":
		var v interface{}
		if err := json.Unmarshal([]byte(trc.JSON), &v); err != nil {
			return nil, err
		}
		return &types.ToolResultContentBlockMemberJson{
			Value: document.NewLazyDocument(v),
		}, nil
	case "document":
		return &types.ToolResultContentBlockMemberDocument{
			Value: types.DocumentBlock{
				Format: types.DocumentFormat(trc.Format),
				Name:   &trc.Name,
				Source: &types.DocumentSourceMemberBytes{
					Value: trc.Source,
				},
			},
		}, nil
	case "image":
		return &types.ToolResultContentBlockMemberImage{
			Value: types.ImageBlock{
				Format: types.ImageFormat(trc.Format),
				Source: &types.ImageSourceMemberBytes{
					Value: trc.Source,
				},
			},
		}, nil
	default:
		return nil, fmt.Errorf("unsupported content type: %s", trc.Type)
	}
}

func (trc *ToolResultContent) UnmarshalTypes(t types.ToolResultContentBlock) error {
	switch c := t.(type) {
	case *types.ToolResultContentBlockMemberText:
		trc.Type = "text"
		trc.Text = c.Value
	case *types.ToolResultContentBlockMemberJson:
		trc.Type = "json"
		bs, err := c.Value.MarshalSmithyDocument()
		if err != nil {
			return err
		}
		trc.JSON = string(bs)
	case *types.ToolResultContentBlockMemberDocument:
		trc.Type = "document"
		trc.Format = string(c.Value.Format)
		trc.Name = *c.Value.Name
		trc.Source = c.Value.Source.(*types.DocumentSourceMemberBytes).Value
	case *types.ToolResultContentBlockMemberImage:
		trc.Type = "image"
		trc.Format = string(c.Value.Format)
		trc.Source = c.Value.Source.(*types.ImageSourceMemberBytes).Value
	default:
		return fmt.Errorf("unsupported content type: %T", t)
	}
	return nil
}
