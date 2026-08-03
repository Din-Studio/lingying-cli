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

func TestLimitModelsRespectsPositiveSize(t *testing.T) {
	models := []client.GatewayModel{{ID: "one"}, {ID: "two"}, {ID: "three"}}
	got := limitModels(models, 2)
	if len(got) != 2 || got[0].ID != "one" || got[1].ID != "two" {
		t.Fatalf("limitModels() = %#v", got)
	}
}
