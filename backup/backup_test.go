package backup

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// withTempHome redirects $HOME so backups land in a clean tmp dir per test.
func withTempHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	return home
}

// seedDataDir creates $workDir/data with a mix of files and subdirs and
// returns the workDir.
func seedDataDir(t *testing.T) string {
	t.Helper()
	workDir := t.TempDir()
	dataDir := filepath.Join(workDir, "data")
	if err := os.MkdirAll(filepath.Join(dataDir, "subA"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dataDir, "subB", "nested"), 0o755); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"users.json":              `[{"id":1,"email":"a@b"}]`,
		"subA/api_gateway.json":   `{"routes":[]}`,
		"subB/dns.json":           `{"hosts":[]}`,
		"subB/nested/deep.json":   `{"x":1}`,
	}
	for rel, content := range files {
		full := filepath.Join(dataDir, rel)
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return workDir
}

func TestCreateAndList(t *testing.T) {
	withTempHome(t)
	workDir := seedDataDir(t)

	info, err := Create(workDir, "1.2.3", "manual")
	assert.NoError(t, err)
	assert.NotNil(t, info)
	assert.Equal(t, 4, info.FileCount)
	assert.Greater(t, info.SizeBytes, int64(0))
	assert.Greater(t, info.UncompressedB, int64(0))
	assert.Equal(t, "manual", info.TriggerReason)

	list, err := List()
	assert.NoError(t, err)
	assert.Len(t, list, 1)
	assert.Equal(t, info.ID, list[0].ID)
	assert.Equal(t, "1.2.3", list[0].RedockVersion)
	assert.Equal(t, 4, list[0].FileCount)
}

func TestCreateMissingSourceFails(t *testing.T) {
	withTempHome(t)
	workDir := t.TempDir() // no data/ subdir

	_, err := Create(workDir, "1.0.0", "manual")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "source not found")
}

func TestRoundTripRestore(t *testing.T) {
	// Create a backup, mutate the live data, restore, and verify the original
	// state is back.
	withTempHome(t)
	workDir := seedDataDir(t)

	originalJSON, err := os.ReadFile(filepath.Join(workDir, "data", "users.json"))
	assert.NoError(t, err)

	created, err := Create(workDir, "v1", "manual")
	assert.NoError(t, err)

	// Mutate live data after backup.
	mutPath := filepath.Join(workDir, "data", "users.json")
	assert.NoError(t, os.WriteFile(mutPath, []byte(`[{"id":99,"email":"mutated"}]`), 0o644))
	// Add a brand-new file post-backup. Restore should remove it (full data swap).
	junk := filepath.Join(workDir, "data", "junk.json")
	assert.NoError(t, os.WriteFile(junk, []byte(`{"junk":true}`), 0o644))

	assert.NoError(t, Restore(created.ID, workDir))

	// Original file is back.
	got, err := os.ReadFile(mutPath)
	assert.NoError(t, err)
	assert.Equal(t, string(originalJSON), string(got))

	// Junk file is gone.
	_, err = os.Stat(junk)
	assert.True(t, os.IsNotExist(err), "post-backup files must be wiped on restore")
}

func TestDelete(t *testing.T) {
	withTempHome(t)
	workDir := seedDataDir(t)

	info, err := Create(workDir, "v1", "manual")
	assert.NoError(t, err)

	assert.NoError(t, Delete(info.ID))

	_, err = Path(info.ID)
	assert.Error(t, err)
}

func TestPathRejectsTraversal(t *testing.T) {
	withTempHome(t)

	cases := []string{
		"../etc/passwd",
		"foo/bar",
		"foo\\bar",
		"..",
		"",
	}
	for _, id := range cases {
		_, err := Path(id)
		assert.Error(t, err, "id %q must be rejected", id)
		if err != nil {
			assert.True(t,
				strings.Contains(err.Error(), "invalid backup id") ||
					strings.Contains(err.Error(), "not found"),
				"id %q got unexpected error: %v", id, err)
		}
	}
}

func TestCreateSkipsUnreadableFiles(t *testing.T) {
	// A 0-mode file (e.g. real-world: an .encryption.key owned by root with
	// 0600) must not abort the backup; it should be recorded as skipped.
	withTempHome(t)
	workDir := seedDataDir(t)
	secret := filepath.Join(workDir, "data", "secret.key")
	if err := os.WriteFile(secret, []byte("hush"), 0o000); err != nil {
		t.Fatal(err)
	}
	// Always restore mode so t.TempDir() cleanup can remove the file.
	t.Cleanup(func() { _ = os.Chmod(secret, 0o644) })

	info, err := Create(workDir, "v1", "manual")
	assert.NoError(t, err, "permission-denied files must not abort backup")
	assert.NotNil(t, info)
	assert.Equal(t, 1, info.SkippedCount, "unreadable file should be reported as skipped")
	// Other files are still archived.
	assert.Equal(t, 4, info.FileCount)
}

func TestPruneKeepsNewestN(t *testing.T) {
	withTempHome(t)
	workDir := seedDataDir(t)

	// Force MaxBackups=3 then make 5 backups; expect oldest 2 to be pruned
	// automatically by the post-Create hook.
	assert.NoError(t, SaveConfig(Config{MaxBackups: 3}))

	var ids []string
	for i := 0; i < 5; i++ {
		info, err := Create(workDir, "v1", "manual")
		assert.NoError(t, err)
		ids = append(ids, info.ID)
	}

	list, err := List()
	assert.NoError(t, err)
	assert.Len(t, list, 3, "after creating 5 with MaxBackups=3, only 3 should remain")

	// The 3 survivors must be the newest 3 — i.e. ids[2], ids[3], ids[4].
	survivorIDs := map[string]bool{}
	for _, b := range list {
		survivorIDs[b.ID] = true
	}
	assert.True(t, survivorIDs[ids[2]])
	assert.True(t, survivorIDs[ids[3]])
	assert.True(t, survivorIDs[ids[4]])
	assert.False(t, survivorIDs[ids[0]], "oldest should have been pruned")
	assert.False(t, survivorIDs[ids[1]], "second oldest should have been pruned")
}

func TestLoadConfigDefaults(t *testing.T) {
	withTempHome(t)
	cfg, err := LoadConfig()
	assert.NoError(t, err)
	assert.Equal(t, defaultMaxBackups, cfg.MaxBackups)
}

func TestSaveConfigClampsInvalid(t *testing.T) {
	withTempHome(t)
	assert.NoError(t, SaveConfig(Config{MaxBackups: -5}))
	cfg, err := LoadConfig()
	assert.NoError(t, err)
	assert.Equal(t, defaultMaxBackups, cfg.MaxBackups, "non-positive max_backups must clamp to default")
}

func TestImportRoundTrip(t *testing.T) {
	// Create a backup, "download" it (read the file), wipe the backup dir,
	// then import it back. The restored Info must match the original.
	withTempHome(t)
	workDir := seedDataDir(t)
	created, err := Create(workDir, "v1", "manual")
	assert.NoError(t, err)

	srcPath, err := Path(created.ID)
	assert.NoError(t, err)
	body, err := os.ReadFile(srcPath)
	assert.NoError(t, err)

	// Wipe and re-import.
	dir, _ := BackupDir()
	assert.NoError(t, os.RemoveAll(dir))
	imported, err := Import(strings.NewReader(string(body)))
	assert.NoError(t, err)
	assert.Equal(t, created.ID, imported.ID)
	assert.Equal(t, created.FileCount, imported.FileCount)
	assert.Equal(t, "manual", imported.TriggerReason)

	// Re-importing the same archive must fail (duplicate ID).
	_, err = Import(strings.NewReader(string(body)))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already exists")
}

func TestImportRejectsBadArchive(t *testing.T) {
	withTempHome(t)
	_, err := Import(strings.NewReader("not a valid tar.gz"))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not a valid redock backup")
}

func TestListSurfacesUnreadableArchive(t *testing.T) {
	// A corrupt .tar.gz in the dir must not break List(); we surface it as
	// a stub with reason="unreadable".
	home := withTempHome(t)
	workDir := seedDataDir(t)
	if _, err := Create(workDir, "v1", "manual"); err != nil {
		t.Fatal(err)
	}
	// Drop a corrupt file in the backup dir.
	corruptPath := filepath.Join(home, backupDirName, filenamePrefix+"99999999_999999"+filenameExt)
	assert.NoError(t, os.WriteFile(corruptPath, []byte("not a tarball"), 0o644))

	list, err := List()
	assert.NoError(t, err)
	assert.Len(t, list, 2)
	foundUnreadable := false
	for _, b := range list {
		if b.TriggerReason == "unreadable" {
			foundUnreadable = true
		}
	}
	assert.True(t, foundUnreadable, "corrupt archive should surface with reason=unreadable")
}
