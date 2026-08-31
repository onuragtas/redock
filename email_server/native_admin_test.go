package email_server

import (
	"os"
	"reflect"
	"strings"
	"testing"
)

func readAdminSource() (string, error) {
	data, err := os.ReadFile("native_admin.go")
	return string(data), err
}

// UpdateNativeSettings copies the configuration field by field, so a setting
// added to the model and to the dashboard but not to that list is accepted by
// the API, reported as saved, and silently thrown away. This checks the list
// keeps up with the struct.
func TestEverySettableFieldIsCarriedThrough(t *testing.T) {
	// Fields the update path is not supposed to take from the request.
	notSettable := map[string]bool{
		"SoftDeleteEntity": true, // identity and timestamps
		"Name":             true,
		"IsRunning":        true, // live state, not configuration
		"DataPath":         true, // resolved from the work directory
		"LastStarted":      true,
		"LastStopped":      true,
		"DKIMEnabled":      true, // managed with the signing keys
	}

	source, err := readAdminSource()
	if err != nil {
		t.Fatalf("could not read the update path: %v", err)
	}

	config := reflect.TypeOf(EmailServerConfig{})
	for i := 0; i < config.NumField(); i++ {
		name := config.Field(i).Name
		if notSettable[name] {
			continue
		}
		if !strings.Contains(source, "current."+name+" =") {
			t.Errorf("EmailServerConfig.%s can be sent by the dashboard but is never copied in UpdateNativeSettings", name)
		}
	}
}
