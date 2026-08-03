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
