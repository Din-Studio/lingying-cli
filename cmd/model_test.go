package cmd

import (
	"testing"

	"github.com/Din-Studio/lingying-cli/internal/client"
)

func TestSelectModelInfoPrefersExactMatchOverPartialMatch(t *testing.T) {
	models := []client.GatewayModel{
		{ID: "seedance2.0 fast", DisplayName: "seedance2.0 fast"},
		{ID: "seedance2.0", DisplayName: "seedance2.0"},
	}
	matched := selectModelInfo(models, "seedance2.0")
	if matched == nil || matched.ID != "seedance2.0" {
		t.Fatalf("selected = %#v, want exact seedance2.0", matched)
	}
}
