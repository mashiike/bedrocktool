package remote

import (
	"encoding/json"
	"fmt"
)

type Specification struct {
	// Name of the tool.
	Name string `json:"name"`
	// Description of the tool.
	Description string `json:"description,omitempty"`
	// InputSchema of the tool.
	InputSchema json.RawMessage `json:"input_schema"`
	// WorkerEndpoint of the tool.
	WorkerEndpoint string `json:"worker_endpoint"`
	// Extra
	Extra json.RawMessage `json:"-,omitempty"`
}

var DefaultSpecificationPath = "/.well-known/bedrock-tool-specification"

func (s *Specification) UnmarshalJSON(data []byte) error {
	type alias Specification
	if err := json.Unmarshal(data, (*alias)(s)); err != nil {
		return err
	}
	var extra map[string]json.RawMessage
	if err := json.Unmarshal(data, &extra); err != nil {
		return err
	}
	delete(extra, "name")
	delete(extra, "description")
	delete(extra, "input_schema")
	delete(extra, "worker_endpoint")
	if len(extra) == 0 {
		return nil
	}
	extraJSON, err := json.Marshal(extra)
	if err != nil {
		return fmt.Errorf("failed to marshal extra fields; %w", err)
	}
	s.Extra = extraJSON
	return nil
}

func (s *Specification) MarshalJSON() ([]byte, error) {
	data := make(map[string]any, len(s.Extra)+4)
	if s.Extra != nil {
		var extra map[string]json.RawMessage
		if err := json.Unmarshal(s.Extra, &extra); err != nil {
			return nil, err
		}
		for k, v := range extra {
			data[k] = v
		}
	}
	data["name"] = s.Name
	data["description"] = s.Description
	data["input_schema"] = s.InputSchema
	data["worker_endpoint"] = s.WorkerEndpoint
	return json.Marshal(data)
}
