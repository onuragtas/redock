package repository

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// An admin has to see every menu there is, except opt-in (hidden) modules,
// which appear once granted. The path list used to be maintained by hand
// alongside the item list, so a page added to one and not the other was
// filtered out of the menu with nothing to show it had happened.
func TestAdminSeesEveryMenuItem(t *testing.T) {
	var hidden []string
	for _, item := range AllMenuItems {
		if item.Hidden {
			hidden = append(hidden, item.Path)
		}
	}

	seen := make(map[string]bool)
	for _, item := range GetMenuItemsForUser(AdminRoleName, nil) {
		seen[item.Path] = true
	}
	for _, item := range AllMenuItems {
		if seen[item.Path] == item.Hidden {
			t.Errorf("%q (%s): hidden=%v but shown to an admin by default = %v", item.Path, item.Name, item.Hidden, seen[item.Path])
		}
	}

	// Granting hidden items makes them appear, alongside everything else.
	all := GetMenuItemsForUser(AdminRoleName, hidden)
	if len(all) != len(AllMenuItems) {
		t.Fatalf("an admin with the opt-in menus granted sees %d of %d items", len(all), len(AllMenuItems))
	}
}

// Hidden modules never reach a regular user by default either.
func TestHiddenMenusAreOptIn(t *testing.T) {
	for _, item := range GetMenuItemsForUser("user", nil) {
		if item.Hidden {
			t.Errorf("%q is hidden but in the default user menu", item.Path)
		}
	}
	got := GetMenuItemsForUser("user", []string{"/rtc"})
	if len(got) != 1 || got[0].Path != "/rtc" {
		t.Fatalf("granted /rtc to a user, got %+v", got)
	}
}

// A menu entry that points nowhere is a dead link; the router has to know the
// path too.
func TestEveryMenuPathHasARoute(t *testing.T) {
	router, err := os.ReadFile("../../web/src/router/index.js")
	if err != nil {
		t.Skipf("the router is not readable from here: %v", err)
	}

	routed := make(map[string]bool)
	routed["/"] = true
	for _, match := range regexp.MustCompile(`path:\s*'([^']*)'`).FindAllStringSubmatch(string(router), -1) {
		path := match[1]
		if path == "" || strings.HasPrefix(path, ":") {
			continue
		}
		// A route may carry parameters, as in "exec/:id?"; the menu links to
		// the part before them.
		if i := strings.Index(path, "/:"); i >= 0 {
			path = path[:i]
		}
		routed["/"+strings.TrimPrefix(path, "/")] = true
	}

	for _, item := range AllMenuItems {
		if !routed[item.Path] {
			t.Errorf("the menu offers %q but the router has no such page", item.Path)
		}
	}
}

// The defaults a plain user gets must all be real menus.
func TestDefaultUserMenusExist(t *testing.T) {
	known := make(map[string]bool, len(AllMenuItems))
	for _, item := range AllMenuItems {
		known[item.Path] = true
	}
	for _, path := range DefaultUserMenuPaths {
		if !known[path] {
			t.Errorf("the default menu list offers %q, which is not a menu item", path)
		}
	}
}

// The menu names its icons; the dashboard maps those names to drawings. An icon
// the map does not know is handed to the SVG as its own name, which the browser
// refuses to draw. The two sides live in different languages, so only a test
// that reads both can keep them in step.
func TestEveryMenuIconIsKnownToTheDashboard(t *testing.T) {
	icons, err := os.ReadFile("../../web/src/menuIcons.js")
	if err != nil {
		t.Skipf("the icon map is not readable from here: %v", err)
	}

	known := make(map[string]bool)
	for _, match := range regexp.MustCompile(`\bmdi[A-Za-z0-9]+\b`).FindAllString(string(icons), -1) {
		known[match] = true
	}

	for _, item := range AllMenuItems {
		if item.Icon == "" {
			continue
		}
		if !known[item.Icon] {
			t.Errorf("the menu asks for the icon %q (%s), which the dashboard cannot draw", item.Icon, item.Path)
		}
	}
}
