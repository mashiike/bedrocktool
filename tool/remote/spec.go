package remote

import "encoding/json"

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
	Extra map[string]json.RawMessage `json:"-,omitempty"`
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
	s.Extra = extra
	return nil
}

func (s *Specification) MarshalJSON() ([]byte, error) {
	data := make(map[string]any, len(s.Extra)+4)
	for k, v := range s.Extra {
		data[k] = v
	}
	data["name"] = s.Name
	data["description"] = s.Description
	data["input_schema"] = s.InputSchema
	data["worker_endpoint"] = s.WorkerEndpoint
	return json.Marshal(data)
}
