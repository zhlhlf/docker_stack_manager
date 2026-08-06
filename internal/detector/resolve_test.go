package detector

import (
	"testing"

	dockerx "docker_stack_manager/internal/docker"
	"docker_stack_manager/internal/models"
)

func TestMatchStackByPrefix(t *testing.T) {
	stacks := map[string]models.Stack{
		"czt":              {Name: "czt"},
		"czt-zhongtoubao":  {Name: "czt-zhongtoubao"},
		"webapp":           {Name: "webapp"},
	}

	cases := []struct {
		svc  string
		want string
	}{
		{"czt-zhongtoubao-czt", "czt-zhongtoubao"},
		{"czt-zhongtoubao", "czt-zhongtoubao"},
		{"czt-other", "czt"},
		{"webapp_api", "webapp"},
		{"webapp", "webapp"},
		{"legacy", ""},
		{"cztzhongtoubao", ""}, // no separator boundary
	}
	for _, c := range cases {
		got := matchStackByPrefix(c.svc, stacks)
		if got != c.want {
			t.Fatalf("service %q: got %q want %q", c.svc, got, c.want)
		}
	}
}

func TestResolveStackPrefersConfiguredLabel(t *testing.T) {
	stacks := map[string]models.Stack{
		"czt-zhongtoubao": {Name: "czt-zhongtoubao"},
	}
	svc := dockerx.ServiceView{
		Name:       "czt-zhongtoubao-czt",
		StackLabel: "czt-zhongtoubao",
	}
	if got := resolveStack(svc, stacks); got != "czt-zhongtoubao" {
		t.Fatalf("got %q", got)
	}
}