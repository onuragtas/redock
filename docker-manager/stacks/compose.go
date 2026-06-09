package stacks

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v2"
)

// ParsedStack is the result of parsing a docker-compose document into the
// stacks model, plus the repository-relative files that must be fetched
// alongside it (build contexts and bind-mounted config files).
type ParsedStack struct {
	Services      []ServiceSpec
	BuildContexts []string          // repo-relative dirs containing a Dockerfile (e.g. "nginx")
	BindFiles     []string          // repo-relative config files bind-mounted into containers
	Unimportable  map[string]string // service name → reason it could not be imported
}

// ParseCompose converts a single docker-compose document into a ParsedStack.
func ParseCompose(data []byte) (*ParsedStack, error) {
	return ParseComposeFiles([][]byte{data})
}

// ParseComposeFiles parses multiple docker-compose documents and MERGES them in
// order (docker-compose `-f a.yml -f b.yml` override semantics): later files
// extend/override earlier ones. Then it converts the merged result to the
// stacks model. This supports the base + override.yml pattern.
func ParseComposeFiles(docs [][]byte) (*ParsedStack, error) {
	merged := map[interface{}]interface{}{}
	for _, data := range docs {
		if len(data) == 0 {
			continue
		}
		var m map[interface{}]interface{}
		if err := yaml.Unmarshal(data, &m); err != nil {
			return nil, fmt.Errorf("parse compose: %w", err)
		}
		mergeYAMLMap(merged, m)
	}

	servicesRaw, _ := merged["services"].(map[interface{}]interface{})
	resolveExtends(servicesRaw)

	names := make([]string, 0, len(servicesRaw))
	for k := range servicesRaw {
		names = append(names, composeStr(k))
	}
	sort.Strings(names)

	ps := &ParsedStack{}
	bindSet := map[string]bool{}
	ctxSet := map[string]bool{}

	for _, name := range names {
		svc, ok := servicesRaw[name].(map[interface{}]interface{})
		if !ok {
			continue
		}
		_, hasImage := svc["image"]
		_, hasBuild := svc["build"]
		if !hasImage && !hasBuild {
			continue // volume placeholder / invalid
		}

		spec := ServiceSpec{Name: name, Category: categorize(name)}

		if cn := composeStr(svc["container_name"]); cn != "" {
			spec.ContainerName = cn
		} else {
			spec.ContainerName = name
		}

		if ctx := buildContext(svc["build"]); ctx != "" {
			spec.Build = &BuildSpec{Context: ctx, Tag: "redock/" + name}
			ctxSet[ctx] = true
		} else if img := composeStr(svc["image"]); img != "" {
			spec.Image = img
		}

		spec.Platform = composeStr(svc["platform"])
		spec.Restart = composeStr(svc["restart"])
		if tty, ok := svc["tty"].(bool); ok {
			spec.TTY = tty
		}
		spec.Hostname = composeStr(svc["hostname"])
		spec.Ports = composePorts(svc["ports"], svc["expose"])
		spec.Env = composeEnv(svc["environment"])
		spec.Volumes = composeVolumes(svc["volumes"], bindSet)
		spec.StaticIP = composeStaticIP(svc["networks"])
		spec.DependsOn = composeDeps(svc["links"], svc["depends_on"])
		spec.Command = composeStrSlice(svc["command"])
		spec.Entrypoint = composeStrSlice(svc["entrypoint"])
		spec.Ulimits = composeUlimits(svc["ulimits"])
		spec.Healthcheck = composeHealthcheck(svc["healthcheck"])
		spec.WorkingDir = composeStr(svc["working_dir"])
		spec.User = composeStr(svc["user"])
		spec.EnvFile = composeEnvFiles(svc["env_file"])
		for _, f := range spec.EnvFile {
			if strings.HasPrefix(f, "./") {
				bindSet[strings.TrimPrefix(f, "./")] = true
			} else if !strings.Contains(f, "/") || strings.HasPrefix(f, ".") {
				bindSet[strings.TrimPrefix(f, "./")] = true
			}
		}

		ps.Services = append(ps.Services, spec)
	}

	for c := range ctxSet {
		ps.BuildContexts = append(ps.BuildContexts, c)
	}
	for f := range bindSet {
		ps.BindFiles = append(ps.BindFiles, f)
	}
	sort.Strings(ps.BuildContexts)
	sort.Strings(ps.BindFiles)
	return ps, nil
}

// resolveExtends resolves within-file `extends` (service-level inheritance):
// the base service's definition is used as defaults, overridden by the service.
// File-based extends (extends.file) is out of scope and ignored.
func resolveExtends(services map[interface{}]interface{}) {
	for key, raw := range services {
		svc, ok := raw.(map[interface{}]interface{})
		if !ok {
			continue
		}
		ext, has := svc["extends"]
		if !has {
			continue
		}
		baseName := ""
		switch e := ext.(type) {
		case string:
			baseName = e
		case map[interface{}]interface{}:
			if composeStr(e["file"]) != "" {
				continue // external file extends unsupported
			}
			baseName = composeStr(e["service"])
		}
		base, ok := services[baseName].(map[interface{}]interface{})
		if !ok || baseName == composeStr(key) {
			delete(svc, "extends")
			continue
		}
		merged, _ := deepCopyYAML(base).(map[interface{}]interface{})
		mergeYAMLMap(merged, svc)
		delete(merged, "extends")
		services[key] = merged
	}
}

// deepCopyYAML returns a deep copy of a yaml.v2 value tree.
func deepCopyYAML(v interface{}) interface{} {
	switch t := v.(type) {
	case map[interface{}]interface{}:
		out := make(map[interface{}]interface{}, len(t))
		for k, val := range t {
			out[k] = deepCopyYAML(val)
		}
		return out
	case []interface{}:
		out := make([]interface{}, len(t))
		for i, val := range t {
			out[i] = deepCopyYAML(val)
		}
		return out
	default:
		return v
	}
}

// mergeYAMLMap deep-merges src into dst with docker-compose override rules.
func mergeYAMLMap(dst, src map[interface{}]interface{}) {
	for k, sv := range src {
		if dv, ok := dst[k]; ok {
			dst[k] = mergeYAMLValue(k, dv, sv)
		} else {
			dst[k] = sv
		}
	}
}

// mergeYAMLValue merges two values: maps merge recursively; list values are
// concatenated (ports/volumes/environment/depends_on...) except command and
// entrypoint which the override replaces; scalars are overridden.
func mergeYAMLValue(key, dv, sv interface{}) interface{} {
	if dm, ok := dv.(map[interface{}]interface{}); ok {
		if sm, ok := sv.(map[interface{}]interface{}); ok {
			mergeYAMLMap(dm, sm)
			return dm
		}
	}
	if ds, ok := dv.([]interface{}); ok {
		if ss, ok := sv.([]interface{}); ok {
			if ks, _ := key.(string); ks == "command" || ks == "entrypoint" {
				return sv // single-value list → override replaces
			}
			return append(ds, ss...) // multi-value list → concatenate
		}
	}
	return sv // scalar → override wins
}

// --- compose value extraction (runtime port of the one-off generator) ---

func composeStr(v interface{}) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprintf("%v", v)
}

func composeStrSlice(v interface{}) []string {
	switch t := v.(type) {
	case string:
		return strings.Fields(t)
	case []interface{}:
		out := make([]string, 0, len(t))
		for _, e := range t {
			out = append(out, composeStr(e))
		}
		return out
	}
	return nil
}

// buildContext returns the repo-relative context dir for a compose `build:`
// (string or {context: ...} map), stripped of a leading "./".
func buildContext(v interface{}) string {
	switch t := v.(type) {
	case string:
		return strings.TrimPrefix(strings.TrimSpace(t), "./")
	case map[interface{}]interface{}:
		return strings.TrimPrefix(strings.TrimSpace(composeStr(t["context"])), "./")
	}
	return ""
}

func composePorts(ports, expose interface{}) []PortMapping {
	var out []PortMapping
	if list, ok := ports.([]interface{}); ok {
		for _, e := range list {
			// Long syntax: {target, published, protocol}.
			if mp, ok := e.(map[interface{}]interface{}); ok {
				pm := PortMapping{
					Host:      composeStr(mp["published"]),
					Container: composeStr(mp["target"]),
					Protocol:  composeStr(mp["protocol"]),
				}
				if pm.Container != "" {
					out = append(out, pm)
				}
				continue
			}
			// Short syntax: "host:container[/proto]" or "container".
			s := composeStr(e)
			proto := ""
			if i := strings.Index(s, "/"); i >= 0 {
				proto = s[i+1:]
				s = s[:i]
			}
			parts := strings.SplitN(s, ":", 2)
			if len(parts) == 2 {
				out = append(out, PortMapping{Host: parts[0], Container: parts[1], Protocol: proto})
			} else {
				out = append(out, PortMapping{Container: parts[0], Protocol: proto})
			}
		}
	}
	if list, ok := expose.([]interface{}); ok {
		for _, e := range list {
			out = append(out, PortMapping{Container: composeStr(e)})
		}
	}
	return out
}

// composeHealthcheck parses a service healthcheck block.
func composeHealthcheck(v interface{}) *HealthcheckSpec {
	m, ok := v.(map[interface{}]interface{})
	if !ok {
		return nil
	}
	if d, _ := m["disable"].(bool); d {
		return nil
	}
	hc := &HealthcheckSpec{
		Interval:    composeStr(m["interval"]),
		Timeout:     composeStr(m["timeout"]),
		StartPeriod: composeStr(m["start_period"]),
	}
	hc.Retries = int(composeInt(m["retries"]))
	switch t := m["test"].(type) {
	case string:
		hc.Test = []string{"CMD-SHELL", t}
	case []interface{}:
		for _, e := range t {
			hc.Test = append(hc.Test, composeStr(e))
		}
	}
	if len(hc.Test) == 0 {
		return nil
	}
	return hc
}

// composeEnvFiles parses env_file (string or list) into repo-relative paths.
func composeEnvFiles(v interface{}) []string {
	switch t := v.(type) {
	case string:
		return []string{t}
	case []interface{}:
		var out []string
		for _, e := range t {
			// long form {path: ...} or plain string
			if mp, ok := e.(map[interface{}]interface{}); ok {
				if p := composeStr(mp["path"]); p != "" {
					out = append(out, p)
				}
				continue
			}
			out = append(out, composeStr(e))
		}
		return out
	}
	return nil
}

func composeEnv(v interface{}) map[string]string {
	out := map[string]string{}
	switch t := v.(type) {
	case map[interface{}]interface{}:
		for k, val := range t {
			out[composeStr(k)] = composeStr(val)
		}
	case []interface{}:
		for _, e := range t {
			kv := strings.SplitN(composeStr(e), "=", 2)
			if len(kv) == 2 {
				out[kv[0]] = kv[1]
			}
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

var fileWithExtRe = regexp.MustCompile(`\.[A-Za-z0-9]+$`)

func composeVolumes(v interface{}, bindSet map[string]bool) []VolumeMount {
	list, ok := v.([]interface{})
	if !ok {
		return nil
	}
	var out []VolumeMount
	for _, e := range list {
		// Long syntax: {type, source, target, read_only}.
		if mp, ok := e.(map[interface{}]interface{}); ok {
			vm := VolumeMount{Source: composeStr(mp["source"]), Target: composeStr(mp["target"])}
			if ro, _ := mp["read_only"].(bool); ro {
				vm.ReadOnly = true
			}
			if composeStr(mp["type"]) == "volume" {
				vm.Kind = VolumeNamed
			} else {
				vm.Kind = VolumeBind
			}
			if vm.Target != "" {
				out = append(out, vm)
			}
			continue
		}
		s := strings.Trim(composeStr(e), "'\"")
		parts := strings.Split(s, ":")
		if len(parts) < 2 {
			continue
		}
		vm := VolumeMount{Source: parts[0], Target: parts[1]}
		if len(parts) >= 3 && parts[2] == "ro" {
			vm.ReadOnly = true
		}
		if !strings.ContainsAny(vm.Source, "/.$") {
			vm.Kind = VolumeNamed
		} else {
			vm.Kind = VolumeBind
			// Repo-relative bind sources that point to a FILE (have an
			// extension) must be fetched from the repository alongside the
			// compose (e.g. ./etc/redis.conf, ./php74/php.ini). Directories
			// (./logs, ./data) and env-paths (${...}) are runtime-provided.
			src := strings.TrimRight(vm.Source, "/")
			if strings.HasPrefix(src, "./") && !strings.Contains(src, "$") {
				base := src[strings.LastIndex(src, "/")+1:]
				if fileWithExtRe.MatchString(base) {
					bindSet[strings.TrimPrefix(src, "./")] = true
				}
			}
		}
		out = append(out, vm)
	}
	return out
}

func composeStaticIP(v interface{}) string {
	nets, ok := v.(map[interface{}]interface{})
	if !ok {
		return ""
	}
	net, ok := nets["net"].(map[interface{}]interface{})
	if !ok {
		return ""
	}
	return composeStr(net["ipv4_address"])
}

func composeDeps(links, dependsOn interface{}) []string {
	seen := map[string]bool{}
	var out []string
	add := func(s string) {
		s = strings.SplitN(s, ":", 2)[0]
		if s != "" && !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	for _, s := range composeStrSlice(links) {
		add(s)
	}
	switch t := dependsOn.(type) {
	case []interface{}:
		for _, e := range t {
			add(composeStr(e))
		}
	case map[interface{}]interface{}:
		for k := range t {
			add(composeStr(k))
		}
	}
	return out
}

func composeUlimits(v interface{}) []Ulimit {
	m, ok := v.(map[interface{}]interface{})
	if !ok {
		return nil
	}
	var out []Ulimit
	for k, val := range m {
		u := Ulimit{Name: composeStr(k)}
		switch t := val.(type) {
		case int:
			u.Soft, u.Hard = int64(t), int64(t)
		case map[interface{}]interface{}:
			u.Soft = composeInt(t["soft"])
			u.Hard = composeInt(t["hard"])
		}
		out = append(out, u)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func composeInt(v interface{}) int64 {
	if i, ok := v.(int); ok {
		return int64(i)
	}
	return 0
}

func categorize(name string) string {
	switch {
	case strings.HasPrefix(name, "php"):
		return "php"
	case name == "nginx" || name == "httpd":
		return "web"
	case strings.Contains(name, "mongo") || strings.Contains(name, "postgres") ||
		name == "db" || name == "redis" || name == "arangodb" || name == "pgbouncer" || name == "proxysql":
		return "database"
	case strings.Contains(name, "elasticsearch") || name == "kibana" || name == "elastichq" ||
		strings.Contains(name, "graylog") || name == "hunspell":
		return "search"
	case name == "rabbitmq" || name == "kafka" || name == "zookeeper" || name == "kafdrop" || name == "kafka-ui":
		return "messaging"
	case name == "prometheus" || name == "grafana" || name == "sonarqube":
		return "monitoring"
	case name == "sentry" || name == "cron" || name == "worker":
		return "tracking"
	case name == "keycloak":
		return "auth"
	case name == "global" || name == "supervisor" || name == "ssh_ui":
		return "infra"
	default:
		return "tooling"
	}
}
