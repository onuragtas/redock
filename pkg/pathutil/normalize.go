// Package pathutil holds tiny path-handling helpers shared across packages.
package pathutil

import "strings"

// dockerEnvDir is the canonical workDir leaf produced by
// docker-manager.GetWorkDir(): "<home>/.docker-environment". When a path
// stored in some entity's JSON references a different host's home, we
// rewrite the prefix up to and including this leaf so the path resolves
// against the running user's home on the current machine.
const dockerEnvDir = "/.docker-environment"

// NormalizeWorkDirPath rewrites a path's docker-environment prefix to
// match the given workDir, so cross-machine and cross-user restores keep
// embedded absolute paths valid.
//
// Example:
//
//	p       = "/Users/onuragtas/.docker-environment/data/tls.crt"
//	workDir = "/home/foo/.docker-environment"
//	result  = "/home/foo/.docker-environment/data/tls.crt"
//
// Paths that don't reference a docker-environment dir are returned
// unchanged — paths outside redock's own work tree (e.g. a deployment's
// repo path) stay as-is and remain the operator's responsibility.
//
// The function is idempotent: feeding it an already-correct path returns
// the same string.
func NormalizeWorkDirPath(p, workDir string) string {
	if p == "" || workDir == "" {
		return p
	}
	// Match "/.docker-environment/" with the trailing slash so we don't
	// accidentally rewrite e.g. "/.docker-environments/" or a path that
	// happens to end in ".docker-environment".
	marker := dockerEnvDir + "/"
	i := strings.Index(p, marker)
	if i < 0 {
		return p
	}
	suffix := p[i+len(dockerEnvDir):] // starts with "/"
	return workDir + suffix
}
