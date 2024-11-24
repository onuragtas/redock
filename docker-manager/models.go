package docker_manager

type DockerCompose struct {
	Version  string      `yaml:"version"`
	Services interface{} `yaml:"services"`
	Networks struct {
		Net struct {
			Ipam struct {
				Driver string `yaml:"driver"`
				Config []struct {
					Subnet string `yaml:"subnet"`
				} `yaml:"config"`
			} `yaml:"ipam"`
		} `yaml:"net"`
	} `yaml:"networks"`
	Volumes struct {
		ElasticsearchData struct {
			Driver string `yaml:"driver"`
		} `yaml:"elasticsearch_data"`
		Global struct {
			Driver string `yaml:"driver"`
		} `yaml:"global"`
	} `yaml:"volumes"`
}

type Services []Service

type Service struct {
	Build         interface{} `json:"build"`
	ContainerName interface{} `json:"container_name"`
	Links         []string    `json:"links"`
	DependsOn     []string    `json:"depends_on"`
	Original      interface{} `json:"original,omitempty"`
	Image         string      `json:"image"`
}
