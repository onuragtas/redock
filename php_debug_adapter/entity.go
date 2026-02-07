package php_debug_adapter

import "redock/platform/memory"

// PhpXDebugSettingsEntity listen adresi (tek satır).
type PhpXDebugSettingsEntity struct {
	memory.BaseEntity
	Listen string `json:"listen"`
}

// PhpXDebugMappingEntity path mapping (Name, Path, URL).
type PhpXDebugMappingEntity struct {
	memory.BaseEntity
	Name string `json:"name"`
	Path string `json:"path"`
	URL  string `json:"url"`
}
