package detector

import (
	"testing"

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