package remote

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSpecification_MarshalJSON(t *testing.T) {
	spec := Specification{
		Name:           "test-tool",
		Description:    "A tool for testing",
		InputSchema:    json.RawMessage(`{"type": "object", "properties": {"input": {"type": "string"}}}`),
		WorkerEndpoint: "http://localhost:8080/worker",
		Extra: map[string]json.RawMessage{
			"extra_field_1": json.RawMessage(`"extra_value_1"`),
			"extra_field_2": json.RawMessage(`{"nested": "value"}`),
		},
	}

	data, err := spec.MarshalJSON()
	require.NoError(t, err)

	expectedJSON := `{
        "name": "test-tool",
        "description": "A tool for testing",
        "input_schema": {"type": "object", "properties": {"input": {"type": "string"}}},
        "worker_endpoint": "http://localhost:8080/worker",
        "extra_field_1": "extra_value_1",
        "extra_field_2": {"nested": "value"}
    }`

	var expected map[string]interface{}
	err = json.Unmarshal([]byte(expectedJSON), &expected)
	require.NoError(t, err)

	var actual map[string]interface{}
	err = json.Unmarshal(data, &actual)
	require.NoError(t, err)

	require.Equal(t, expected, actual)
}

func TestSpecification_UnmarshalJSON(t *testing.T) {
	jsonData := `{
        "name": "test-tool",
        "description": "A tool for testing",
        "input_schema": {"type": "object", "properties": {"input": {"type": "string"}}},
        "worker_endpoint": "http://localhost:8080/worker",
        "extra_field_1": "extra_value_1",
        "extra_field_2": {"nested": "value"}
    }`

	var spec Specification
	err := json.Unmarshal([]byte(jsonData), &spec)
	require.NoError(t, err)

	require.Equal(t, "test-tool", spec.Name)
	require.Equal(t, "A tool for testing", spec.Description)
	require.JSONEq(t, `{"type": "object", "properties": {"input": {"type": "string"}}}`, string(spec.InputSchema))
	require.Equal(t, "http://localhost:8080/worker", spec.WorkerEndpoint)
	require.JSONEq(t, `"extra_value_1"`, string(spec.Extra["extra_field_1"]))
	require.JSONEq(t, `{"nested": "value"}`, string(spec.Extra["extra_field_2"]))
}

func TestSpecification_EmptyExtra(t *testing.T) {
	spec := Specification{
		Name:           "test-tool",
		Description:    "A tool for testing",
		InputSchema:    json.RawMessage(`{"type": "object", "properties": {"input": {"type": "string"}}}`),
		WorkerEndpoint: "http://localhost:8080/worker",
		Extra:          map[string]json.RawMessage{},
	}

	data, err := spec.MarshalJSON()
	require.NoError(t, err)

	expectedJSON := `{
        "name": "test-tool",
        "description": "A tool for testing",
        "input_schema": {"type": "object", "properties": {"input": {"type": "string"}}},
        "worker_endpoint": "http://localhost:8080/worker"
    }`

	var expected map[string]interface{}
	err = json.Unmarshal([]byte(expectedJSON), &expected)
	require.NoError(t, err)

	var actual map[string]interface{}
	err = json.Unmarshal(data, &actual)
	require.NoError(t, err)

	require.Equal(t, expected, actual)
}

func TestSpecification_UnmarshalJSON_EmptyExtra(t *testing.T) {
	jsonData := `{
        "name": "test-tool",
        "description": "A tool for testing",
        "input_schema": {"type": "object", "properties": {"input": {"type": "string"}}},
        "worker_endpoint": "http://localhost:8080/worker"
    }`

	var spec Specification
	err := json.Unmarshal([]byte(jsonData), &spec)
	require.NoError(t, err)

	require.Equal(t, "test-tool", spec.Name)
	require.Equal(t, "A tool for testing", spec.Description)
	require.JSONEq(t, `{"type": "object", "properties": {"input": {"type": "string"}}}`, string(spec.InputSchema))
	require.Equal(t, "http://localhost:8080/worker", spec.WorkerEndpoint)
	require.Empty(t, spec.Extra)
}
