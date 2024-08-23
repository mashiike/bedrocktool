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
	// VerifyEndpoint of the tool. (optional) check for tool execution authorization.
	VerifyEndpoint string `json:"verify_endpoint,omitempty"`
}

var SpecificationPath = "/.well-known/bedrock-tool-specification"
