package docker_manager

import (
	"fmt"
	"sort"
	"strings"

	"redock/platform/database"
	"redock/platform/memory"
)

type ServiceOverride struct {
	CustomName string   `json:"custom_name"`
	Ports      []string `json:"ports"`
}

type ServiceSettings struct {
	ContainerNamePrefix string                      `json:"container_name_prefix"`
	Overrides           map[string]*ServiceOverride `json:"overrides"`
}

type ServiceMetadata struct {
	Name                   string   `json:"name"`
	Image                  string   `json:"image"`
	DefaultContainerName   string   `json:"default_container_name"`
	EffectiveContainerName string   `json:"effective_container_name"`
	DefaultPorts           []string `json:"default_ports"`
}

func defaultServiceSettings() *ServiceSettings {
	return &ServiceSettings{
		Overrides: make(map[string]*ServiceOverride),
	}
}

func (t *DockerEnvironmentManager) loadServiceSettings() {
	db := database.GetMemoryDB()
	if db == nil {
		t.ServiceSettings = defaultServiceSettings()
		return
	}
	list := memory.FindAll[*ServiceSettingsEntity](db, "service_settings")
	if len(list) == 0 {
		t.ServiceSettings = defaultServiceSettings()
		return
	}
	e := list[0]
	t.ServiceSettings = &ServiceSettings{
		ContainerNamePrefix: e.ContainerNamePrefix,
		Overrides:           e.Overrides,
	}
	if t.ServiceSettings.Overrides == nil {
		t.ServiceSettings.Overrides = make(map[string]*ServiceOverride)
	}
}

func (t *DockerEnvironmentManager) GetServiceSettings() *ServiceSettings {
	if t.ServiceSettings == nil {
		t.loadServiceSettings()
	}
	return t.ServiceSettings.clone()
}

func (t *DockerEnvironmentManager) SaveServiceSettings(settings *ServiceSettings) error {
	sanitized := normalizeServiceSettings(settings)
	db := database.GetMemoryDB()
	if db == nil {
		t.ServiceSettings = sanitized
		return nil
	}
	list := memory.FindAll[*ServiceSettingsEntity](db, "service_settings")
	if len(list) == 0 {
		entity := &ServiceSettingsEntity{
			ContainerNamePrefix: sanitized.ContainerNamePrefix,
			Overrides:           sanitized.Overrides,
		}
		if err := memory.Create(db, "service_settings", entity); err != nil {
			return err
		}
	} else {
		list[0].ContainerNamePrefix = sanitized.ContainerNamePrefix
		list[0].Overrides = sanitized.Overrides
		if err := memory.Update(db, "service_settings", list[0]); err != nil {
			return err
		}
	}
	t.ServiceSettings = sanitized
	return nil
}

func normalizeServiceSettings(settings *ServiceSettings) *ServiceSettings {
	result := defaultServiceSettings()
	if settings == nil {
		return result
	}
	result.ContainerNamePrefix = strings.TrimSpace(settings.ContainerNamePrefix)
	for name, override := range settings.Overrides {
		normalized := normalizeOverride(override)
		if normalized != nil {
			result.Overrides[name] = normalized
		}
	}
	return result
}

func normalizeOverride(override *ServiceOverride) *ServiceOverride {
	if override == nil {
		return nil
	}
	normalized := &ServiceOverride{
		CustomName: strings.TrimSpace(override.CustomName),
	}
	seen := map[string]struct{}{}
	for _, port := range override.Ports {
		value := strings.TrimSpace(port)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		normalized.Ports = append(normalized.Ports, value)
	}
	if normalized.CustomName == "" && len(normalized.Ports) == 0 {
		return nil
	}
	return normalized
}

func (s *ServiceSettings) clone() *ServiceSettings {
	if s == nil {
		return defaultServiceSettings()
	}
	clone := &ServiceSettings{
		ContainerNamePrefix: s.ContainerNamePrefix,
		Overrides:           make(map[string]*ServiceOverride, len(s.Overrides)),
	}
	for name, override := range s.Overrides {
		if override == nil {
			continue
		}
		clone.Override(name, override)
	}
	return clone
}

func (s *ServiceSettings) Override(name string, override *ServiceOverride) {
	if s.Overrides == nil {
		s.Overrides = make(map[string]*ServiceOverride)
	}
	copy := &ServiceOverride{
		CustomName: override.CustomName,
		Ports:      append([]string{}, override.Ports...),
	}
	s.Overrides[name] = copy
}

func (t *DockerEnvironmentManager) serviceDefinitionWithOverrides(name string) interface{} {
	service, ok := t.GetService(name)
	if !ok {
		return nil
	}
	definition, ok := service.Original.(map[interface{}]interface{})
	if !ok {
		return service.Original
	}
	copied, ok := deepCopyInterface(definition).(map[interface{}]interface{})
	if !ok {
		return service.Original
	}

	if resolved := t.computeContainerName(name, copied); resolved != "" {
		copied["container_name"] = resolved
	}

	if override := t.getServiceOverride(name); override != nil && len(override.Ports) > 0 {
		ports := make([]interface{}, 0, len(override.Ports))
		for _, port := range override.Ports {
			ports = append(ports, port)
		}
		copied["ports"] = ports
	}

	return copied
}

func (t *DockerEnvironmentManager) getServiceOverride(name string) *ServiceOverride {
	if t.ServiceSettings == nil || t.ServiceSettings.Overrides == nil {
		return nil
	}
	if override, ok := t.ServiceSettings.Overrides[name]; ok {
		return override
	}
	return nil
}

func (t *DockerEnvironmentManager) computeContainerName(name string, definition map[interface{}]interface{}) string {
	base := name
	if raw, ok := definition["container_name"]; ok {
		if str, ok := raw.(string); ok && str != "" {
			base = str
		}
	}

	if override := t.getServiceOverride(name); override != nil && override.CustomName != "" {
		return override.CustomName
	}

	if t.ServiceSettings != nil && t.ServiceSettings.ContainerNamePrefix != "" {
		return fmt.Sprintf("%s-%s", t.ServiceSettings.ContainerNamePrefix, base)
	}

	if raw, ok := definition["container_name"]; ok {
		if str, ok := raw.(string); ok && str != "" {
			return str
		}
	}

	return ""
}

func (t *DockerEnvironmentManager) containerNameForService(name string) string {
	if def, ok := t.serviceDefinitionWithOverrides(name).(map[interface{}]interface{}); ok {
		if raw, ok := def["container_name"]; ok {
			if str, ok := raw.(string); ok && str != "" {
				return str
			}
		}
	}
	return name
}

func deepCopyInterface(value interface{}) interface{} {
	switch v := value.(type) {
	case map[interface{}]interface{}:
		copied := make(map[interface{}]interface{}, len(v))
		for key, val := range v {
			copied[key] = deepCopyInterface(val)
		}
		return copied
	case map[string]interface{}:
		copied := make(map[interface{}]interface{}, len(v))
		for key, val := range v {
			copied[key] = deepCopyInterface(val)
		}
		return copied
	case []interface{}:
		copied := make([]interface{}, len(v))
		for i, val := range v {
			copied[i] = deepCopyInterface(val)
		}
		return copied
	default:
		return v
	}
}

func (t *DockerEnvironmentManager) ListServiceMetadata() []ServiceMetadata {
	services := make([]ServiceMetadata, 0, len(t.Services))
	for _, service := range t.Services {
		name, _ := service.ContainerName.(string)
		defaultName := extractDefaultContainerName(name, service.Original)
		effectiveName := defaultName
		if resolved := t.containerNameForService(name); resolved != "" {
			effectiveName = resolved
		}
		services = append(services, ServiceMetadata{
			Name:                   name,
			Image:                  service.Image,
			DefaultContainerName:   defaultName,
			EffectiveContainerName: effectiveName,
			DefaultPorts:           extractPorts(service.Original),
		})
	}
	sort.Slice(services, func(i, j int) bool {
		return services[i].Name < services[j].Name
	})
	return services
}

func extractDefaultContainerName(name string, definition interface{}) string {
	if defMap, ok := definition.(map[interface{}]interface{}); ok {
		if raw, ok := defMap["container_name"]; ok {
			if str, ok := raw.(string); ok && str != "" {
				return str
			}
		}
	}
	return name
}

func extractPorts(definition interface{}) []string {
	defMap, ok := definition.(map[interface{}]interface{})
	if !ok {
		return []string{}
	}
	raw, ok := defMap["ports"]
	if !ok {
		return []string{}
	}
	switch ports := raw.(type) {
	case []interface{}:
		var result []string
		for _, port := range ports {
			if value := stringifyPort(port); value != "" {
				result = append(result, value)
			}
		}
		return result
	case []string:
		return append([]string{}, ports...)
	default:
		if value := stringifyPort(ports); value != "" {
			return []string{value}
		}
	}
	return []string{}
}

func stringifyPort(port interface{}) string {
	switch v := port.(type) {
	case string:
		return v
	case fmt.Stringer:
		return v.String()
	case map[interface{}]interface{}:
		target := fmt.Sprint(v["target"])
		published := fmt.Sprint(v["published"])
		protocol := fmt.Sprint(v["protocol"])
		if published == "" || published == "<nil>" {
			return target
		}
		if protocol != "" && protocol != "<nil>" {
			return fmt.Sprintf("%s:%s/%s", published, target, protocol)
		}
		return fmt.Sprintf("%s:%s", published, target)
	default:
		return fmt.Sprint(v)
	}
}

func (t *DockerEnvironmentManager) ReapplyServiceSettings() {
	t.createComposeFile(t.ActiveServices)
	for _, service := range t.ActiveServices {
		t.command.RunCommand(t.GetWorkDir(), "docker-compose", "up", "-d", "--force-recreate", service)
	}
	t.Init()
}
