package stacks

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestCachePathRejectsEscape(t *testing.T) {
	cache := t.TempDir()
	for _, rel := range []string{"../x", "../../../../etc/cron.d/x", "a/../../x"} {
		if _, err := cachePath(cache, rel); err == nil {
			t.Errorf("cachePath(%q) should fail", rel)
		}
	}
	got, err := cachePath(cache, "etc/redis.conf")
	if err != nil || got != filepath.Join(cache, "etc", "redis.conf") {
		t.Fatalf("cachePath(etc/redis.conf) = %q, %v", got, err)
	}
}

func TestValidateName(t *testing.T) {
	for _, bad := range []string{"", ".", "..", "../x", "a/b", "a..b", ".hidden", "-x"} {
		if validateName("repository", bad) == nil {
			t.Errorf("name %q should be rejected", bad)
		}
	}
	for _, ok := range []string{"default", "my-repo", "php8.3", "svc_1"} {
		if err := validateName("repository", ok); err != nil {
			t.Errorf("name %q rejected: %v", ok, err)
		}
	}
}

func TestPlatformSpecVariant(t *testing.T) {
	p := platformSpec("linux/arm/v7")
	if p.OS != "linux" || p.Architecture != "arm" || p.Variant != "v7" {
		t.Fatalf("got %+v", p)
	}
	if p := platformSpec("linux/amd64"); p.Architecture != "amd64" || p.Variant != "" {
		t.Fatalf("got %+v", p)
	}
}

func TestDrainReportsStreamError(t *testing.T) {
	stream := `{"status":"Pulling"}` + "\n" +
		`{"errorDetail":{"message":"no matching manifest for linux/arm/v7"},"error":"no matching manifest for linux/arm/v7"}` + "\n"
	var out strings.Builder
	err := drain(strings.NewReader(stream), &out)
	if err == nil || !strings.Contains(err.Error(), "no matching manifest") {
		t.Fatalf("err = %v", err)
	}
	if out.String() != stream {
		t.Fatal("stream not copied to log output")
	}
	if err := drain(strings.NewReader(`{"stream":"Step 1/2"}`), nil); err != nil {
		t.Fatalf("clean stream returned %v", err)
	}
}

// The config hash is what detects edits: it must be stable for the same spec
// and change when an env value changes.
func TestConfigHashTracksEnv(t *testing.T) {
	e := &Engine{}
	spec := ServiceSpec{Name: "db", Image: "mysql:8", Env: map[string]string{"A": "1", "B": "2"}}
	h1 := e.buildCreateConfig(spec).hash()
	if h2 := e.buildCreateConfig(spec).hash(); h1 != h2 {
		t.Fatal("hash not stable")
	}
	spec.Env = map[string]string{"A": "1", "B": "3"}
	if e.buildCreateConfig(spec).hash() == h1 {
		t.Fatal("env change did not change the hash")
	}
}

func TestUpdateRepositoryKeepsEnabled(t *testing.T) {
	m := &Manager{WorkDir: t.TempDir(), Registry: NewRegistry(t.TempDir())}
	m.Registry.Repos = append(m.Registry.Repos, Repository{Name: "r", Kind: RepoLocal, Location: "/a", Enabled: true})
	if err := m.UpdateRepository("r", Repository{Kind: RepoLocal, Location: "/b"}); err != nil {
		t.Fatal(err)
	}
	r := m.Registry.Repos[len(m.Registry.Repos)-1]
	if !r.Enabled || r.Location != "/b" {
		t.Fatalf("got %+v", r)
	}
	if err := m.RemoveCustomService(".."); err == nil {
		t.Fatal(`RemoveCustomService("..") must be rejected`)
	}
}
