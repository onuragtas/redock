package migrations

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"

	"redock/app/models"
	"redock/deployment"
	devenv "redock/devenv"
	docker_manager "redock/docker-manager"
	dns_server "redock/dns_server"
	localproxy "redock/local_proxy"
	"redock/platform/database"
	"redock/platform/memory"

	"gopkg.in/yaml.v2"
)

// MemoryMigrations returns the list of memory DB migrations (run once in version order).
func MemoryMigrations() []database.MemoryMigration {
	return []database.MemoryMigration{
		{
			Version: 1,
			Name:    "cleanup_legacy_users",
			Up: func(db *memory.Database, _ string) error {
				// Bir kerelik: eski createAdmin ile oluşturulmuş user'ları siler.
				users := memory.FindAll[*models.User](db, "users")
				for _, u := range users {
					if err := memory.Delete[*models.User](db, "users", u.GetID()); err != nil {
						return err
					}
				}
				return nil
			},
		},
		{
			Version: 2,
			Name:    "cleanup_legacy_users2",
			Up: func(db *memory.Database, _ string) error {
				// Bir kerelik: eski createAdmin ile oluşturulmuş user'ları siler.
				users := memory.FindAll[*models.User](db, "users")
				for _, u := range users {
					if err := memory.Delete[*models.User](db, "users", u.GetID()); err != nil {
						return err
					}
				}
				return nil
			},
		},
		{
			Version: 3,
			Name:    "import_local_proxy_json",
			Up: func(db *memory.Database, dataDir string) error {
				path := filepath.Join(dataDir, "local_proxy.json")
				data, err := os.ReadFile(path)
				if err != nil {
					if os.IsNotExist(err) {
						return nil
					}
					return err
				}
				var list []localproxy.Item
				if err := json.Unmarshal(data, &list); err != nil {
					return err
				}
				for _, item := range list {
					entity := &localproxy.LocalProxyItem{
						Name:       item.Name,
						LocalPort:  item.LocalPort,
						Host:       item.Host,
						RemotePort: item.RemotePort,
						Timeout:    item.Timeout,
						Started:    item.Started,
					}
					if err := memory.Create(db, "local_proxy_items", entity); err != nil {
						return err
					}
				}
				return nil
			},
		},
		{
			Version: 4,
			Name:    "import_dns_logs_jsonl",
			Up: func(db *memory.Database, dataDir string) error {
				pattern := filepath.Join(dataDir, "dns_logs_*.jsonl")
				files, err := filepath.Glob(pattern)
				if err != nil {
					return err
				}
				for _, path := range files {
					f, err := os.Open(path)
					if err != nil {
						continue
					}
					scanner := bufio.NewScanner(f)
					buf := make([]byte, 0, 64*1024)
					scanner.Buffer(buf, 1024*1024)
					for scanner.Scan() {
						var entity dns_server.DNSQueryLog
						if err := json.Unmarshal(scanner.Bytes(), &entity); err != nil {
							continue
						}
						if entity.CreatedAt.IsZero() {
							continue
						}
						if entity.UpdatedAt.IsZero() {
							entity.UpdatedAt = entity.CreatedAt
						}
						if err := memory.CreatePreserveTimestamps(db, "dns_query_logs", &entity); err != nil {
							f.Close()
							return err
						}
					}
					f.Close()
				}
				return nil
			},
		},
		{
			Version: 5,
			Name:    "import_devenv_json",
			Up: func(db *memory.Database, dataDir string) error {
				// devenv.json workdir kökünde: dataDir = workdir/data -> workdir = filepath.Dir(dataDir)
				workdir := filepath.Dir(dataDir)
				path := filepath.Join(workdir, "devenv.json")
				data, err := os.ReadFile(path)
				if err != nil {
					if os.IsNotExist(err) {
						return nil
					}
					return err
				}
				var list []struct {
					Username   string `json:"username"`
					Password   string `json:"password"`
					Port       int    `json:"port"`
					RedockPort int    `json:"redockPort"`
				}
				if err := json.Unmarshal(data, &list); err != nil {
					return err
				}
				for _, item := range list {
					entity := &devenv.DevEnvEntity{
						Username:   item.Username,
						Password:   item.Password,
						Port:       item.Port,
						RedockPort: item.RedockPort,
					}
					if err := memory.Create(db, "dev_envs", entity); err != nil {
						return err
					}
				}
				// Eski dosyayı yedekle (tekrar migrate edilmesin)
				_ = os.Rename(path, path+".bak")
				return nil
			},
		},
		{
			Version: 6,
			Name:    "import_deployment_json",
			Up: func(db *memory.Database, dataDir string) error {
				path := filepath.Join(dataDir, "deployment.json")
				data, err := os.ReadFile(path)
				if err != nil {
					if os.IsNotExist(err) {
						return nil
					}
					return err
				}
				var cfg struct {
					Username string                          `yaml:"username" json:"username"`
					Token    string                          `yaml:"token" json:"token"`
					Settings struct{ CheckTime int }         `yaml:"settings" json:"settings"`
					Projects []deployment.DeploymentProjectEntity `yaml:"projects" json:"projects"`
				}
				if err := yaml.Unmarshal(data, &cfg); err != nil {
					return err
				}
				checkTime := cfg.Settings.CheckTime
				if checkTime <= 0 {
					checkTime = 60
				}
				settings := &deployment.DeploymentSettingsEntity{
					Username:  cfg.Username,
					Token:     cfg.Token,
					CheckTime: checkTime,
				}
				if err := memory.Create(db, "deployment_settings", settings); err != nil {
					return err
				}
				for i := range cfg.Projects {
					entity := &cfg.Projects[i]
					if err := memory.Create(db, "deployment_projects", entity); err != nil {
						return err
					}
				}
				_ = os.Rename(path, path+".bak")
				return nil
			},
		},
		{
			Version: 7,
			Name:    "import_service_settings_json",
			Up: func(db *memory.Database, dataDir string) error {
				workdir := filepath.Dir(dataDir)
				path := filepath.Join(workdir, "service-settings.json")
				data, err := os.ReadFile(path)
				if err != nil {
					if os.IsNotExist(err) {
						return nil
					}
					return err
				}
				var raw struct {
					ContainerNamePrefix string                              `json:"container_name_prefix"`
					Overrides           map[string]*docker_manager.ServiceOverride `json:"overrides"`
				}
				if err := json.Unmarshal(data, &raw); err != nil {
					return err
				}
				if raw.Overrides == nil {
					raw.Overrides = make(map[string]*docker_manager.ServiceOverride)
				}
				entity := &docker_manager.ServiceSettingsEntity{
					ContainerNamePrefix: raw.ContainerNamePrefix,
					Overrides:           raw.Overrides,
				}
				if err := memory.Create(db, "service_settings", entity); err != nil {
					return err
				}
				_ = os.Rename(path, path+".bak")
				return nil
			},
		},
		{
			Version: 8,
			Name:    "import_starred_vhosts_json",
			Up: func(db *memory.Database, dataDir string) error {
				path := filepath.Join(dataDir, "starred_vhosts.json")
				data, err := os.ReadFile(path)
				if err != nil {
					if os.IsNotExist(err) {
						return nil
					}
					return err
				}
				var paths []string
				if err := json.Unmarshal(data, &paths); err != nil {
					return err
				}
				for _, p := range paths {
					if p == "" {
						continue
					}
					if err := memory.Create(db, "starred_vhosts", &docker_manager.StarredVHostEntity{Path: p}); err != nil {
						return err
					}
				}
				_ = os.Rename(path, path+".bak")
				return nil
			},
		},
	}
}
