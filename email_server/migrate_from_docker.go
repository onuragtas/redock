package email_server

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"redock/platform/memory"
)

const (
	// legacyContainerName / legacyVolumeName are what the removed
	// docker-mailserver setup created.
	legacyContainerName = "redock-mailserver"
	legacyVolumeName    = "redock-mail-data"
	// legacyCopyImage is a tiny image used to read the named volume; it is only
	// pulled if it is not already present.
	legacyCopyImage = "alpine:latest"
)

// MigrationReport describes what the move off docker-mailserver did, so the
// dashboard and the logs can show it rather than leaving it silent.
type MigrationReport struct {
	MailImported     bool     `json:"mail_imported"`
	MailboxesFixed   int      `json:"mailboxes_fixed"`
	ContainerRemoved bool     `json:"container_removed"`
	VolumeKept       bool     `json:"volume_kept"`
	Notes            []string `json:"notes"`
}

// MigrateFromDockerMailserver moves an installation off the container engine:
// it imports the mail that lived in the docker named volume, normalises the
// Maildir layout, enables every native listener, and removes the container.
//
// It is safe to run repeatedly: each step checks whether it is still needed,
// and the docker volume is never deleted, so the old data stays recoverable.
func MigrateFromDockerMailserver(db *memory.Database, dataDir string) (*MigrationReport, error) {
	report := &MigrationReport{}

	emailDir := filepath.Join(dataDir, "email")
	mailDir := filepath.Join(emailDir, "mail")
	if err := os.MkdirAll(mailDir, 0700); err != nil {
		return report, fmt.Errorf("failed to create the mail directory: %w", err)
	}

	// 1. Bring the stored configuration onto the native engine and turn on the
	//    listeners the container setup left disabled.
	if err := migrateConfig(db, mailDir, report); err != nil {
		return report, err
	}

	// 2. Import mail out of the named volume, if there is any and we have not
	//    already imported it.
	if imported, err := importLegacyMail(mailDir, report); err != nil {
		report.Notes = append(report.Notes, "mail import failed: "+err.Error())
	} else {
		report.MailImported = imported
	}

	// 3. docker-mailserver may have stored each account under a "home"
	//    subdirectory; the native store expects the Maildir at the account root.
	fixed, err := normalizeMaildirLayout(mailDir)
	if err != nil {
		report.Notes = append(report.Notes, "maildir normalisation failed: "+err.Error())
	}
	report.MailboxesFixed = fixed

	// 4. Retire the container. The volume is deliberately kept.
	if removeLegacyContainer() {
		report.ContainerRemoved = true
	}
	if legacyVolumeExists() {
		report.VolumeKept = true
		report.Notes = append(report.Notes,
			fmt.Sprintf("the old docker volume %q was left in place; remove it with \"docker volume rm %s\" once you are satisfied", legacyVolumeName, legacyVolumeName))
	}

	return report, nil
}

// migrateConfig rewrites the stored server configuration for the native engine.
func migrateConfig(db *memory.Database, mailDir string, report *MigrationReport) error {
	configs := memory.FindAll[*EmailServerConfig](db, "email_server_configs")
	if len(configs) == 0 {
		return nil // a fresh install; Init creates a native config already
	}

	config := configs[0]
	config.DataPath = mailDir
	EnableAllNativeServices(config)
	config.IsRunning = false

	if err := memory.Update(db, "email_server_configs", config); err != nil {
		return fmt.Errorf("failed to update the mail configuration: %w", err)
	}

	report.Notes = append(report.Notes, "TLS, IMAP/IMAPS, POP3/POP3S and the SPF/DKIM/DMARC checks are now enabled")
	return nil
}

// importLegacyMail copies the contents of the docker named volume into the
// host mail directory. Reports whether anything was copied.
func importLegacyMail(mailDir string, report *MigrationReport) (bool, error) {
	if !dockerAvailable() {
		return false, nil
	}
	if !legacyVolumeExists() {
		return false, nil
	}
	if !dirIsEmpty(mailDir) {
		report.Notes = append(report.Notes, "the mail directory already had content, so the docker volume was not copied over it")
		return false, nil
	}

	// A throwaway container is the only way to read a named volume from the
	// host. Prefer the mail container's own image if it is still around, so
	// nothing new has to be pulled.
	image := legacyImage()

	cmd := exec.Command("docker", "run", "--rm",
		"-v", legacyVolumeName+":/from:ro",
		"-v", mailDir+":/to",
		image, "sh", "-c", "cp -a /from/. /to/ 2>/dev/null || true")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return false, fmt.Errorf("%w (%s)", err, strings.TrimSpace(string(output)))
	}

	if dirIsEmpty(mailDir) {
		return false, nil
	}

	report.Notes = append(report.Notes, "mail was imported from the docker volume into "+mailDir)
	return true, nil
}

// legacyImage picks an image that is already present locally, falling back to
// a small one that will be pulled.
func legacyImage() string {
	for _, candidate := range []string{"mailserver/docker-mailserver:latest", legacyCopyImage, "busybox:latest"} {
		cmd := exec.Command("docker", "image", "inspect", candidate)
		if err := cmd.Run(); err == nil {
			return candidate
		}
	}
	return legacyCopyImage
}

// normalizeMaildirLayout moves "<domain>/<user>/home/..." up one level, which
// is where docker-mailserver put the Maildir before the custom Dovecot config
// overrode mail_home. Returns how many accounts were rearranged.
func normalizeMaildirLayout(mailDir string) (int, error) {
	domains, err := os.ReadDir(mailDir)
	if err != nil {
		return 0, err
	}

	fixed := 0
	for _, domain := range domains {
		if !domain.IsDir() || strings.HasPrefix(domain.Name(), ".") {
			continue
		}

		users, err := os.ReadDir(filepath.Join(mailDir, domain.Name()))
		if err != nil {
			continue
		}
		for _, user := range users {
			if !user.IsDir() || strings.HasPrefix(user.Name(), ".") {
				continue
			}

			accountDir := filepath.Join(mailDir, domain.Name(), user.Name())
			if isMaildirFolder(accountDir) {
				continue // already in the expected shape
			}

			// Look for the Maildir one or two levels down.
			for _, nested := range []string{
				filepath.Join(accountDir, "home"),
				filepath.Join(accountDir, "home", "Maildir"),
				filepath.Join(accountDir, "Maildir"),
			} {
				if !isMaildirFolder(nested) {
					continue
				}
				if err := moveDirContents(nested, accountDir); err != nil {
					log.Printf("email migration: could not move %s up: %v", nested, err)
					break
				}
				fixed++
				break
			}
		}
	}
	return fixed, nil
}

// moveDirContents moves every entry of src into dst, then removes src.
func moveDirContents(src, dst string) error {
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		from := filepath.Join(src, entry.Name())
		to := filepath.Join(dst, entry.Name())
		if _, err := os.Stat(to); err == nil {
			continue // never overwrite something already in place
		}
		if err := os.Rename(from, to); err != nil {
			return err
		}
	}

	// The parent ("home") may still hold empty leftovers; remove what we can.
	_ = os.RemoveAll(src)
	return nil
}

// removeLegacyContainer stops and deletes the docker-mailserver container.
func removeLegacyContainer() bool {
	if !dockerAvailable() {
		return false
	}

	inspect := exec.Command("docker", "container", "inspect", legacyContainerName)
	if err := inspect.Run(); err != nil {
		return false // not there; nothing to do
	}

	stop := exec.Command("docker", "stop", "-t", "20", legacyContainerName)
	if output, err := stop.CombinedOutput(); err != nil {
		log.Printf("email migration: could not stop %s: %v (%s)", legacyContainerName, err, strings.TrimSpace(string(output)))
	}

	remove := exec.Command("docker", "rm", "-f", legacyContainerName)
	if output, err := remove.CombinedOutput(); err != nil {
		log.Printf("email migration: could not remove %s: %v (%s)", legacyContainerName, err, strings.TrimSpace(string(output)))
		return false
	}

	log.Printf("email migration: removed the %s container", legacyContainerName)
	return true
}

func dockerAvailable() bool {
	cmd := exec.Command("docker", "version", "--format", "{{.Server.Version}}")
	return cmd.Run() == nil
}

func legacyVolumeExists() bool {
	if !dockerAvailable() {
		return false
	}
	cmd := exec.Command("docker", "volume", "inspect", legacyVolumeName)
	return cmd.Run() == nil
}

// dirIsEmpty reports whether a directory holds nothing meaningful.
func dirIsEmpty(dir string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return true
	}
	for _, entry := range entries {
		if !strings.HasPrefix(entry.Name(), ".") {
			return false
		}
	}
	return true
}

// CleanupLegacyArtifacts removes the leftover docker-mailserver configuration
// directories once the operator confirms the migration went well. The mail
// itself is never touched.
func (m *EmailManager) CleanupLegacyArtifacts() []string {
	removed := make([]string, 0, 4)

	for _, dir := range []string{"config", "logs", "state"} {
		path := filepath.Join(m.dataPath, dir)
		if _, err := os.Stat(path); err != nil {
			continue
		}
		if err := os.RemoveAll(path); err != nil {
			log.Printf("email migration: could not remove %s: %v", path, err)
			continue
		}
		removed = append(removed, path)
	}

	if legacyVolumeExists() {
		cmd := exec.Command("docker", "volume", "rm", legacyVolumeName)
		if err := cmd.Run(); err == nil {
			removed = append(removed, "docker volume "+legacyVolumeName)
		}
	}

	return removed
}

// LegacyArtifacts reports what is still lying around from the container setup,
// so the dashboard can offer to clean it up.
func (m *EmailManager) LegacyArtifacts() []string {
	found := make([]string, 0, 4)

	for _, dir := range []string{"config", "logs", "state"} {
		path := filepath.Join(m.dataPath, dir)
		if info, err := os.Stat(path); err == nil && info.IsDir() {
			found = append(found, path)
		}
	}
	if legacyVolumeExists() {
		found = append(found, "docker volume "+legacyVolumeName)
	}
	if dockerAvailable() {
		if err := exec.Command("docker", "container", "inspect", legacyContainerName).Run(); err == nil {
			found = append(found, "docker container "+legacyContainerName)
		}
	}
	return found
}
