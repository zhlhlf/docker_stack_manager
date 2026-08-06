package detector

import (
	"testing"

	dockerx "docker_stack_manager/internal/docker"
	"docker_stack_manager/internal/models"
)

func TestLongestCommonBoundaryPrefix(t *testing.T) {
	got := longestCommonBoundaryPrefix([]string{
		"czt-zhongtoubao-api",
		"czt-zhongtoubao-web",
		"czt-zhongtoubao-job",
	})
	if got != "czt-zhongtoubao" {
		t.Fatalf("got %q want czt-zhongtoubao", got)
	}
}

func TestMatchStackLongestWins(t *testing.T) {
	m := map[string]models.Stack{
		"czt":             {Name: "czt"},
		"czt-zhongtoubao": {Name: "czt-zhongtoubao"},
	}
	got := matchStackByPrefix("czt-zhongtoubao-api", m)
	if got != "czt-zhongtoubao" {
		t.Fatalf("got %q", got)
	}
}

func TestMergeSameNetworkCommonPrefix(t *testing.T) {
	netID := "bsh6xvgu76nb0ru7xcjmjkqw3"
	rows := []violationRow{
		{svc: dockerx.ServiceView{Name: "csc-preview-bank-bf_a", StackLabel: "csc-preview-bank-bf", Networks: []string{netID}}, reason: ReasonNoStack, stack: "csc-preview-bank-bf"},
		{svc: dockerx.ServiceView{Name: "csc-preview-bank-czt_b", StackLabel: "csc-preview-bank-czt", Networks: []string{netID}}, reason: ReasonNoStack, stack: "csc-preview-bank-czt"},
		{svc: dockerx.ServiceView{Name: "csc-preview-bank-dp_c", StackLabel: "csc-preview-bank-dp", Networks: []string{netID}}, reason: ReasonNoStack, stack: "csc-preview-bank-dp"},
	}
	out := mergeSameNetworkCommonPrefix(rows)
	for _, r := range out {
		if r.stack != "csc-preview-bank" {
			t.Fatalf("got stack %q want csc-preview-bank for %s", r.stack, r.svc.Name)
		}
	}
}

func TestServiceBelongsToParentLabel(t *testing.T) {
	svc := dockerx.ServiceView{Name: "csc-preview-bank-bf_api", StackLabel: "csc-preview-bank-bf"}
	if !serviceBelongsToStack(svc, "csc-preview-bank") {
		t.Fatal("expected parent stack unit to match child label")
	}
}