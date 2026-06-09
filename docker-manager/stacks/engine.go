package stacks

import (
	"archive/tar"
	"bytes"
	"context"
	"fmt"
	"io"
	"io/fs"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/api/types/strslice"
	"github.com/docker/docker/api/types/volume"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
	"github.com/docker/go-connections/nat"
	units "github.com/docker/go-units"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
)

// Engine drives the Docker Engine API to materialize ServiceSpecs without
// docker-compose. It is safe to construct once and reuse.
type Engine struct {
	cli *client.Client
	// WorkDir is the base directory relative bind-mount sources resolve against
	// (the redock-managed equivalent of the old ~/.docker-environment).
	WorkDir string
	// BuildFS provides build contexts referenced by BuildSpec.Context. It is
	// typically os.DirFS(<repository cache dir>); the contexts are fetched from
	// a repository (compose URL / local folder), not embedded in the binary.
	BuildFS fs.FS
	// networkName is the resolved network to attach containers to. EnsureNetwork
	// reuses an existing network that already owns our subnet (e.g. a running
	// compose stack's lemp_net) instead of creating a conflicting one.
	networkName string
}

// Network returns the resolved network name (after EnsureNetwork), defaulting
// to the redock-managed network.
func (e *Engine) Network() string {
	if e.networkName != "" {
		return e.networkName
	}
	return NetworkName
}

// NewEngine creates an Engine using the ambient Docker environment (DOCKER_HOST
// etc.) with API-version negotiation, matching the rest of the codebase.
func NewEngine(workDir string, buildFS fs.FS) (*Engine, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, fmt.Errorf("docker client: %w", err)
	}
	return &Engine{cli: cli, WorkDir: workDir, BuildFS: buildFS}, nil
}

// Client exposes the underlying SDK client for callers that need raw access.
func (e *Engine) Client() *client.Client { return e.cli }

// Close releases the underlying client.
func (e *Engine) Close() error { return e.cli.Close() }

// EnsureNetwork resolves the network to use. If a network already owns our
// subnet (e.g. a running compose stack's lemp_net), it is reused to avoid an
// "address space overlaps" error and to keep static IPs consistent. Otherwise
// the redock-managed bridge network is created. Idempotent.
func (e *Engine) EnsureNetwork(ctx context.Context) error {
	// 1. Reuse an existing network that already owns our subnet.
	if nets, err := e.cli.NetworkList(ctx, network.ListOptions{}); err == nil {
		for _, n := range nets {
			for _, cfg := range n.IPAM.Config {
				if cfg.Subnet == NetworkSubnet {
					e.networkName = n.Name
					return nil
				}
			}
		}
	}

	// 2. If our network already exists, reuse it.
	if _, err := e.cli.NetworkInspect(ctx, NetworkName, network.InspectOptions{}); err == nil {
		e.networkName = NetworkName
		return nil
	}

	// 3. Create the redock-managed network with the subnet.
	_, err := e.cli.NetworkCreate(ctx, NetworkName, network.CreateOptions{
		Driver: "bridge",
		IPAM: &network.IPAM{
			Driver: "default",
			Config: []network.IPAMConfig{{Subnet: NetworkSubnet}},
		},
	})
	if err != nil && !strings.Contains(err.Error(), "already exists") {
		return fmt.Errorf("create network %s: %w", NetworkName, err)
	}
	e.networkName = NetworkName
	return nil
}

// EnsureVolume creates a named volume if absent. Idempotent.
func (e *Engine) EnsureVolume(ctx context.Context, name string) error {
	_, err := e.cli.VolumeCreate(ctx, volume.CreateOptions{Name: name})
	if err != nil {
		return fmt.Errorf("create volume %s: %w", name, err)
	}
	return nil
}

// PullImage pulls a remote image (honoring an optional platform pin),
// streaming progress to logOut (may be nil to discard).
func (e *Engine) PullImage(ctx context.Context, ref, platform string, logOut io.Writer) error {
	rc, err := e.cli.ImagePull(ctx, ref, image.PullOptions{Platform: platform})
	if err != nil {
		return fmt.Errorf("pull %s: %w", ref, err)
	}
	defer rc.Close()
	return drain(rc, logOut)
}

// BuildImage builds spec.Build from the embedded build context, streaming the
// build log to logOut (may be nil to discard).
func (e *Engine) BuildImage(ctx context.Context, b *BuildSpec, platform string, logOut io.Writer) error {
	if e.BuildFS == nil {
		return fmt.Errorf("build %s: no embedded build FS configured", b.Tag)
	}
	dockerfile := b.Dockerfile
	if dockerfile == "" {
		dockerfile = "Dockerfile"
	}
	tarReader, err := tarFromFS(e.BuildFS, b.Context)
	if err != nil {
		return fmt.Errorf("tar context %s: %w", b.Context, err)
	}
	resp, err := e.cli.ImageBuild(ctx, tarReader, types.ImageBuildOptions{
		Tags:        []string{b.Tag},
		Dockerfile:  dockerfile,
		Remove:      true,
		ForceRemove: true,
		Platform:    platform,
	})
	if err != nil {
		return fmt.Errorf("build %s: %w", b.Tag, err)
	}
	defer resp.Body.Close()
	return drain(resp.Body, logOut)
}

// EnsureImage builds or pulls the image backing spec, as appropriate.
func (e *Engine) EnsureImage(ctx context.Context, spec ServiceSpec, logOut io.Writer) error {
	if spec.Build != nil {
		return e.BuildImage(ctx, spec.Build, spec.Platform, logOut)
	}
	if spec.Image != "" {
		return e.PullImage(ctx, spec.Image, spec.Platform, logOut)
	}
	return fmt.Errorf("service %s has neither Image nor Build", spec.Name)
}

// imageRef returns the image reference used to run spec.
func imageRef(spec ServiceSpec) string {
	if spec.Build != nil {
		return spec.Build.Tag
	}
	return spec.Image
}

// CreateAndStart creates (recreating if a same-named container exists) and
// starts the container for an already env-resolved spec. Returns the new ID.
func (e *Engine) CreateAndStart(ctx context.Context, spec ServiceSpec) (string, error) {
	name := spec.ContainerName
	if name == "" {
		name = spec.Name
	}

	// Remove any pre-existing container with the same name (recreate semantics).
	if existing, err := e.findByName(ctx, name); err == nil && existing != "" {
		_ = e.cli.ContainerRemove(ctx, existing, container.RemoveOptions{Force: true})
	}

	exposed, bindings := portConfig(spec.Ports)

	config := &container.Config{
		Image:        imageRef(spec),
		Hostname:     spec.Hostname,
		Tty:          spec.TTY,
		OpenStdin:    spec.StdinOpen,
		Env:          envSlice(spec.Env),
		ExposedPorts: exposed,
		WorkingDir:   spec.WorkingDir,
		User:         spec.User,
		Labels:       spec.Labels,
		Healthcheck:  healthConfig(spec.Healthcheck),
	}
	if len(spec.Command) > 0 {
		config.Cmd = strslice.StrSlice(spec.Command)
	}
	if len(spec.Entrypoint) > 0 {
		config.Entrypoint = strslice.StrSlice(spec.Entrypoint)
	}

	hostConfig := &container.HostConfig{
		Mounts:         e.mounts(spec.Volumes),
		PortBindings:   bindings,
		Privileged:     spec.Privileged,
		ReadonlyRootfs: spec.ReadOnly,
		ExtraHosts:     spec.ExtraHosts,
		DNS:            spec.DNS,
		Tmpfs:          tmpfsMap(spec.Tmpfs),
		ShmSize:        spec.ShmSize,
	}
	hostConfig.Resources.Ulimits = ulimits(spec.Ulimits)
	hostConfig.Resources.Memory = spec.Memory
	hostConfig.Resources.MemorySwap = spec.MemorySwap
	hostConfig.Resources.NanoCPUs = spec.NanoCPUs
	if spec.PidsLimit > 0 {
		pl := spec.PidsLimit
		hostConfig.Resources.PidsLimit = &pl
	}
	if spec.Restart != "" {
		hostConfig.RestartPolicy = container.RestartPolicy{Name: container.RestartPolicyMode(spec.Restart)}
	}

	netConfig := &network.NetworkingConfig{
		EndpointsConfig: map[string]*network.EndpointSettings{
			e.Network(): endpoint(spec),
		},
	}

	resp, err := e.cli.ContainerCreate(ctx, config, hostConfig, netConfig, platformSpec(spec.Platform), name)
	if err != nil {
		return "", fmt.Errorf("create %s: %w", name, err)
	}
	if err := e.cli.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		return "", fmt.Errorf("start %s: %w", name, err)
	}
	return resp.ID, nil
}

// Stop stops a container by name (best-effort, with a grace timeout).
func (e *Engine) Stop(ctx context.Context, name string) error {
	id, err := e.findByName(ctx, name)
	if err != nil || id == "" {
		return err
	}
	timeout := 10
	return e.cli.ContainerStop(ctx, id, container.StopOptions{Timeout: &timeout})
}

// Remove force-removes a container by name (no-op if absent).
func (e *Engine) Remove(ctx context.Context, name string) error {
	id, err := e.findByName(ctx, name)
	if err != nil || id == "" {
		return err
	}
	return e.cli.ContainerRemove(ctx, id, container.RemoveOptions{Force: true})
}

// Restart restarts a container by name.
func (e *Engine) Restart(ctx context.Context, name string) error {
	id, err := e.findByName(ctx, name)
	if err != nil || id == "" {
		return fmt.Errorf("restart %s: not found", name)
	}
	timeout := 10
	return e.cli.ContainerRestart(ctx, id, container.StopOptions{Timeout: &timeout})
}

// Status reports whether a container exists and is running.
func (e *Engine) Status(ctx context.Context, name string) (exists, running bool, state string, err error) {
	id, ferr := e.findByName(ctx, name)
	if ferr != nil {
		return false, false, "", ferr
	}
	if id == "" {
		return false, false, "", nil
	}
	insp, err := e.cli.ContainerInspect(ctx, id)
	if err != nil {
		return true, false, "", err
	}
	return true, insp.State.Running, insp.State.Status, nil
}

// Logs streams a container's logs to out. When follow is false it returns the
// current buffered logs and stops.
func (e *Engine) Logs(ctx context.Context, name string, follow bool, tail string, out io.Writer) error {
	id, err := e.findByName(ctx, name)
	if err != nil || id == "" {
		return fmt.Errorf("logs %s: not found", name)
	}
	rc, err := e.cli.ContainerLogs(ctx, id, container.LogsOptions{
		ShowStdout: true, ShowStderr: true, Follow: follow, Tail: tail, Timestamps: false,
	})
	if err != nil {
		return err
	}
	defer rc.Close()
	_, err = stdcopy.StdCopy(out, out, rc)
	return err
}

// Exec runs cmd inside a container and returns combined stdout+stderr.
func (e *Engine) Exec(ctx context.Context, name string, cmd []string) (string, error) {
	id, err := e.findByName(ctx, name)
	if err != nil || id == "" {
		return "", fmt.Errorf("exec %s: not found", name)
	}
	idResp, err := e.cli.ContainerExecCreate(ctx, id, container.ExecOptions{
		Cmd: cmd, AttachStdout: true, AttachStderr: true,
	})
	if err != nil {
		return "", err
	}
	att, err := e.cli.ContainerExecAttach(ctx, idResp.ID, container.ExecAttachOptions{})
	if err != nil {
		return "", err
	}
	defer att.Close()
	var buf bytes.Buffer
	if _, err := stdcopy.StdCopy(&buf, &buf, att.Reader); err != nil {
		return buf.String(), err
	}
	return buf.String(), nil
}

// CopyToContainer copies a single file's bytes to dstPath inside the container
// (replacement for `docker cp`, used by the xdebug.ini injection path).
func (e *Engine) CopyToContainer(ctx context.Context, name, dstPath string, content []byte) error {
	id, err := e.findByName(ctx, name)
	if err != nil || id == "" {
		return fmt.Errorf("cp %s: not found", name)
	}
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	hdr := &tar.Header{Name: path.Base(dstPath), Mode: 0o644, Size: int64(len(content)), ModTime: time.Unix(0, 0)}
	if err := tw.WriteHeader(hdr); err != nil {
		return err
	}
	if _, err := tw.Write(content); err != nil {
		return err
	}
	if err := tw.Close(); err != nil {
		return err
	}
	return e.cli.CopyToContainer(ctx, id, path.Dir(dstPath), &buf, container.CopyToContainerOptions{})
}

// --- helpers ---

// findByName returns the container ID for an exact container name, or "".
func (e *Engine) findByName(ctx context.Context, name string) (string, error) {
	list, err := e.cli.ContainerList(ctx, container.ListOptions{
		All:     true,
		Filters: filters.NewArgs(filters.Arg("name", name)),
	})
	if err != nil {
		return "", err
	}
	want := "/" + name
	for _, c := range list {
		for _, n := range c.Names {
			if n == want || n == name {
				return c.ID, nil
			}
		}
	}
	return "", nil
}

func endpoint(spec ServiceSpec) *network.EndpointSettings {
	ep := &network.EndpointSettings{}
	if spec.StaticIP != "" {
		ep.IPAMConfig = &network.EndpointIPAMConfig{IPv4Address: spec.StaticIP}
	}
	aliases := spec.Aliases
	if spec.Name != "" {
		aliases = append([]string{spec.Name}, aliases...)
	}
	ep.Aliases = dedupe(aliases)
	return ep
}

func (e *Engine) mounts(vms []VolumeMount) []mount.Mount {
	out := make([]mount.Mount, 0, len(vms))
	for _, v := range vms {
		m := mount.Mount{Target: v.Target, ReadOnly: v.ReadOnly}
		switch v.Kind {
		case VolumeNamed:
			m.Type = mount.TypeVolume
			m.Source = v.Source
		default:
			m.Type = mount.TypeBind
			m.Source = e.resolveBind(v.Source)
		}
		out = append(out, m)
	}
	return out
}

// resolveBind makes a bind source absolute relative to WorkDir when needed.
func (e *Engine) resolveBind(src string) string {
	if filepath.IsAbs(src) {
		return src
	}
	clean := strings.TrimPrefix(src, "./")
	return filepath.Join(e.WorkDir, clean)
}

func portConfig(ports []PortMapping) (nat.PortSet, nat.PortMap) {
	exposed := nat.PortSet{}
	bindings := nat.PortMap{}
	for _, p := range ports {
		proto := p.Protocol
		if proto == "" {
			proto = "tcp"
		}
		if p.Container == "" {
			continue
		}
		natPort := nat.Port(fmt.Sprintf("%s/%s", p.Container, proto))
		exposed[natPort] = struct{}{}
		if p.Host != "" {
			bindings[natPort] = append(bindings[natPort], nat.PortBinding{HostIP: "0.0.0.0", HostPort: p.Host})
		}
	}
	return exposed, bindings
}

func envSlice(env map[string]string) []string {
	if len(env) == 0 {
		return nil
	}
	out := make([]string, 0, len(env))
	for k, v := range env {
		out = append(out, k+"="+v)
	}
	return out
}

func healthConfig(h *HealthcheckSpec) *container.HealthConfig {
	if h == nil || len(h.Test) == 0 {
		return nil
	}
	return &container.HealthConfig{
		Test:        h.Test,
		Retries:     h.Retries,
		Interval:    parseDur(h.Interval),
		Timeout:     parseDur(h.Timeout),
		StartPeriod: parseDur(h.StartPeriod),
	}
}

func parseDur(s string) time.Duration {
	if s == "" {
		return 0
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return 0
	}
	return d
}

func tmpfsMap(paths []string) map[string]string {
	if len(paths) == 0 {
		return nil
	}
	m := make(map[string]string, len(paths))
	for _, p := range paths {
		if p != "" {
			m[p] = ""
		}
	}
	return m
}

func ulimits(us []Ulimit) []*units.Ulimit {
	if len(us) == 0 {
		return nil
	}
	out := make([]*units.Ulimit, 0, len(us))
	for _, u := range us {
		out = append(out, &units.Ulimit{Name: u.Name, Soft: u.Soft, Hard: u.Hard})
	}
	return out
}

func platformSpec(p string) *ocispec.Platform {
	if p == "" {
		return nil
	}
	parts := strings.SplitN(p, "/", 2)
	plat := &ocispec.Platform{OS: parts[0]}
	if len(parts) == 2 {
		plat.Architecture = parts[1]
	}
	return plat
}

func dedupe(in []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(in))
	for _, s := range in {
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}

// tarFromFS builds an in-memory tar archive of the directory tree rooted at
// root inside fsys, suitable as a Docker build context.
func tarFromFS(fsys fs.FS, root string) (io.Reader, error) {
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	err := fs.WalkDir(fsys, root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		data, err := fs.ReadFile(fsys, p)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		hdr := &tar.Header{
			Name:    filepath.ToSlash(rel),
			Mode:    0o644,
			Size:    int64(len(data)),
			ModTime: time.Unix(0, 0),
		}
		if err := tw.WriteHeader(hdr); err != nil {
			return err
		}
		_, err = tw.Write(data)
		return err
	})
	if err != nil {
		return nil, err
	}
	if err := tw.Close(); err != nil {
		return nil, err
	}
	return &buf, nil
}

// drain copies r to out (or discards it) until EOF. Used to block on
// build/pull completion.
func drain(r io.Reader, out io.Writer) error {
	if out == nil {
		_, err := io.Copy(io.Discard, r)
		return err
	}
	_, err := io.Copy(out, r)
	return err
}
