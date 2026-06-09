package stacks

import "redock/platform/memory"

// Memory-DB table names for the stacks orchestration state.
const (
	TableRepositories   = "stacks_repositories"
	TableCustomServices = "stacks_custom_services"
	TableActiveServices = "stacks_active_services"
	TableDevEnvSettings = "stacks_devenv_settings"
	TableMeta           = "stacks_meta"
)

// MetaEntity is a generic key/value flag used for one-time migrations (e.g.
// recording that the active-service seed from running containers has run).
type MetaEntity struct {
	memory.BaseEntity
	Key   string `json:"key"`
	Value string `json:"value"`
}

// DevEnvSettingsEntity persists the configurable parameters for personal
// development containers (the stacks replacement for serviceip.sh).
type DevEnvSettingsEntity struct {
	memory.BaseEntity
	Settings DevEnvSettings `json:"settings"`
}

// RepositoryEntity persists one configured repository (default, user git/url,
// or local folder). Survives restarts.
type RepositoryEntity struct {
	memory.BaseEntity
	Name     string   `json:"name"`
	Kind     RepoKind `json:"kind"`
	Location string   `json:"location"`
	Compose  string   `json:"compose"`
	Enabled  bool     `json:"enabled"`
}

func (e *RepositoryEntity) toRepository() Repository {
	return Repository{
		Name:     e.Name,
		Kind:     e.Kind,
		Location: e.Location,
		Compose:  e.Compose,
		Enabled:  e.Enabled,
	}
}

// CustomServiceEntity persists a single Hub-image service added directly by the
// user (no repository, no build).
type CustomServiceEntity struct {
	memory.BaseEntity
	Spec ServiceSpec `json:"spec"`
}

// ActiveServiceEntity records that a service has been activated (started),
// replacing the old "active services" derivation from a generated compose file.
type ActiveServiceEntity struct {
	memory.BaseEntity
	Name string `json:"name"`
}
