package stacks

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSyncURLFetchesDeps(t *testing.T) {
	base := "https://example.test/stack/"
	files := map[string]string{
		base + "docker-compose.yml": `
services:
  nginx:
    build: ./nginx
    volumes:
      - ./etc/redis.conf:/etc/redis.conf
`,
		base + "nginx/Dockerfile":   "FROM nginx:alpine\nCOPY default.conf /etc/nginx/conf.d/default.conf\n",
		base + "nginx/default.conf": "server {}",
		base + "etc/redis.conf":     "maxmemory 256mb",
	}
	get := func(url string) ([]byte, error) {
		if v, ok := files[url]; ok {
			return []byte(v), nil
		}
		return nil, os.ErrNotExist
	}

	work := t.TempDir()
	repo := Repository{Name: "t", Kind: RepoComposeURL, Location: base + "docker-compose.yml", Enabled: true}
	ps, err := repo.Sync(work, get)
	if err != nil {
		t.Fatal(err)
	}
	if len(ps.Services) != 1 {
		t.Fatalf("want 1 service, got %d", len(ps.Services))
	}
	cache := repo.CacheDir(work)
	for _, rel := range []string{"docker-compose.yml", "nginx/Dockerfile", "nginx/default.conf", "etc/redis.conf"} {
		if _, err := os.Stat(filepath.Join(cache, filepath.FromSlash(rel))); err != nil {
			t.Errorf("expected fetched file %s in cache: %v", rel, err)
		}
	}
}
