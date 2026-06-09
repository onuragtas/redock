// Package stacks implements docker-compose-free orchestration of the redock
// development environment directly against the Docker Engine API (Go SDK).
//
// It replaces the previous approach (cloning onuragtas/docker, generating a
// docker-compose.yml and shelling out to docker-compose/bash) with:
//   - a declarative service catalog expressed as Go structs (ServiceSpec),
//   - an env model that interpolates ${VAR} references,
//   - an engine that drives the Docker SDK (network/volume/build/pull/run/...).
//
// Phase 0 (this file + env.go + engine.go) provides the foundation only; it is
// not wired into the live code path yet. The existing DockerEnvironmentManager
// keeps working unchanged until cutover.
package stacks

// BuildSpec describes how to build an image from an embedded build context.
type BuildSpec struct {
	// Context is the path of the build context (e.g. "php74_xdebug") within the
	// repository. It must contain a Dockerfile.
	Context string `json:"context"`
	// Dockerfile is the Dockerfile name relative to Context (default "Dockerfile").
	Dockerfile string `json:"dockerfile,omitempty"`
	// Tag is the image tag to assign to the built image (e.g. "redock/php74_xdebug").
	Tag string `json:"tag"`
}

// PortMapping is a host:container port publish rule. Values may be literal
// ("8080") or env references ("${NGINX_PORT}") resolved at materialization.
type PortMapping struct {
	Host      string `json:"host"`               // host port (may be an env ref)
	Container string `json:"container"`          // container port
	Protocol  string `json:"protocol,omitempty"` // "tcp" (default) or "udp"
}

// VolumeKind distinguishes bind mounts from named volumes.
type VolumeKind int

const (
	// VolumeBind is a host-path bind mount.
	VolumeBind VolumeKind = iota
	// VolumeNamed is a Docker named volume.
	VolumeNamed
)

// VolumeMount is a single mount on a service container.
type VolumeMount struct {
	Kind     VolumeKind `json:"kind"`
	Source   string     `json:"source"` // host path (bind) or volume name (named)
	Target   string     `json:"target"` // path inside the container
	ReadOnly bool       `json:"read_only,omitempty"`
}

// Ulimit mirrors a compose ulimit entry.
type Ulimit struct {
	Name string `json:"name"` // e.g. "nofile", "memlock"
	Soft int64  `json:"soft"`
	Hard int64  `json:"hard"`
}

// HealthcheckSpec mirrors a container healthcheck. Durations are strings like
// "30s" / "1m" (parsed when materialized).
type HealthcheckSpec struct {
	Test        []string `json:"test,omitempty"` // e.g. ["CMD","curl","-f","http://localhost"]
	Interval    string   `json:"interval,omitempty"`
	Timeout     string   `json:"timeout,omitempty"`
	Retries     int      `json:"retries,omitempty"`
	StartPeriod string   `json:"start_period,omitempty"`
}

// ServiceSpec is the stacks, compose-equivalent definition of one service.
// Exactly one of Image or Build must be set.
type ServiceSpec struct {
	Name          string `json:"name"`           // logical service name (compose key)
	ContainerName string `json:"container_name"` // resolved container name

	Image string     `json:"image,omitempty"` // pulled image (mutually exclusive with Build)
	Build *BuildSpec `json:"build,omitempty"` // built-from-context image (mutually exclusive with Image)

	Ports   []PortMapping     `json:"ports,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
	Volumes []VolumeMount     `json:"volumes,omitempty"`

	StaticIP string   `json:"static_ip,omitempty"` // ipv4 on the redock network
	Aliases  []string `json:"aliases,omitempty"`   // network aliases (replaces legacy links)

	DependsOn  []string `json:"depends_on,omitempty"` // start-order dependencies
	Command    []string `json:"command,omitempty"`    // overrides image CMD
	Entrypoint []string `json:"entrypoint,omitempty"` // overrides image ENTRYPOINT

	Restart  string   `json:"restart,omitempty"` // "", "no", "always", "on-failure", "unless-stopped"
	TTY      bool     `json:"tty,omitempty"`
	StdinOpen bool    `json:"stdin_open,omitempty"` // -i
	Hostname string   `json:"hostname,omitempty"`
	Ulimits  []Ulimit `json:"ulimits,omitempty"`
	Platform string   `json:"platform,omitempty"` // e.g. "linux/amd64" (pin)

	// Environment / runtime
	WorkingDir string            `json:"working_dir,omitempty"`
	User       string            `json:"user,omitempty"`
	Labels     map[string]string `json:"labels,omitempty"`
	EnvFile    []string          `json:"env_file,omitempty"` // repo-relative env files

	// Networking
	ExtraHosts []string `json:"extra_hosts,omitempty"` // "host:ip"
	DNS        []string `json:"dns,omitempty"`

	// Storage
	Tmpfs    []string `json:"tmpfs,omitempty"` // container paths
	ReadOnly bool     `json:"read_only,omitempty"`
	ShmSize  int64    `json:"shm_size,omitempty"` // bytes

	// Resources / privileges
	Privileged bool  `json:"privileged,omitempty"`
	Memory     int64 `json:"memory,omitempty"`      // bytes
	MemorySwap int64 `json:"memory_swap,omitempty"` // bytes
	NanoCPUs   int64 `json:"nano_cpus,omitempty"`   // CPU * 1e9
	PidsLimit  int64 `json:"pids_limit,omitempty"`

	// Lifecycle
	Healthcheck *HealthcheckSpec `json:"healthcheck,omitempty"`

	Category string `json:"category,omitempty"` // grouping for the UI

	// SourceDir is a runtime-resolved field (not from compose): the on-disk
	// repository cache directory this service came from. Build contexts and
	// repo-relative bind sources resolve against it. Empty for custom
	// Hub-image services.
	SourceDir string `json:"source_dir,omitempty"`

	// ImportError, when non-empty, means the service was parsed but cannot be
	// started (e.g. its build context could not be fetched). It is kept in the
	// catalog and flagged in the UI rather than silently dropped.
	ImportError string `json:"import_error,omitempty"`

	// Repo is the name of the repository this service came from ("custom" for a
	// directly-added Hub-image service). Runtime-resolved during merge.
	Repo string `json:"repo,omitempty"`
}

// NetworkName is the single bridge network all services attach to.
const NetworkName = "redock_net"

// NetworkSubnet matches the legacy compose subnet so existing static IPs and
// any hard-coded references keep resolving.
const NetworkSubnet = "172.28.0.0/16"
