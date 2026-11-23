package repository

import (
	"testing"

	"redock/ssh_server"
)

// While the web terminal kill switch is off, its page is in no menu, including
// menus stored for a user before the switch was turned off.
func TestTerminalNotInMenusWhenDisabled(t *testing.T) {
	if ssh_server.WebTerminalEnabled {
		t.Skip("web terminal enabled")
	}
	for _, item := range GetMenuItemsForUser(AdminRoleName, nil) {
		if item.Path == "/exec" {
			t.Fatal("admin menu lists the web terminal")
		}
	}
	for _, item := range GetMenuItemsForUser("user", []string{"/", "/exec"}) {
		if item.Path == "/exec" {
			t.Fatal("stored user menu lists the web terminal")
		}
	}
	for _, p := range append(append([]string{}, AllMenuPaths...), DefaultUserMenuPaths...) {
		if p == "/exec" {
			t.Fatal("menu paths list the web terminal")
		}
	}
	if got := WithoutDisabledPaths([]string{"/", "/exec"}); len(got) != 1 || got[0] != "/" {
		t.Fatalf("WithoutDisabledPaths = %v", got)
	}
}
