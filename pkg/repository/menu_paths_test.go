package repository

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// An admin has to see every menu there is. The path list used to be maintained
// by hand alongside the item list, so a page added to one and not the other was
// filtered out of the menu with nothing to show it had happened.
func TestAdminSeesEveryMenuItem(t *testing.T) {
	items := GetMenuItemsForUser(AdminRoleName, nil)
	if len(items) != len(AllMenuItems) {
		t.Fatalf("an admin sees %d of %d menu items", len(items), len(AllMenuItems))
	}

	seen := make(map[string]bool, len(items))
	for _, item := range items {
		seen[item.Path] = true
	}
	for _, item := range AllMenuItems {
		if !seen[item.Path] {
			t.Errorf("%q (%s) is defined but never reaches the menu", item.Path, item.Name)
		}
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
