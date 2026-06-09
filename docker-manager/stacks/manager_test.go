package stacks

import "testing"

func TestResolveOrderDependencies(t *testing.T) {
	m := &Manager{}
	cat := map[string]ServiceSpec{
		"global":       {Name: "global"},
		"php74_xdebug": {Name: "php74_xdebug", DependsOn: []string{"global"}},
		"nginx":        {Name: "nginx", DependsOn: []string{"php74_xdebug"}},
		"redis":        {Name: "redis"},
	}
	order, err := m.resolveOrder([]string{"nginx"}, cat)
	if err != nil {
		t.Fatal(err)
	}
	pos := map[string]int{}
	for i, n := range order {
		pos[n] = i
	}
	// transitive deps must be included
	for _, n := range []string{"global", "php74_xdebug", "nginx"} {
		if _, ok := pos[n]; !ok {
			t.Fatalf("missing %q in order %v", n, order)
		}
	}
	// dependencies first
	if pos["global"] > pos["php74_xdebug"] || pos["php74_xdebug"] > pos["nginx"] {
		t.Errorf("wrong order: %v", order)
	}
	if _, ok := pos["redis"]; ok {
		t.Errorf("redis should not be pulled in: %v", order)
	}
}

func TestResolveOrderCycle(t *testing.T) {
	m := &Manager{}
	cat := map[string]ServiceSpec{
		"a": {Name: "a", DependsOn: []string{"b"}},
		"b": {Name: "b", DependsOn: []string{"a"}},
	}
	if _, err := m.resolveOrder([]string{"a"}, cat); err == nil {
		t.Error("expected cycle error")
	}
}

func TestMaterializeBindResolution(t *testing.T) {
	work := t.TempDir()
	m := &Manager{WorkDir: work, Env: ParseEnv("PROJECT_DEFAULT_PATH=/srv/sites\n")}
	spec := ServiceSpec{
		Name: "x", SourceDir: "", // no repo
		Volumes: []VolumeMount{
			{Kind: VolumeBind, Source: "${PROJECT_DEFAULT_PATH}", Target: "/var/www/html"},
			{Kind: VolumeBind, Source: "./logs", Target: "/var/log"},
		},
	}
	s := m.materialize(spec)
	if s.Volumes[0].Source != "/srv/sites" {
		t.Errorf("env path not resolved: %q", s.Volumes[0].Source)
	}
	if !pathExists(s.Volumes[1].Source) {
		t.Errorf("runtime dir not created: %q", s.Volumes[1].Source)
	}
}
