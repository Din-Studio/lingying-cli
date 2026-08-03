package cmd

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/Din-Studio/lingying-cli/internal/client"
)

func TestSelectMediaModelRejectsMissingRequestedModel(t *testing.T) {
	models := []client.GatewayModel{{ID: "image-1", ModelID: "model-image-1", ModelType: "image", DisplayName: "Image One"}}
	_, err := selectMediaModel(models, map[string]bool{"image": true}, "not-present")
	if err == nil || !strings.Contains(err.Error(), "未找到") {
		t.Fatalf("selectMediaModel() error = %v, want missing-model error", err)
	}
}

func TestSelectMediaModelRejectsWrongModelType(t *testing.T) {
	models := []client.GatewayModel{{ID: "video-1", ModelID: "model-video-1", ModelType: "video", DisplayName: "Video One"}}
	_, err := selectMediaModel(models, map[string]bool{"image": true}, "video-1")
	if err == nil || !strings.Contains(err.Error(), "类型") {
		t.Fatalf("selectMediaModel() error = %v, want type error", err)
	}
}

func TestSelectTextModelRejectsWrongModelType(t *testing.T) {
	models := []client.GatewayModel{{ID: "image-1", ModelType: "image", DisplayName: "Image One"}}
	_, err := selectTextModel(models, "image-1")
	if err == nil || !strings.Contains(err.Error(), "类型") {
		t.Fatalf("selectTextModel() error = %v, want type error", err)
	}
}

func TestSelectTextModelChoosesStableDynamicDefault(t *testing.T) {
	models := []client.GatewayModel{
		{ID: "text-z", ModelType: "text", DisplayName: "Z"},
		{ID: "image-1", ModelType: "image", DisplayName: "Image"},
		{ID: "text-a", ModelType: "text", DisplayName: "A"},
	}
	model, err := selectTextModel(models, "")
	if err != nil {
		t.Fatalf("selectTextModel() error = %v", err)
	}
	if model.ID != "text-a" {
		t.Fatalf("selected ID = %q, want text-a", model.ID)
	}
}

func TestSelectMediaModelChoosesStableDynamicDefault(t *testing.T) {
	models := []client.GatewayModel{
		{ID: "image-z", ModelID: "model-z", ModelType: "image", DisplayName: "Z"},
		{ID: "video-1", ModelID: "model-video", ModelType: "video", DisplayName: "Video"},
		{ID: "image-a", ModelID: "model-a", ModelType: "image", DisplayName: "A"},
	}
	model, err := selectMediaModel(models, map[string]bool{"image": true}, "")
	if err != nil {
		t.Fatalf("selectMediaModel() error = %v", err)
	}
	if model.ID != "image-a" {
		t.Fatalf("selected ID = %q, want image-a", model.ID)
	}
}

func TestApplyRequiredSchemaDefaultsFillsMissingRequiredValues(t *testing.T) {
	input := map[string]any{
		"prompt":     "赛博朋克猫",
		"resolution": "4K",
	}
	schema := json.RawMessage(`{
		"required": ["prompt", "aspect_ratio", "resolution"],
		"properties": {
			"prompt": {"type": "string"},
			"aspect_ratio": {"type": "string", "default": "1:1"},
			"resolution": {"type": "string", "default": "1K"},
			"quality": {"type": "string", "default": "medium"}
		}
	}`)

	applyRequiredSchemaDefaults(input, schema)

	if got := input["aspect_ratio"]; got != "1:1" {
		t.Fatalf("aspect_ratio = %#v, want default 1:1", got)
	}
	if got := input["resolution"]; got != "4K" {
		t.Fatalf("resolution = %#v, want user value 4K", got)
	}
	if _, exists := input["quality"]; exists {
		t.Fatal("optional default quality must be left to Gateway")
	}
}

func TestCoerceParamValueUsesDiscoveredSchemaTypes(t *testing.T) {
	schema := json.RawMessage(`{
		"properties": {
			"aspect_ratio": {"type": "string"},
			"resolution": {"type": "string"},
			"duration": {"type": "integer"},
			"generate_audio": {"type": "boolean"}
		}
	}`)

	cases := []struct {
		key  string
		raw  string
		want any
	}{
		{key: "aspect_ratio", raw: "9:16", want: "9:16"},
		{key: "resolution", raw: "1K", want: "1K"},
		{key: "duration", raw: "10", want: int64(10)},
		{key: "generate_audio", raw: "yes", want: true},
		{key: "unknown", raw: "1", want: "1"},
	}
	for _, tc := range cases {
		if got := coerceParamValue(tc.key, tc.raw, schema); got != tc.want {
			t.Fatalf("coerceParamValue(%q, %q) = %#v (%T), want %#v (%T)", tc.key, tc.raw, got, got, tc.want, tc.want)
		}
	}
}

func TestApplySchemaParamBuildsNestedObjectsAndUsesLeafTypes(t *testing.T) {
	schema := json.RawMessage(`{
		"properties": {
			"metadata": {
				"type": "object",
				"properties": {
					"quality": {"type": "string"},
					"light_elevation": {"type": "number"},
					"generate_audio": {"type": "boolean"},
					"conversion_slots": {"type": "array"}
				}
			}
		}
	}`)
	input := map[string]any{}

	for _, param := range [][2]string{
		{"metadata.quality", "high"},
		{"metadata.light_elevation", "30"},
		{"metadata.generate_audio", "false"},
		{"metadata.conversion_slots", `["slot_a","slot_b"]`},
	} {
		if err := applySchemaParam(input, param[0], param[1], schema); err != nil {
			t.Fatalf("applySchemaParam(%q) error = %v", param[0], err)
		}
	}

	metadata, ok := input["metadata"].(map[string]any)
	if !ok {
		t.Fatalf("metadata = %#v (%T), want object", input["metadata"], input["metadata"])
	}
	if got := metadata["quality"]; got != "high" {
		t.Fatalf("metadata.quality = %#v, want high", got)
	}
	if got := metadata["light_elevation"]; got != float64(30) {
		t.Fatalf("metadata.light_elevation = %#v (%T), want number 30", got, got)
	}
	if got := metadata["generate_audio"]; got != false {
		t.Fatalf("metadata.generate_audio = %#v (%T), want false", got, got)
	}
	if slots, ok := metadata["conversion_slots"].([]any); !ok || len(slots) != 2 {
		t.Fatalf("metadata.conversion_slots = %#v, want JSON array", metadata["conversion_slots"])
	}
}

func TestApplySchemaParamParsesTopLevelObjectsAndArraysAsJSON(t *testing.T) {
	schema := json.RawMessage(`{
		"properties": {
			"metadata": {"type": "object"},
			"images": {"type": "array"}
		}
	}`)
	input := map[string]any{}

	if err := applySchemaParam(input, "metadata", `{"quality":"high"}`, schema); err != nil {
		t.Fatalf("object parameter error = %v", err)
	}
	if err := applySchemaParam(input, "images", `["https://example.test/a.png"]`, schema); err != nil {
		t.Fatalf("array parameter error = %v", err)
	}

	if _, ok := input["metadata"].(map[string]any); !ok {
		t.Fatalf("metadata = %#v (%T), want object", input["metadata"], input["metadata"])
	}
	if _, ok := input["images"].([]any); !ok {
		t.Fatalf("images = %#v (%T), want array", input["images"], input["images"])
	}
}

func TestApplySchemaParamRejectsNonJSONContainerAndUnknownNestedPath(t *testing.T) {
	schema := json.RawMessage(`{"properties":{"metadata":{"type":"object","properties":{"quality":{"type":"string"}}}}}`)
	input := map[string]any{}

	if err := applySchemaParam(input, "metadata", "quality=high", schema); err == nil {
		t.Fatal("object without JSON should fail")
	}
	if err := applySchemaParam(input, "metadata.unknown", "high", schema); err == nil {
		t.Fatal("unknown nested path should fail")
	}
}

func TestGetPromptFromArgsPreservesEquals(t *testing.T) {
	got := getPromptFromArgs([]string{"画面中展示 x = y"})
	if got != "画面中展示 x = y" {
		t.Fatalf("prompt = %q, want original text", got)
	}
}

func TestAppendInputURLPreservesSchemaArrayAndAddsUploadedURL(t *testing.T) {
	input := map[string]any{"images": []any{"https://example.test/first.png"}}
	if err := appendInputURL(input, "images", "https://example.test/second.png"); err != nil {
		t.Fatalf("appendInputURL error = %v", err)
	}
	images, ok := input["images"].([]any)
	if !ok || len(images) != 2 || images[1] != "https://example.test/second.png" {
		t.Fatalf("images = %#v, want two URLs", input["images"])
	}
}

func TestInputFieldForFileUsesMediaTypeInsteadOfFeatureMapOrder(t *testing.T) {
	model := &client.GatewayModel{FeatureTypes: map[string]client.Feature{
		"video_to_video": {MatchFields: []string{"prompt", "videos"}},
		"image_to_video": {MatchFields: []string{"prompt", "images"}},
	}}
	if got := inputFieldForFile(model, "video", "./reference.jpg"); got != "images" {
		t.Fatalf("image input field = %q, want images", got)
	}
	if got := inputFieldForFile(model, "video", "./source.mp4"); got != "videos" {
		t.Fatalf("video input field = %q, want videos", got)
	}
}
