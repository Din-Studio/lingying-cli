package cmd

import (
	"errors"
	"strings"
	"testing"

	"github.com/Din-Studio/lingying-cli/internal/output"
	"github.com/spf13/cobra"
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

func newProjectFlagCmd() *cobra.Command {
	c := &cobra.Command{}
	c.Flags().String("project", "", "")
	return c
}

func TestResolveProjectIDReturnsEmptyByDefault(t *testing.T) {
	if got := resolveProjectID(newProjectFlagCmd()); got != "" {
		t.Fatalf("resolveProjectID() = %q, want empty", got)
	}
}

func TestResolveProjectIDUsesEnvWhenFlagUnset(t *testing.T) {
	t.Setenv("LY_PROJECT_ID", "proj-env")
	if got := resolveProjectID(newProjectFlagCmd()); got != "proj-env" {
		t.Fatalf("resolveProjectID() = %q, want proj-env", got)
	}
}

func TestResolveProjectIDFlagOverridesEnv(t *testing.T) {
	t.Setenv("LY_PROJECT_ID", "proj-env")
	c := newProjectFlagCmd()
	if err := c.Flags().Set("project", "proj-flag"); err != nil {
		t.Fatal(err)
	}
	if got := resolveProjectID(c); got != "proj-flag" {
		t.Fatalf("resolveProjectID() = %q, want proj-flag (flag wins)", got)
	}
}
