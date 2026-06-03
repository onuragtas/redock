package backup

import (
	"encoding/json"
	"errors"
	"log"
	"os"
	"path/filepath"
)

const (
	defaultMaxBackups = 10
	configFileName    = "config.json"
)

// Config holds tunable knobs for the backup subsystem. Persisted next to
// the archives at $HOME/redock_backup/config.json so it survives data dir
// restores (which only touch $DOCKER_WORK_DIR/data/).
type Config struct {
	MaxBackups int `json:"max_backups"` // newest N kept after each Create/Import; older ones pruned
}

// DefaultConfig returns the baseline values used when no config file exists.
func DefaultConfig() Config {
	return Config{MaxBackups: defaultMaxBackups}
}

func configPath() (string, error) {
	dir, err := BackupDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, configFileName), nil
}

// LoadConfig reads $HOME/redock_backup/config.json, falling back to defaults
// if the file is missing or has invalid values.
func LoadConfig() (Config, error) {
	p, err := configPath()
	if err != nil {
		return DefaultConfig(), err
	}
	data, err := os.ReadFile(p)
	if errors.Is(err, os.ErrNotExist) {
		return DefaultConfig(), nil
	}
	if err != nil {
		return DefaultConfig(), err
	}
	var c Config
	if err := json.Unmarshal(data, &c); err != nil {
		return DefaultConfig(), err
	}
	if c.MaxBackups <= 0 {
		c.MaxBackups = defaultMaxBackups
	}
	return c, nil
}

// SaveConfig writes the given config to disk, replacing whatever was there.
func SaveConfig(c Config) error {
	if c.MaxBackups <= 0 {
		c.MaxBackups = defaultMaxBackups
	}
	p, err := configPath()
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(p, data, 0o644)
}

// Prune deletes the oldest backups so that at most n remain. List() returns
// newest-first; everything past index n is removed. Returns the IDs that
// were deleted (empty slice if nothing was pruned).
func Prune(n int) ([]string, error) {
	if n <= 0 {
		return nil, nil
	}
	list, err := List()
	if err != nil {
		return nil, err
	}
	if len(list) <= n {
		return nil, nil
	}
	deleted := make([]string, 0, len(list)-n)
	for _, b := range list[n:] {
		if err := Delete(b.ID); err != nil {
			log.Printf("backup: prune delete %s: %v", b.ID, err)
			continue
		}
		deleted = append(deleted, b.ID)
	}
	return deleted, nil
}

// pruneToConfiguredLimit applies the persisted MaxBackups setting. Called
// after every successful Create/Import. Errors are logged but not fatal —
// the new backup is already on disk and the user shouldn't see "create
// succeeded but prune failed" as an API error.
func pruneToConfiguredLimit() {
	cfg, _ := LoadConfig()
	deleted, err := Prune(cfg.MaxBackups)
	if err != nil {
		log.Printf("backup: prune failed: %v", err)
		return
	}
	if len(deleted) > 0 {
		log.Printf("backup: pruned %d old backups (kept newest %d): %v", len(deleted), cfg.MaxBackups, deleted)
	}
}
