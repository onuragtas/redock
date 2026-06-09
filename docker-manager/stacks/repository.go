package stacks

import (
	"archive/zip"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	git "gopkg.in/src-d/go-git.v4"
	"gopkg.in/src-d/go-git.v4/plumbing"
)

// RepoKind enumerates where a repository's services come from.
type RepoKind string

const (
	// RepoComposeURL is a remote docker-compose file (e.g. on GitHub Pages).
	// Its build contexts and bind-mounted config files are fetched over HTTP,
	// relative to the compose URL's directory, and cached on disk.
	RepoComposeURL RepoKind = "compose-url"
	// RepoLocal is a local directory containing a docker-compose file and its
	// build contexts / config files (read in place, no fetch).
	RepoLocal RepoKind = "local"
	// RepoGit is a git repository cloned/pulled into the cache; the whole tree
	// (compose + build contexts + config files) lands on disk in one operation,
	// so no per-file fetching/COPY-parsing is needed.
	RepoGit RepoKind = "git"
)

// DefaultRepoName / DefaultRepoComposeURL define the built-in repository. Like
// update.json, it is served as a static file (GitHub Pages / raw) and fetched
// on demand — nothing is compiled into the binary. Override via settings.
const (
	DefaultRepoName       = "default"
	DefaultRepoComposeURL = "https://raw.githubusercontent.com/onuragtas/docker/master/docker-compose.yml.dist"
)

// Repository is a source of service definitions.
type Repository struct {
	Name     string   `json:"name"`
	Kind     RepoKind `json:"kind"`
	Location string   `json:"location"` // compose URL, local dir path, or git URL
	Compose  string   `json:"compose"`  // compose file(s); comma-separated, merged in order
	Ref      string   `json:"ref"`      // git branch/tag/commit (RepoGit only)
	Enabled  bool     `json:"enabled"`
}

// composeList returns the compose file name(s), defaulting to docker-compose.yml.
// Multiple comma-separated entries are merged in order (override semantics).
func (r Repository) composeList() []string {
	if strings.TrimSpace(r.Compose) == "" {
		return []string{"docker-compose.yml"}
	}
	var out []string
	for _, p := range strings.Split(r.Compose, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// CacheDir is where a compose-url repository's fetched content lands. For a
// local repository it is the repository's own directory.
func (r Repository) CacheDir(workDir string) string {
	if r.Kind == RepoLocal {
		return r.Location
	}
	return filepath.Join(workDir, "repositories", r.Name)
}

// HTTPGetter fetches a URL's bytes (overridable for tests).
type HTTPGetter func(url string) ([]byte, error)

func defaultHTTPGet(url string) ([]byte, error) {
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: status %d", url, resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}

// Sync materializes the repository on disk and returns the parsed (merged)
// stack. Local repos are read in place; compose-url repos fetch the compose
// file(s) + deps over HTTP; git repos clone/pull the whole tree.
func (r Repository) Sync(workDir string, get HTTPGetter) (*ParsedStack, error) {
	if get == nil {
		get = defaultHTTPGet
	}
	switch r.Kind {
	case RepoLocal:
		return r.parseLocal(r.Location)
	case RepoGit:
		return r.syncGit(workDir)
	case RepoComposeURL:
		return r.syncURL(workDir, get)
	default:
		return nil, fmt.Errorf("unknown repository kind %q", r.Kind)
	}
}

// parseLocal reads and merges the compose file(s) from a directory on disk.
func (r Repository) parseLocal(dir string) (*ParsedStack, error) {
	var docs [][]byte
	for _, name := range r.composeList() {
		data, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(name)))
		if err != nil {
			return nil, fmt.Errorf("read compose %s: %w", name, err)
		}
		docs = append(docs, data)
	}
	return ParseComposeFiles(docs)
}

// normalizeRawURL converts common git-host "view" URLs to raw file URLs so a
// pasted github.com/.../blob/... link fetches YAML instead of an HTML page.
func normalizeRawURL(u string) string {
	switch {
	case strings.HasPrefix(u, "https://github.com/") && strings.Contains(u, "/blob/"):
		rest := strings.TrimPrefix(u, "https://github.com/")
		rest = strings.Replace(rest, "/blob/", "/", 1)
		return "https://raw.githubusercontent.com/" + rest
	case strings.Contains(u, "gitlab.com/") && strings.Contains(u, "/-/blob/"):
		return strings.Replace(u, "/-/blob/", "/-/raw/", 1)
	case strings.Contains(u, "bitbucket.org/") && strings.Contains(u, "/src/"):
		return strings.Replace(u, "/src/", "/raw/", 1)
	}
	return u
}

func (r Repository) syncURL(workDir string, get HTTPGetter) (*ParsedStack, error) {
	cache := r.CacheDir(workDir)
	if err := os.MkdirAll(cache, 0o755); err != nil {
		return nil, err
	}

	loc := normalizeRawURL(r.Location)
	baseURL := loc[:strings.LastIndex(loc, "/")+1]

	// Compose file names: explicit list, or the basename of the (normalized) URL.
	names := r.composeList()
	if strings.TrimSpace(r.Compose) == "" {
		names = []string{path.Base(loc)}
	}

	var docs [][]byte
	for _, name := range names {
		data, err := get(baseURL + name)
		if err != nil {
			return nil, fmt.Errorf("fetch compose %s: %w", name, err)
		}
		if err := os.WriteFile(filepath.Join(cache, filepath.FromSlash(name)), data, 0o644); err != nil {
			return nil, err
		}
		docs = append(docs, data)
	}

	ps, err := ParseComposeFiles(docs)
	if err != nil {
		return nil, err
	}

	// Build contexts: fetch Dockerfile + every file it COPY/ADDs. A context
	// whose Dockerfile can't be fetched (even with fallback suffixes) makes its
	// service un-importable — recorded and dropped from the catalog.
	ctxFailed := map[string]string{}
	for _, ctx := range ps.BuildContexts {
		if err := r.fetchBuildContext(cache, baseURL, ctx, get); err != nil {
			ctxFailed[ctx] = err.Error()
		}
	}

	// Bind-mounted config files (e.g. ./etc/redis.conf, ./php74/php.ini).
	// Missing ones are non-fatal (the service still runs without that mount).
	for _, f := range ps.BindFiles {
		if err := r.fetchFile(cache, baseURL, f, get); err != nil {
			fmt.Printf("⚠️  repo %s: optional bind file %s not found\n", r.Name, f)
		}
	}

	// Repo env files (defaults for ${VAR} interpolation). Best-effort.
	for _, n := range []string{".env.example", ".env"} {
		if data, err := get(baseURL + n); err == nil {
			_ = os.WriteFile(filepath.Join(cache, n), data, 0o644)
		}
	}

	// Flag (but keep) services whose build context is unavailable. They stay in
	// the catalog marked with an ImportError; everything else imports normally.
	// We never drop the whole repository for one bad service.
	ps.Unimportable = map[string]string{}
	for i := range ps.Services {
		s := &ps.Services[i]
		if s.Build == nil {
			continue
		}
		if reason, bad := ctxFailed[s.Build.Context]; bad {
			s.ImportError = fmt.Sprintf("build context %q unavailable: %s", s.Build.Context, reason)
			ps.Unimportable[s.Name] = s.ImportError
		}
	}

	return ps, nil
}

// syncGit clones (or pulls) the git repository into the cache and parses the
// compose file(s) from the working tree. A clone brings the whole tree —
// compose, build contexts and config files — so no per-file fetching is needed.
func (r Repository) syncGit(workDir string) (*ParsedStack, error) {
	cache := r.CacheDir(workDir)

	repo, err := git.PlainOpen(cache)
	if err != nil {
		opts := &git.CloneOptions{URL: r.Location}
		if r.Ref != "" {
			opts.ReferenceName = plumbing.NewBranchReferenceName(r.Ref)
		}
		repo, err = git.PlainClone(cache, false, opts)
		if err != nil {
			return nil, fmt.Errorf("git clone %s: %w", r.Location, err)
		}
	} else if w, werr := repo.Worktree(); werr == nil {
		if r.Ref != "" {
			_ = w.Checkout(&git.CheckoutOptions{Branch: plumbing.NewBranchReferenceName(r.Ref)})
		}
		// Best-effort update; ignore "already up-to-date".
		_ = w.Pull(&git.PullOptions{RemoteName: "origin"})
	}

	return r.parseLocal(cache)
}

var dockerfileCopyRe = regexp.MustCompile(`(?mi)^\s*(?:COPY|ADD)\s+(.+)$`)

func (r Repository) fetchBuildContext(cache, baseURL, ctx string, get HTTPGetter) error {
	dfRel := path.Join(ctx, "Dockerfile")
	if err := r.fetchFile(cache, baseURL, dfRel, get); err != nil {
		return err
	}
	dfBytes, err := os.ReadFile(filepath.Join(cache, filepath.FromSlash(dfRel)))
	if err != nil {
		return err
	}
	for _, m := range dockerfileCopyRe.FindAllStringSubmatch(string(dfBytes), -1) {
		for _, src := range copySources(m[1]) {
			rel := path.Join(ctx, src)
			if err := r.fetchFile(cache, baseURL, rel, get); err != nil {
				fmt.Printf("⚠️  repo %s: %s: %v\n", r.Name, rel, err)
			}
		}
	}
	return nil
}

// copySources extracts the source operands of a COPY/ADD instruction, dropping
// the destination (last token), flags (--from=...), JSON-array brackets, and
// remote URLs.
func copySources(args string) []string {
	args = strings.TrimSpace(args)
	// COPY/ADD --from=<stage|image> copies from a build stage or another image,
	// not from the repository context — nothing to fetch.
	if strings.Contains(args, "--from=") {
		return nil
	}
	args = strings.TrimPrefix(args, "[")
	args = strings.TrimSuffix(args, "]")
	args = strings.ReplaceAll(args, `"`, "")
	args = strings.ReplaceAll(args, ",", " ")
	fields := strings.Fields(args)

	var srcs []string
	for _, f := range fields {
		if strings.HasPrefix(f, "--") {
			continue
		}
		srcs = append(srcs, f)
	}
	if len(srcs) <= 1 {
		return nil // only a destination, nothing to fetch
	}
	srcs = srcs[:len(srcs)-1] // drop destination
	var out []string
	for _, s := range srcs {
		if strings.Contains(s, "://") || strings.ContainsAny(s, "*?") {
			continue // remote URL or glob — skip
		}
		out = append(out, s)
	}
	return out
}

// fetchFallbackSuffixes are tried (in order) when the exact path 404s — many
// repos ship template files (foo.yml.dist, .env.example) whose runtime name has
// no suffix. A successful fallback is saved under the ORIGINAL (suffix-less)
// name so bind mounts still resolve.
var fetchFallbackSuffixes = []string{"", ".dist", ".example", ".sample"}

// fetchFile downloads <baseURL><rel> (trying fallback suffixes) into
// <cache>/<rel>, creating dirs. Returns the last error if every attempt fails.
func (r Repository) fetchFile(cache, baseURL, rel string, get HTTPGetter) error {
	var data []byte
	var err error
	for _, suf := range fetchFallbackSuffixes {
		data, err = get(baseURL + rel + suf)
		if err == nil {
			break
		}
	}
	if err != nil {
		return err
	}
	dst := filepath.Join(cache, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0o644)
}

// Registry is the merged view of all service sources: the default repository,
// any user-added repositories, and individual Hub-image services the user
// configures directly (no repository, no build).
//
// Persistence (memory DB) and controller wiring come in a later phase; for now
// the registry is constructed in memory.
type Registry struct {
	WorkDir string
	Repos   []Repository
	Custom  []ServiceSpec // single services added directly from a Hub image
	Get     HTTPGetter

	mu    sync.Mutex
	cache map[string]repoCache
	ttl   time.Duration
}

type repoCache struct {
	stack *ParsedStack
	at    time.Time
}

// NewRegistry returns a registry seeded with the default repository.
func NewRegistry(workDir string) *Registry {
	return &Registry{
		WorkDir: workDir,
		Repos: []Repository{
			{Name: DefaultRepoName, Kind: RepoComposeURL, Location: DefaultRepoComposeURL, Compose: "docker-compose.yml.dist", Enabled: true},
		},
		Get:   defaultHTTPGet,
		cache: map[string]repoCache{},
		ttl:   5 * time.Minute,
	}
}

// Invalidate clears the per-repository sync cache.
func (reg *Registry) Invalidate() {
	reg.mu.Lock()
	reg.cache = map[string]repoCache{}
	reg.mu.Unlock()
}

// syncCached returns a repository's parsed stack from cache when fresh, or
// (re)syncs and caches it. force bypasses the cache (explicit sync).
func (reg *Registry) syncCached(repo Repository, force bool) (*ParsedStack, error) {
	if !force {
		reg.mu.Lock()
		ce, ok := reg.cache[repo.Name]
		reg.mu.Unlock()
		if ok && time.Since(ce.at) < reg.ttl {
			return ce.stack, nil
		}
	}
	ps, err := repo.Sync(reg.WorkDir, reg.Get)
	if err != nil {
		return nil, err
	}
	reg.mu.Lock()
	if reg.cache == nil {
		reg.cache = map[string]repoCache{}
	}
	reg.cache[repo.Name] = repoCache{stack: ps, at: time.Now()}
	reg.mu.Unlock()
	return ps, nil
}

// AddCustomService registers a single Hub-image service. Build must be nil.
func (reg *Registry) AddCustomService(s ServiceSpec) error {
	if s.Build != nil {
		return fmt.Errorf("custom service %q must use an image, not a build context", s.Name)
	}
	if s.Image == "" {
		return fmt.Errorf("custom service %q requires an image", s.Name)
	}
	if s.ContainerName == "" {
		s.ContainerName = s.Name
	}
	reg.Custom = append(reg.Custom, s)
	return nil
}

// Effective syncs all enabled repositories and merges their services with the
// custom Hub-image services. Precedence (highest wins): custom > later repos >
// earlier repos. Returns the merged catalog plus any per-repo sync errors.
func (reg *Registry) Effective(force bool) ([]ServiceSpec, map[string]error) {
	merged := map[string]ServiceSpec{}
	errs := map[string]error{}

	for _, repo := range reg.Repos {
		if !repo.Enabled {
			continue
		}
		ps, err := reg.syncCached(repo, force)
		if err != nil {
			errs[repo.Name] = err
			continue
		}
		cache := repo.CacheDir(reg.WorkDir)
		for _, s := range ps.Services {
			s.SourceDir = cache
			s.Repo = repo.Name
			merged[s.Name] = s
		}
		for name, reason := range ps.Unimportable {
			errs["service:"+name] = fmt.Errorf("%s", reason)
		}
	}
	for _, s := range reg.Custom {
		s.Repo = "custom"
		merged[s.Name] = s
	}

	out := make([]ServiceSpec, 0, len(merged))
	for _, s := range merged {
		out = append(out, s)
	}
	return out, errs
}

// ImportZipRepository extracts an uploaded zip (a stack with a compose file and
// build contexts) into the repository cache and registers it as a local repo.
// Protects against zip-slip path traversal.
func (m *Manager) ImportZipRepository(name, compose string, zr *zip.Reader) error {
	if name == "" {
		return fmt.Errorf("repository requires a name")
	}
	dir := filepath.Join(m.WorkDir, "repositories", name)
	_ = os.RemoveAll(dir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	base, _ := filepath.Abs(dir)
	for _, f := range zr.File {
		rel := strings.TrimPrefix(filepath.Clean("/"+f.Name), "/")
		if rel == "" {
			continue
		}
		dst := filepath.Join(dir, rel)
		if abs, _ := filepath.Abs(dst); !strings.HasPrefix(abs, base+string(os.PathSeparator)) {
			continue // zip-slip guard
		}
		if f.FileInfo().IsDir() {
			_ = os.MkdirAll(dst, 0o755)
			continue
		}
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		out, err := os.Create(dst)
		if err != nil {
			rc.Close()
			return err
		}
		_, copyErr := io.Copy(out, rc)
		out.Close()
		rc.Close()
		if copyErr != nil {
			return copyErr
		}
	}
	return m.AddRepository(Repository{Name: name, Kind: RepoLocal, Location: dir, Compose: compose, Enabled: true})
}
