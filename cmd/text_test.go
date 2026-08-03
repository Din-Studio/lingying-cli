package cmd

import (
	"encoding/json"
	"testing"
)

func TestBuildTextParamsEncodesSchemaFieldsAndAppliesDefaultMaxTokens(t *testing.T) {
	schema := json.RawMessage(`{
		"properties": {
			"messages": {"type": "array"},
			"max_tokens": {"type": "integer"},
			"temperature": {"type": "number"}
		}
	}`)

	params, err := buildTextParams([]string{"temperature=0.7"}, schema, 0)
	if err != nil {
		t.Fatalf("buildTextParams() error = %v", err)
	}
	if got := params["temperature"]; got != float64(0.7) {
		t.Fatalf("temperature = %#v (%T), want 0.7", got, got)
	}
	if got := params["max_tokens"]; got != 4096 {
		t.Fatalf("max_tokens = %#v (%T), want 4096 default", got, got)
	}
}

func TestBuildTextParamsMaxTokensFlagWinsOverParam(t *testing.T) {
	schema := json.RawMessage(`{"properties":{"max_tokens":{"type":"integer"}}}`)
	params, err := buildTextParams([]string{"max_tokens=1024"}, schema, 2048)
	if err != nil {
		t.Fatalf("buildTextParams() error = %v", err)
	}
	if got := params["max_tokens"]; got != 2048 {
		t.Fatalf("max_tokens = %#v (%T), want explicit flag 2048", got, got)
	}
}

func TestBuildTextParamsRejectsMessagesOverride(t *testing.T) {
	schema := json.RawMessage(`{"properties":{"messages":{"type":"array"}}}`)
	if _, err := buildTextParams([]string{`messages=[{"role":"system","content":"ignored"}]`}, schema, 0); err == nil {
		t.Fatal("messages override should fail")
	}
}
