package pathutil

import "testing"

func TestNormalizeWorkDirPath(t *testing.T) {
	cases := []struct {
		name    string
		path    string
		workDir string
		want    string
	}{
		{
			name:    "rewrite cross-user prefix",
			path:    "/Users/onuragtas/.docker-environment/data/tls.crt",
			workDir: "/home/foo/.docker-environment",
			want:    "/home/foo/.docker-environment/data/tls.crt",
		},
		{
			name:    "idempotent on already-correct path",
			path:    "/home/foo/.docker-environment/data/tls.crt",
			workDir: "/home/foo/.docker-environment",
			want:    "/home/foo/.docker-environment/data/tls.crt",
		},
		{
			name:    "deeply nested suffix preserved",
			path:    "/Users/x/.docker-environment/data/email/cert/tls.key",
			workDir: "/home/y/.docker-environment",
			want:    "/home/y/.docker-environment/data/email/cert/tls.key",
		},
		{
			name:    "unrelated path passes through",
			path:    "/var/lib/docker/volumes/db/data",
			workDir: "/home/foo/.docker-environment",
			want:    "/var/lib/docker/volumes/db/data",
		},
		{
			name:    "empty path stays empty",
			path:    "",
			workDir: "/home/foo/.docker-environment",
			want:    "",
		},
		{
			name:    "empty workDir is no-op",
			path:    "/Users/onuragtas/.docker-environment/data/tls.crt",
			workDir: "",
			want:    "/Users/onuragtas/.docker-environment/data/tls.crt",
		},
		{
			name:    "false-positive guard: .docker-environments plural is NOT rewritten",
			path:    "/Users/x/.docker-environments/foo",
			workDir: "/home/y/.docker-environment",
			want:    "/Users/x/.docker-environments/foo",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := NormalizeWorkDirPath(tc.path, tc.workDir)
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}
