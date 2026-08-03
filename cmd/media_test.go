package cmd

import (
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
