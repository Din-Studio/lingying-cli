package cmd

import (
	"errors"
	"strings"
	"testing"

	"github.com/Din-Studio/lingying-cli/internal/output"
)

func TestCommandFailureUsesNonZeroErrorInBothModes(t *testing.T) {
	plain := commandFailure(false, "no_auth", "未配置鉴权", nil)
	if plain == nil || plain.Error() != "未配置鉴权" {
		t.Fatalf("plain error = %v", plain)
	}

	jsonErr := commandFailure(true, "no_auth", "未配置鉴权", nil)
	if !output.IsEmittedError(jsonErr) {
		t.Fatalf("json error = %v, want emitted error", jsonErr)
	}
}

func TestReadInteractivePromptPreservesSpaces(t *testing.T) {
	got, err := readInteractivePrompt(strings.NewReader("一只 赛博 朋克 猫\n"))
	if err != nil || got != "一只 赛博 朋克 猫" {
		t.Fatalf("readInteractivePrompt() = %q, %v", got, err)
	}
}

func TestReadInteractivePromptReportsEmptyInput(t *testing.T) {
	_, err := readInteractivePrompt(strings.NewReader("\n"))
	if err == nil || !errors.Is(err, errEmptyPrompt) {
		t.Fatalf("error = %v, want errEmptyPrompt", err)
	}
}
