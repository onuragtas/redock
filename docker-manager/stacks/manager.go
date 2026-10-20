package stacks

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"

	"redock/platform/database"
	"redock/platform/memory"
)

var (
	singleton     *Manager
	singletonOnce sync.Once
	singletonErr  error
)

// GetManager returns the process-wide stacks Manager, constructing it once
// against the given work dir (the redock-managed equivalent of
// ~/.docker-environment).
func GetManager(workDir string) (*Manager, error) {
	singletonOnce.Do(func() {
		singleton, singletonErr = NewManager(workDir)
	})
	return singleton, singletonErr
}

// Enabled reports whether stacks orchestration is turned on via the
// USE_NATIVE_ORCHESTRATION env var.
func Enabled() bool {
	return true
	// return os.Getenv("USE_NATIVE_ORCHESTRATION") == "1"
}

// Manager is the stacks orchestration entrypoint: it owns the repository
// registry, the env model, and the Docker engine, and exposes the lifecycle
// operations (Up/Down/Restart/Status/...) that the controllers call. State
// (repositories, custom services, active set) is persisted in the memory DB.
type Manager struct {
	WorkDir  string
	Registry *Registry
	Engine   *Engine
	Env      *EnvModel
	db       *memory.Database

	// mu guards Registry.Repos/Custom and Env (short critical sections; may
	// include a repository sync). opMu serializes container lifecycle work
	// (Up/Down/Restart/Apply) so concurrent requests can't interleave
	// recreates of the same container. Lock order: opMu before mu.
	mu   sync.Mutex
	opMu sync.Mutex
}

// namePattern restricts repository / custom-service names: they become
// directory names under the work dir (and are passed to os.RemoveAll), so
// separators and dot-only names must never get through.
var namePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,63}$`)

func validateName(kind, name string) error {
	if !namePattern.MatchString(name) || strings.Contains(name, "..") {
		return fmt.Errorf("invalid %s name %q: use letters, digits, '-', '_' or '.'", kind, name)
	}
	return nil
}

// NewManager constructs the manager, loading persisted state (or seeding the
// default repository on first run).
func NewManager(workDir string) (*Manager, error) {
	m := &Manager{
		WorkDir:  workDir,
		Registry: NewRegistry(workDir),
		Env:      loadEnv(workDir),
		db:       database.GetMemoryDB(),
	}
	m.loadFromDB()

	eng, err := NewEngine(workDir)
	if err != nil {
		return nil, err
	}
	m.Engine = eng
	return m, nil
}

// loadEnv reads <workDir>/.env if present (the file the existing UI maintains),
// falling back to an empty model.
func loadEnv(workDir string) *EnvModel {
	if env, err := LoadEnvFile(filepath.Join(workDir, ".env")); err == nil {
		return env
	}
	return NewEnvModel()
}

// ReloadEnv re-reads the .env file (call after the user edits it).
func (m *Manager) ReloadEnv() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.rebuildEnv()
}

func (m *Manager) loadFromDB() {
	if m.db == nil {
		return // in-memory default registry already seeded
	}
	reps := memory.FindAll[*RepositoryEntity](m.db, TableRepositories)
	if len(reps) == 0 {
		def := &RepositoryEntity{
			Name: DefaultRepoName, Kind: RepoComposeURL,
			Location: DefaultRepoComposeURL, Compose: "docker-compose.yml.dist", Enabled: true,
		}
		_ = memory.Create(m.db, TableRepositories, def)
		m.Registry.Repos = []Repository{def.toRepository()}
	} else {
		m.Registry.Repos = nil
		for _, e := range reps {
			m.Registry.Repos = append(m.Registry.Repos, e.toRepository())
		}
	}
	m.Registry.Custom = nil
	for _, c := range memory.FindAll[*CustomServiceEntity](m.db, TableCustomServices) {
		m.Registry.Custom = append(m.Registry.Custom, c.Spec)
	}
}

// --- catalog ---

// Catalog returns the effective, env-resolved catalog (default + user repos +
// custom services) and any per-repository sync errors.
func (m *Manager) Catalog() ([]ServiceSpec, map[string]error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.catalogLocked()
}

func (m *Manager) catalogLocked() ([]ServiceSpec, map[string]error) {
	specs, errs := m.Registry.Effective(false)
	m.rebuildEnv()
	for i := range specs {
		specs[i] = m.Env.Resolve(specs[i])
		specs[i].SourceDir = specsSourceDir(m.Registry, specs[i]) // keep after Resolve
	}
	sort.Slice(specs, func(i, j int) bool { return specs[i].Name < specs[j].Name })
	return specs, errs
}

// rebuildEnv layers the env model for ${VAR} interpolation: each enabled
// repository's .env.example (defaults) then .env, overlaid by the global
// <workDir>/.env (user overrides, highest precedence).
func (m *Manager) rebuildEnv() {
	merged := NewEnvModel()
	for _, repo := range m.Registry.Repos {
		if !repo.Enabled {
			continue
		}
		cache := repo.CacheDir(m.WorkDir)
		mergeEnvFile(merged, filepath.Join(cache, ".env.example"))
		mergeEnvFile(merged, filepath.Join(cache, ".env"))
	}
	mergeEnvFile(merged, filepath.Join(m.WorkDir, ".env"))
	m.Env = merged
}

func mergeEnvFile(m *EnvModel, path string) {
	if e, err := LoadEnvFile(path); err == nil {
		for k, v := range e.All() {
			m.Set(k, v)
		}
	}
}

// EnvVar is one environment variable for the structured stacks env editor.
type EnvVar struct {
	Key        string   `json:"key"`
	Value      string   `json:"value"`      // effective value (default overlaid by override)
	Default    string   `json:"default"`    // repo-provided default ("" if none)
	Overridden bool     `json:"overridden"` // set in the global .env
	Repo       string   `json:"repo"`       // repository that provides the default
	Repos      []string `json:"repos"`      // all associated repos (source .env + using services')
	Services   []string `json:"services"`   // services that reference ${KEY}
}

// repoDefaults merges the .env.example + .env of every enabled repository.
func (m *Manager) repoDefaults() *EnvModel {
	d := NewEnvModel()
	for _, repo := range m.Registry.Repos {
		if !repo.Enabled {
			continue
		}
		cache := repo.CacheDir(m.WorkDir)
		mergeEnvFile(d, filepath.Join(cache, ".env.example"))
		mergeEnvFile(d, filepath.Join(cache, ".env"))
	}
	return d
}

// EnvVars returns every variable (repo defaults + global overrides + any keys
// referenced by services) with its effective value, default, override flag,
// source repository, and the services that reference it — sorted by key.
func (m *Manager) EnvVars() []EnvVar {
	m.mu.Lock()
	defer m.mu.Unlock()
	specs, _ := m.Registry.Effective(false) // unresolved specs (so ${VAR} survive)

	// Defaults + the repository that provides each one.
	defaults := map[string]string{}
	srcRepo := map[string]string{}
	for _, repo := range m.Registry.Repos {
		if !repo.Enabled {
			continue
		}
		cache := repo.CacheDir(m.WorkDir)
		for _, f := range []string{".env.example", ".env"} {
			if e, err := LoadEnvFile(filepath.Join(cache, f)); err == nil {
				for k, v := range e.All() {
					defaults[k] = v
					srcRepo[k] = repo.Name
				}
			}
		}
	}

	global := NewEnvModel()
	mergeEnvFile(global, filepath.Join(m.WorkDir, ".env"))
	overrides := global.All()

	// Service → repo (for attributing referenced vars to repositories).
	serviceRepo := map[string]string{}
	for _, s := range specs {
		serviceRepo[s.Name] = s.Repo
	}

	// Which services reference each ${KEY}, inline defaults, and the set of
	// repositories associated with each var (using services' repos).
	usage := map[string]map[string]bool{}
	inlineDef := map[string]string{}
	varRepos := map[string]map[string]bool{}
	for _, s := range specs {
		for k, def := range specEnvRefs(s) {
			if usage[k] == nil {
				usage[k] = map[string]bool{}
				varRepos[k] = map[string]bool{}
			}
			usage[k][s.Name] = true
			if s.Repo != "" {
				varRepos[k][s.Repo] = true
			}
			if def != "" && inlineDef[k] == "" {
				inlineDef[k] = def
			}
		}
	}

	seen := map[string]bool{}
	var keys []string
	add := func(k string) {
		if k != "" && !seen[k] {
			seen[k] = true
			keys = append(keys, k)
		}
	}
	for k := range defaults {
		add(k)
	}
	for k := range overrides {
		add(k)
	}
	for k := range usage {
		add(k)
	}
	sort.Strings(keys)

	out := make([]EnvVar, 0, len(keys))
	for _, k := range keys {
		// Default: repo .env value, else the inline ${VAR:-default} from compose.
		def, hasRepoDefault := defaults[k]
		if !hasRepoDefault {
			def = inlineDef[k]
		}
		val := def
		_, isOverride := overrides[k]
		if isOverride {
			val = overrides[k]
		}
		svcs := make([]string, 0, len(usage[k]))
		for s := range usage[k] {
			svcs = append(svcs, s)
		}
		sort.Strings(svcs)

		repoSet := map[string]bool{}
		if srcRepo[k] != "" {
			repoSet[srcRepo[k]] = true
		}
		for r := range varRepos[k] {
			repoSet[r] = true
		}
		repos := make([]string, 0, len(repoSet))
		for r := range repoSet {
			repos = append(repos, r)
		}
		sort.Strings(repos)

		out = append(out, EnvVar{
			Key: k, Value: val, Default: def,
			Overridden: isOverride, Repo: srcRepo[k], Repos: repos, Services: svcs,
		})
	}
	return out
}

// specEnvRefs returns the ${VAR} keys referenced by a (unresolved) spec, mapped
// to their inline ${VAR:-default} default ("" if none).
func specEnvRefs(s ServiceSpec) map[string]string {
	refs := map[string]string{}
	scan := func(strs ...string) {
		for _, str := range strs {
			for _, mtch := range interpRe.FindAllStringSubmatch(str, -1) {
				if _, ok := refs[mtch[1]]; !ok || (refs[mtch[1]] == "" && mtch[3] != "") {
					refs[mtch[1]] = mtch[3]
				}
			}
		}
	}
	scan(s.Image, s.Hostname, s.StaticIP, s.WorkingDir, s.User)
	scan(s.Command...)
	scan(s.Entrypoint...)
	scan(s.Aliases...)
	scan(s.DependsOn...)
	scan(s.ExtraHosts...)
	scan(s.DNS...)
	scan(s.Tmpfs...)
	for _, p := range s.Ports {
		scan(p.Host, p.Container)
	}
	for _, v := range s.Volumes {
		scan(v.Source, v.Target)
	}
	for _, val := range s.Env {
		scan(val)
	}
	for _, val := range s.Labels {
		scan(val)
	}
	return refs
}

// SaveEnvVars writes the global .env with only the values that differ from the
// repo defaults (plus any custom keys), keeping the override file minimal.
func (m *Manager) SaveEnvVars(vars map[string]string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	defaults := m.repoDefaults().All()
	keys := make([]string, 0, len(vars))
	for k, v := range vars {
		if strings.ContainsAny(k, "=\r\n") || strings.ContainsAny(v, "\r\n") {
			return fmt.Errorf("invalid variable %q: keys can't contain '=' or line breaks, values can't contain line breaks", k)
		}
		if k != "" {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, k := range keys {
		v := vars[k]
		if dv, ok := defaults[k]; ok && dv == v {
			continue // equals repo default → no need to persist an override
		}
		b.WriteString(k)
		b.WriteString("=")
		b.WriteString(v)
		b.WriteString("\n")
	}
	if err := os.WriteFile(filepath.Join(m.WorkDir, ".env"), []byte(b.String()), 0o644); err != nil {
		return err
	}
	m.rebuildEnv()
	return nil
}

// specsSourceDir preserves the SourceDir set by Effective (Resolve copies the
// struct, so it is retained; this is a guard for clarity/no-op).
func specsSourceDir(_ *Registry, s ServiceSpec) string { return s.SourceDir }

func (m *Manager) catalogMap() map[string]ServiceSpec {
	specs, _ := m.Catalog()
	out := make(map[string]ServiceSpec, len(specs))
	for _, s := range specs {
		out[s.Name] = s
	}
	return out
}

// Sync force-refreshes all enabled repositories (bypasses the cache).
func (m *Manager) Sync() map[string]error {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, errs := m.Registry.Effective(true)
	return errs
}

// --- lifecycle ---

// plan resolves names (plus transitive dependencies) into start order and
// returns each service materialized against the current env, all under one
// consistent snapshot of the registry/env.
func (m *Manager) plan(names []string, withDeps bool) ([]ServiceSpec, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	specs, _ := m.catalogLocked()
	cat := make(map[string]ServiceSpec, len(specs))
	for _, s := range specs {
		cat[s.Name] = s
	}
	order := names
	if withDeps {
		var err error
		if order, err = m.resolveOrder(names, cat); err != nil {
			return nil, err
		}
	}
	out := make([]ServiceSpec, 0, len(order))
	for _, name := range order {
		spec, ok := cat[name]
		if !ok {
			return nil, fmt.Errorf("service %q not found in catalog", name)
		}
		if spec.ImportError != "" {
			return nil, fmt.Errorf("cannot start %q: %s", name, spec.ImportError)
		}
		out = append(out, m.materialize(spec, true))
	}
	return out, nil
}

// Up starts the named services (and their transitive dependencies) in
// dependency order, building/pulling images as needed, and marks them active.
// Every container is recreated, so the current configuration always applies.
func (m *Manager) Up(ctx context.Context, names ...string) error {
	m.opMu.Lock()
	defer m.opMu.Unlock()
	order, err := m.plan(names, true)
	if err != nil {
		return err
	}
	if err := m.Engine.EnsureNetwork(ctx); err != nil {
		return err
	}
	// Seed base web configs so nginx/httpd work without the legacy git clone.
	m.SeedWebConfig()
	for _, s := range order {
		if err := m.create(ctx, s, true); err != nil {
			return err
		}
		m.markActive(s.Name)
	}
	return nil
}

// create ensures volumes and the image, then (re)creates the container.
// pull=false only fetches/builds the image when it is missing locally (used
// when recreating to apply a config edit, which shouldn't upgrade images).
func (m *Manager) create(ctx context.Context, s ServiceSpec, pull bool) error {
	for _, v := range s.Volumes {
		if v.Kind == VolumeNamed {
			if err := m.Engine.EnsureVolume(ctx, v.Source); err != nil {
				return err
			}
		}
	}
	if pull || !m.Engine.HasImage(ctx, s) {
		var buildFS fs.FS
		if s.Build != nil {
			buildFS = os.DirFS(s.SourceDir)
		}
		if err := m.Engine.EnsureImage(ctx, s, buildFS, os.Stdout); err != nil {
			return fmt.Errorf("ensure image for %s: %w", s.Name, err)
		}
	}
	if _, err := m.Engine.CreateAndStart(ctx, s); err != nil {
		return err
	}
	return nil
}

// Down stops and removes a service's container and clears its active mark.
func (m *Manager) Down(ctx context.Context, name string) error {
	m.opMu.Lock()
	defer m.opMu.Unlock()
	cat := m.catalogMap()
	containerName := name
	if s, ok := cat[name]; ok && s.ContainerName != "" {
		containerName = s.ContainerName
	}
	if err := m.Engine.Remove(ctx, containerName); err != nil {
		return err
	}
	m.unmarkActive(name)
	return nil
}

// Restart restarts a service's container. If its configuration changed since
// the container was created (env vars, ports, image, ...), the container is
// recreated instead — a plain restart would keep the old configuration.
func (m *Manager) Restart(ctx context.Context, name string) error {
	m.opMu.Lock()
	defer m.opMu.Unlock()
	specs, err := m.plan([]string{name}, false)
	if err != nil {
		// Not in the catalog (e.g. a devenv container): plain restart by name.
		return m.Engine.Restart(ctx, name)
	}
	s := specs[0]
	exists, outdated, err := m.Engine.Outdated(ctx, s)
	if err != nil {
		return err
	}
	if exists && !outdated {
		return m.Engine.Restart(ctx, containerName(s))
	}
	if err := m.Engine.EnsureNetwork(ctx); err != nil {
		return err
	}
	return m.create(ctx, s, false)
}

// ApplyChanges recreates every active service whose container exists but no
// longer matches its configuration (after an env or service edit), so edits
// take effect without the user deleting containers. Returns the recreated
// services; on error, those recreated before it.
func (m *Manager) ApplyChanges(ctx context.Context) ([]string, error) {
	m.opMu.Lock()
	defer m.opMu.Unlock()
	active := m.Active()
	specs, err := m.planActive(active)
	if err != nil {
		return nil, err
	}
	var applied []string
	networkReady := false
	for _, s := range specs {
		exists, outdated, err := m.Engine.Outdated(ctx, s)
		if err != nil || !exists || !outdated {
			continue
		}
		if !networkReady {
			if err := m.Engine.EnsureNetwork(ctx); err != nil {
				return applied, err
			}
			networkReady = true
		}
		if err := m.create(ctx, s, false); err != nil {
			return applied, fmt.Errorf("apply %s: %w", s.Name, err)
		}
		applied = append(applied, s.Name)
	}
	return applied, nil
}

// planActive materializes the given (active) services that are still in the
// catalog, skipping ones that disappeared or can't be imported.
func (m *Manager) planActive(names []string) ([]ServiceSpec, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	specs, _ := m.catalogLocked()
	want := make(map[string]bool, len(names))
	for _, n := range names {
		want[n] = true
	}
	var out []ServiceSpec
	for _, s := range specs {
		if want[s.Name] && s.ImportError == "" {
			out = append(out, m.materialize(s, true))
		}
	}
	return out, nil
}

// ServiceStatus is the runtime state of one catalog service.
type ServiceStatus struct {
	Name      string `json:"name"`
	Container string `json:"container"`
	Category  string `json:"category"`
	Active    bool   `json:"active"`   // user-activated (persisted)
	Exists    bool   `json:"exists"`   // container present
	Running   bool   `json:"running"`  // container running
	Outdated  bool   `json:"outdated"` // config changed since the container was created
	State     string `json:"state"`
}

// Status returns the runtime status of every catalog service.
func (m *Manager) Status(ctx context.Context) ([]ServiceStatus, error) {
	m.mu.Lock()
	specs, _ := m.catalogLocked()
	resolved := make([]ServiceSpec, len(specs))
	for i, s := range specs {
		resolved[i] = m.materialize(s, false)
	}
	m.mu.Unlock()

	active := m.activeSet()
	seed := !m.metaFlag(metaActiveSeeded)
	out := make([]ServiceStatus, 0, len(specs))
	for i, s := range specs {
		cn := containerName(s)
		exists, running, state, _ := m.Engine.Status(ctx, cn)
		// First-run migration: a service whose container already exists was
		// started by the previous (docker-compose) stack — adopt it as active so
		// the UI reflects reality instead of showing everything as stopped.
		if seed && exists && !active[s.Name] {
			m.markActive(s.Name)
			active[s.Name] = true
		}
		outdated := false
		if exists && s.ImportError == "" {
			_, outdated, _ = m.Engine.Outdated(ctx, resolved[i])
		}
		out = append(out, ServiceStatus{
			Name: s.Name, Container: cn, Category: s.Category,
			Active: active[s.Name], Exists: exists, Running: running, Outdated: outdated, State: state,
		})
	}
	if seed {
		m.setMetaFlag(metaActiveSeeded)
	}
	return out, nil
}

// materialize env-resolves a spec and turns repo-relative bind sources into
// absolute host paths: repository config files resolve to the repo cache, other
// relative dirs (logs, data, ...) to the work dir (created on demand).
// create=false computes the same result without creating missing runtime
// directories (used by read-only callers such as Status).
// Caller must hold m.mu.
func (m *Manager) materialize(spec ServiceSpec, create bool) ServiceSpec {
	s := m.Env.Resolve(spec)
	s.SourceDir = spec.SourceDir

	// env_file provides a base layer that explicit `environment` entries override
	// (docker-compose semantics). Files are repo-relative (resolve against the
	// repo cache) or work-dir-relative.
	if len(s.EnvFile) > 0 {
		base := map[string]string{}
		for _, f := range s.EnvFile {
			rel := strings.TrimPrefix(strings.TrimRight(f, "/"), "./")
			path := ""
			if s.SourceDir != "" && pathExists(filepath.Join(s.SourceDir, rel)) {
				path = filepath.Join(s.SourceDir, rel)
			} else if pathExists(filepath.Join(m.WorkDir, rel)) {
				path = filepath.Join(m.WorkDir, rel)
			} else if filepath.IsAbs(f) && pathExists(f) {
				path = f
			}
			if path == "" {
				continue
			}
			if e, err := LoadEnvFile(path); err == nil {
				for k, v := range e.All() {
					base[k] = m.Env.Expand(v)
				}
			}
		}
		for k, v := range s.Env {
			base[k] = v
		}
		s.Env = base
	}

	// Named volumes are project-prefixed by docker-compose (e.g. lemp_postgres_data);
	// mirror that prefix so existing data created by the old compose is reused.
	prefix := m.Env.Get("COMPOSE_PROJECT_NAME")

	for i := range s.Volumes {
		v := &s.Volumes[i]
		if v.Kind == VolumeNamed {
			if prefix != "" && !strings.HasPrefix(v.Source, prefix+"_") {
				v.Source = prefix + "_" + v.Source
			}
			continue
		}
		if v.Kind != VolumeBind {
			continue
		}
		// Redirect nginx/httpd config mounts to the redock-managed, writable
		// vhost dirs (so user virtual hosts survive repository syncs).
		if override, ok := m.vhostBindOverride(v.Target); ok {
			v.Source = override
			continue
		}
		src := v.Source
		if src == "" || filepath.IsAbs(src) {
			continue
		}
		rel := strings.TrimPrefix(strings.TrimRight(src, "/"), "./")
		if s.SourceDir != "" {
			if cand := filepath.Join(s.SourceDir, rel); pathExists(cand) {
				v.Source = cand
				continue
			}
		}
		// Runtime path under the work dir; ensure the directory exists.
		wd := filepath.Join(m.WorkDir, rel)
		if create && !pathExists(wd) {
			_ = os.MkdirAll(wd, 0o755)
		}
		v.Source = wd
	}
	return s
}

func pathExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

// --- dependency ordering ---

// resolveOrder expands the requested services with their transitive
// dependencies and returns them in start order (dependencies first).
func (m *Manager) resolveOrder(names []string, cat map[string]ServiceSpec) ([]string, error) {
	closure := map[string]bool{}
	var visit func(string) error
	var stack []string
	visit = func(n string) error {
		if closure[n] {
			return nil
		}
		for _, s := range stack {
			if s == n {
				return fmt.Errorf("dependency cycle at %q", n)
			}
		}
		spec, ok := cat[n]
		if !ok {
			return fmt.Errorf("service %q not found", n)
		}
		stack = append(stack, n)
		for _, dep := range spec.DependsOn {
			if _, ok := cat[dep]; ok {
				if err := visit(dep); err != nil {
					return err
				}
			}
		}
		stack = stack[:len(stack)-1]
		closure[n] = true
		return nil
	}
	for _, n := range names {
		if err := visit(n); err != nil {
			return nil, err
		}
	}

	// Kahn topological sort over the closure for deterministic order.
	indeg := map[string]int{}
	for n := range closure {
		indeg[n] = 0
	}
	for n := range closure {
		for _, dep := range cat[n].DependsOn {
			if closure[dep] {
				indeg[n]++
			}
		}
	}
	var ready []string
	for n, d := range indeg {
		if d == 0 {
			ready = append(ready, n)
		}
	}
	sort.Strings(ready)
	var order []string
	for len(ready) > 0 {
		n := ready[0]
		ready = ready[1:]
		order = append(order, n)
		var newly []string
		for m2 := range closure {
			for _, dep := range cat[m2].DependsOn {
				if dep == n {
					indeg[m2]--
					if indeg[m2] == 0 {
						newly = append(newly, m2)
					}
				}
			}
		}
		sort.Strings(newly)
		ready = append(ready, newly...)
	}
	if len(order) != len(closure) {
		return nil, fmt.Errorf("dependency cycle detected")
	}
	return order, nil
}

// --- active-set persistence ---

// metaActiveSeeded marks that the one-time active-service seed (adopting
// containers from the previous docker-compose stack) has run.
const metaActiveSeeded = "active_seeded"

func (m *Manager) metaFlag(key string) bool {
	if m.db == nil {
		return true // no DB: skip one-time migrations
	}
	for _, e := range memory.FindAll[*MetaEntity](m.db, TableMeta) {
		if e.Key == key {
			return true
		}
	}
	return false
}

func (m *Manager) setMetaFlag(key string) {
	if m.db == nil || m.metaFlag(key) {
		return
	}
	_ = memory.Create(m.db, TableMeta, &MetaEntity{Key: key, Value: "1"})
}

func (m *Manager) activeSet() map[string]bool {
	out := map[string]bool{}
	if m.db == nil {
		return out
	}
	for _, e := range memory.FindAll[*ActiveServiceEntity](m.db, TableActiveServices) {
		out[e.Name] = true
	}
	return out
}

// Active returns the names of user-activated services.
func (m *Manager) Active() []string {
	set := m.activeSet()
	out := make([]string, 0, len(set))
	for n := range set {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

func (m *Manager) markActive(name string) {
	if m.db == nil {
		return
	}
	for _, e := range memory.FindAll[*ActiveServiceEntity](m.db, TableActiveServices) {
		if e.Name == name {
			return
		}
	}
	_ = memory.Create(m.db, TableActiveServices, &ActiveServiceEntity{Name: name})
}

func (m *Manager) unmarkActive(name string) {
	if m.db == nil {
		return
	}
	for _, e := range memory.FindAll[*ActiveServiceEntity](m.db, TableActiveServices) {
		if e.Name == name {
			_ = memory.Delete[*ActiveServiceEntity](m.db, TableActiveServices, e.GetID())
			return
		}
	}
}

// --- repository / custom-service management (persisted) ---

// AddRepository registers and persists a new repository.
func (m *Manager) AddRepository(r Repository) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.addRepositoryLocked(r)
}

func (m *Manager) hasRepositoryLocked(name string) bool {
	for _, r := range m.Registry.Repos {
		if r.Name == name {
			return true
		}
	}
	return false
}

func (m *Manager) addRepositoryLocked(r Repository) error {
	if err := validateName("repository", r.Name); err != nil {
		return err
	}
	if r.Location == "" {
		return fmt.Errorf("repository requires a name and location")
	}
	if m.hasRepositoryLocked(r.Name) {
		return fmt.Errorf("repository %q already exists", r.Name)
	}
	if m.db != nil {
		e := &RepositoryEntity{Name: r.Name, Kind: r.Kind, Location: r.Location, Compose: r.Compose, Ref: r.Ref, Enabled: r.Enabled}
		if err := memory.Create(m.db, TableRepositories, e); err != nil {
			return err
		}
	}
	m.Registry.Repos = append(m.Registry.Repos, r)
	return nil
}

// SetRepositoryEnabled toggles a repository on/off without removing it. A
// disabled repo contributes no services or env defaults to the catalog.
func (m *Manager) SetRepositoryEnabled(name string, enabled bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	found := false
	for i := range m.Registry.Repos {
		if m.Registry.Repos[i].Name == name {
			m.Registry.Repos[i].Enabled = enabled
			found = true
		}
	}
	if !found {
		return fmt.Errorf("repository %q not found", name)
	}
	if m.db != nil {
		for _, e := range memory.FindAll[*RepositoryEntity](m.db, TableRepositories) {
			if e.Name == name {
				e.Enabled = enabled
				_ = memory.Update(m.db, TableRepositories, e)
			}
		}
	}
	m.Registry.Invalidate()
	return nil
}

// UpdateRepository edits an existing repository's source (kind/location/compose).
// The name is the immutable key; the cache is invalidated so the next sync
// re-fetches from the new location.
func (m *Manager) UpdateRepository(name string, r Repository) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := validateName("repository", name); err != nil {
		return err
	}
	if r.Location == "" {
		return fmt.Errorf("repository requires a location")
	}
	found := false
	for i := range m.Registry.Repos {
		if m.Registry.Repos[i].Name == name {
			r.Name = name
			// Enabled is toggled through SetRepositoryEnabled; an edit of the
			// source must not silently switch the repository off.
			r.Enabled = m.Registry.Repos[i].Enabled
			m.Registry.Repos[i] = r
			found = true
		}
	}
	if !found {
		return fmt.Errorf("repository %q not found", name)
	}
	if m.db != nil {
		for _, e := range memory.FindAll[*RepositoryEntity](m.db, TableRepositories) {
			if e.Name == name {
				e.Kind, e.Location, e.Compose, e.Ref = r.Kind, r.Location, r.Compose, r.Ref
				_ = memory.Update(m.db, TableRepositories, e)
			}
		}
	}
	// Drop the stale on-disk cache for remote repos so the new source is fetched.
	if r.Kind != RepoLocal {
		_ = os.RemoveAll(filepath.Join(m.WorkDir, "repositories", name))
	}
	m.Registry.Invalidate()
	return nil
}

// RemoveRepository removes a repository by name (the default repo is protected).
func (m *Manager) RemoveRepository(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := validateName("repository", name); err != nil {
		return err
	}
	if name == DefaultRepoName {
		return fmt.Errorf("the default repository cannot be removed")
	}
	var removed *Repository
	for i := range m.Registry.Repos {
		if m.Registry.Repos[i].Name == name {
			r := m.Registry.Repos[i]
			removed = &r
		}
	}
	if m.db != nil {
		for _, e := range memory.FindAll[*RepositoryEntity](m.db, TableRepositories) {
			if e.Name == name {
				_ = memory.Delete[*RepositoryEntity](m.db, TableRepositories, e.GetID())
			}
		}
	}
	kept := m.Registry.Repos[:0]
	for _, r := range m.Registry.Repos {
		if r.Name != name {
			kept = append(kept, r)
		}
	}
	m.Registry.Repos = kept
	// Clean up the on-disk cache (never delete a user's local source folder).
	if removed != nil && removed.Kind != RepoLocal {
		_ = os.RemoveAll(filepath.Join(m.WorkDir, "repositories", name))
	}
	m.Registry.Invalidate()
	return nil
}

// AddCustomService registers and persists a single Hub-image service.
func (m *Manager) AddCustomService(s ServiceSpec) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.checkNewCustomLocked(s.Name); err != nil {
		return err
	}
	if err := m.Registry.AddCustomService(s); err != nil {
		return err
	}
	if s.ContainerName == "" {
		s.ContainerName = s.Name
	}
	if m.db != nil {
		return memory.Create(m.db, TableCustomServices, &CustomServiceEntity{Spec: s})
	}
	return nil
}

// AddCustomBuildService registers a single service built from an inline
// Dockerfile (+ optional extra context files, e.g. an entrypoint script). The
// context is written under <workDir>/custom/<name>/ and built via the SDK.
func (m *Manager) checkNewCustomLocked(name string) error {
	if err := validateName("service", name); err != nil {
		return err
	}
	for _, c := range m.Registry.Custom {
		if c.Name == name {
			return fmt.Errorf("custom service %q already exists", name)
		}
	}
	return nil
}

func (m *Manager) AddCustomBuildService(s ServiceSpec, dockerfile string, files map[string]string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.checkNewCustomLocked(s.Name); err != nil {
		return err
	}
	if dockerfile == "" {
		return fmt.Errorf("service %q requires a Dockerfile", s.Name)
	}
	dir := filepath.Join(m.WorkDir, "custom", s.Name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "Dockerfile"), []byte(dockerfile), 0o644); err != nil {
		return err
	}
	for rel, content := range files {
		rel = strings.TrimPrefix(filepath.Clean("/"+rel), "/") // prevent path traversal
		if rel == "" {
			continue
		}
		dst := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(dst, []byte(content), 0o644); err != nil {
			return err
		}
	}

	s.Image = ""
	s.Build = &BuildSpec{Context: s.Name, Tag: "redock/" + s.Name}
	s.SourceDir = filepath.Join(m.WorkDir, "custom")
	if s.ContainerName == "" {
		s.ContainerName = s.Name
	}
	s.Repo = "custom"

	m.Registry.Custom = append(m.Registry.Custom, s)
	if m.db != nil {
		return memory.Create(m.db, TableCustomServices, &CustomServiceEntity{Spec: s})
	}
	return nil
}

// UpdateCustomService edits an existing custom (Hub-image) service in place. The
// name is the immutable key. Build-based custom services keep their build dir.
func (m *Manager) UpdateCustomService(name string, s ServiceSpec) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := validateName("service", name); err != nil {
		return err
	}
	s.Name = name
	if s.ContainerName == "" {
		s.ContainerName = name
	}
	found := false
	for i := range m.Registry.Custom {
		if m.Registry.Custom[i].Name == name {
			// Preserve build metadata so an image-edit doesn't drop the Dockerfile.
			if s.Build == nil {
				s.Build = m.Registry.Custom[i].Build
				s.SourceDir = m.Registry.Custom[i].SourceDir
			}
			s.Repo = m.Registry.Custom[i].Repo
			m.Registry.Custom[i] = s
			found = true
		}
	}
	if !found {
		return fmt.Errorf("custom service %q not found", name)
	}
	if m.db != nil {
		for _, e := range memory.FindAll[*CustomServiceEntity](m.db, TableCustomServices) {
			if e.Spec.Name == name {
				e.Spec = s
				_ = memory.Update(m.db, TableCustomServices, e)
			}
		}
	}
	m.Registry.Invalidate()
	return nil
}

// RemoveCustomService removes a custom service by name.
func (m *Manager) RemoveCustomService(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := validateName("service", name); err != nil {
		return err
	}
	if m.db != nil {
		for _, e := range memory.FindAll[*CustomServiceEntity](m.db, TableCustomServices) {
			if e.Spec.Name == name {
				_ = memory.Delete[*CustomServiceEntity](m.db, TableCustomServices, e.GetID())
			}
		}
	}
	kept := m.Registry.Custom[:0]
	for _, s := range m.Registry.Custom {
		if s.Name != name {
			kept = append(kept, s)
		}
	}
	m.Registry.Custom = kept
	// Remove the inline build context written by AddCustomBuildService (if any).
	_ = os.RemoveAll(filepath.Join(m.WorkDir, "custom", name))
	m.Registry.Invalidate()
	return nil
}
