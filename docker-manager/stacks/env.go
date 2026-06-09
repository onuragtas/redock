package stacks

import (
	"os"
	"regexp"
	"strings"
)

// EnvModel holds the resolved key/value environment used to interpolate
// ${VAR} references inside service specs (ports, IPs, passwords, ...).
//
// It is the stacks replacement for the legacy .env / .env.example files. For
// now it is still loaded from a .env-style file so the existing UI editor keeps
// working; later it can move into the memory DB.
type EnvModel struct {
	values map[string]string
}

// NewEnvModel builds an empty model.
func NewEnvModel() *EnvModel {
	return &EnvModel{values: map[string]string{}}
}

// Get returns the value for key (empty string if absent).
func (e *EnvModel) Get(key string) string { return e.values[key] }

// Set assigns a value.
func (e *EnvModel) Set(key, value string) { e.values[key] = value }

// All returns a copy of the underlying map.
func (e *EnvModel) All() map[string]string {
	out := make(map[string]string, len(e.values))
	for k, v := range e.values {
		out[k] = v
	}
	return out
}

var envLineRe = regexp.MustCompile(`^\s*([A-Za-z_][A-Za-z0-9_]*)\s*=\s*(.*)$`)

// ParseEnv parses a .env-style document into an EnvModel. Comment lines
// (starting with #) and blanks are ignored. Surrounding quotes are stripped.
func ParseEnv(content string) *EnvModel {
	m := NewEnvModel()
	for _, raw := range strings.Split(content, "\n") {
		line := strings.TrimRight(raw, "\r")
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		match := envLineRe.FindStringSubmatch(line)
		if match == nil {
			continue
		}
		key := match[1]
		val := strings.TrimSpace(match[2])
		val = strings.TrimSuffix(strings.TrimPrefix(val, `"`), `"`)
		val = strings.TrimSuffix(strings.TrimPrefix(val, `'`), `'`)
		m.values[key] = val
	}
	return m
}

// LoadEnvFile reads and parses a .env file from disk.
func LoadEnvFile(path string) (*EnvModel, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return ParseEnv(string(b)), nil
}

// interpRe matches ${VAR}, ${VAR:-default} and ${VAR-default}. Group 1 is the
// variable name; group 3 (if present) is the inline default.
var interpRe = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)(:?-([^}]*))?\}`)

// Expand replaces every ${VAR} / ${VAR:-default} reference in s. A variable
// that is unset (or empty) falls back to its inline default, then to "".
func (e *EnvModel) Expand(s string) string {
	return interpRe.ReplaceAllStringFunc(s, func(tok string) string {
		m := interpRe.FindStringSubmatch(tok)
		if v, ok := e.values[m[1]]; ok && v != "" {
			return v
		}
		return m[3] // inline default (empty if none)
	})
}

// expandSlice expands ${VAR} in each element of a string slice (nil-safe).
func expandSlice(e *EnvModel, in []string) []string {
	if in == nil {
		return nil
	}
	out := make([]string, len(in))
	for i, s := range in {
		out[i] = e.Expand(s)
	}
	return out
}

// Resolve returns a deep copy of spec with all ${VAR} references in the
// env-bearing fields (ports, static IP, env values, volume sources, hostname)
// expanded against the model. The catalog stays declarative; resolution is a
// pure transform applied just before the engine materializes the container.
func (e *EnvModel) Resolve(spec ServiceSpec) ServiceSpec {
	out := spec
	out.Image = e.Expand(spec.Image)
	out.StaticIP = e.Expand(spec.StaticIP)
	out.Hostname = e.Expand(spec.Hostname)
	out.WorkingDir = e.Expand(spec.WorkingDir)
	out.User = e.Expand(spec.User)
	out.Command = expandSlice(e, spec.Command)
	out.Entrypoint = expandSlice(e, spec.Entrypoint)
	out.ExtraHosts = expandSlice(e, spec.ExtraHosts)
	out.DNS = expandSlice(e, spec.DNS)
	out.Aliases = expandSlice(e, spec.Aliases)

	if spec.Labels != nil {
		out.Labels = make(map[string]string, len(spec.Labels))
		for k, v := range spec.Labels {
			out.Labels[k] = e.Expand(v)
		}
	}

	out.Ports = make([]PortMapping, len(spec.Ports))
	for i, p := range spec.Ports {
		out.Ports[i] = PortMapping{
			Host:      e.Expand(p.Host),
			Container: e.Expand(p.Container),
			Protocol:  p.Protocol,
		}
	}

	if spec.Env != nil {
		out.Env = make(map[string]string, len(spec.Env))
		for k, v := range spec.Env {
			out.Env[k] = e.Expand(v)
		}
	}

	out.Volumes = make([]VolumeMount, len(spec.Volumes))
	for i, v := range spec.Volumes {
		out.Volumes[i] = VolumeMount{
			Kind:     v.Kind,
			Source:   e.Expand(v.Source),
			Target:   e.Expand(v.Target),
			ReadOnly: v.ReadOnly,
		}
	}
	return out
}
