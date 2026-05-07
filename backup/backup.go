// Package backup creates and restores tar.gz snapshots of the redock data
// directory ($DOCKER_WORK_DIR/data/), where the in-memory DB persists every
// entity as JSON. Backups live in $HOME/redock_backup/ and carry a
// manifest.json with metadata + SHA256 for integrity.
package backup

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	backupDirName     = "redock_backup"
	manifestFileName  = "manifest.json"
	dataSubdir        = "data" // backed-up subdir inside $DOCKER_WORK_DIR
	filenamePrefix    = "redock_backup_"
	filenameTimestamp = "20060102_150405"
	filenameExt       = ".tar.gz"
)

// Manifest captures backup metadata, embedded inside the tar.gz so a backup
// is self-describing and verifiable after a copy/move.
type Manifest struct {
	ID             string    `json:"id"`              // file basename without extension
	CreatedAt      time.Time `json:"created_at"`
	RedockVersion  string    `json:"redock_version,omitempty"`
	SourcePath     string    `json:"source_path"`
	FileCount      int       `json:"file_count"`
	UncompressedB  int64     `json:"uncompressed_bytes"`
	ContentSHA256  string    `json:"content_sha256"`  // hash of (path|size|mtime) tuples — detects content drift
	TriggerReason  string    `json:"trigger_reason"`  // "manual", "pre-update", "pre-restore"
	SkippedFiles   []string  `json:"skipped_files,omitempty"` // unreadable entries (e.g. 0600 secrets owned by root)
}

// Info is what the API returns to the frontend (manifest + on-disk size).
type Info struct {
	ID            string    `json:"id"`
	Filename      string    `json:"filename"`
	CreatedAt     time.Time `json:"created_at"`
	SizeBytes     int64     `json:"size_bytes"`         // compressed file size
	UncompressedB int64     `json:"uncompressed_bytes"`
	FileCount     int       `json:"file_count"`
	SkippedCount  int       `json:"skipped_count,omitempty"`
	RedockVersion string    `json:"redock_version,omitempty"`
	TriggerReason string    `json:"trigger_reason"`
}

// BackupDir returns the absolute path of the backup directory ($HOME/redock_backup),
// creating it if missing.
func BackupDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home dir: %w", err)
	}
	dir := filepath.Join(home, backupDirName)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create backup dir: %w", err)
	}
	return dir, nil
}

// SourceDataDir returns the absolute path of the data directory we back up
// ($DOCKER_WORK_DIR/data). The caller passes the work dir from
// docker-manager so this package stays decoupled from it.
func SourceDataDir(workDir string) string {
	return filepath.Join(workDir, dataSubdir)
}

// Create writes a new backup of $workDir/data/ and returns its info. reason
// is recorded in the manifest (e.g. "manual", "pre-update"). version may be
// empty.
func Create(workDir, version, reason string) (*Info, error) {
	src := SourceDataDir(workDir)
	if st, err := os.Stat(src); err != nil || !st.IsDir() {
		return nil, fmt.Errorf("source not found or not a directory: %s", src)
	}

	dir, err := BackupDir()
	if err != nil {
		return nil, err
	}

	now := time.Now()
	// Nanosecond suffix prevents ID collisions when Create is called twice
	// in the same second — e.g. Restore() takes a "pre-restore" safety
	// snapshot right before extracting the original; without nanos the
	// safety snapshot would overwrite the source archive on disk.
	id := fmt.Sprintf("%s%s_%09d", filenamePrefix, now.Format(filenameTimestamp), now.Nanosecond())
	tmpPath := filepath.Join(dir, id+filenameExt+".tmp")
	finalPath := filepath.Join(dir, id+filenameExt)
	if _, err := os.Stat(finalPath); err == nil {
		return nil, fmt.Errorf("backup file already exists at %s", finalPath)
	}

	manifest := &Manifest{
		ID:            id,
		CreatedAt:     now,
		RedockVersion: version,
		SourcePath:    src,
		TriggerReason: reason,
	}

	count, totalBytes, _, err := writeArchive(tmpPath, src, manifest)
	if err != nil {
		os.Remove(tmpPath)
		return nil, err
	}
	if err := os.Rename(tmpPath, finalPath); err != nil {
		os.Remove(tmpPath)
		return nil, fmt.Errorf("finalize backup: %w", err)
	}

	st, err := os.Stat(finalPath)
	if err != nil {
		return nil, err
	}

	pruneToConfiguredLimit()

	return &Info{
		ID:            id,
		Filename:      filepath.Base(finalPath),
		CreatedAt:     now,
		SizeBytes:     st.Size(),
		UncompressedB: totalBytes,
		FileCount:     count,
		SkippedCount:  len(manifest.SkippedFiles),
		RedockVersion: version,
		TriggerReason: reason,
	}, nil
}

// writeArchive walks src and writes a .tar.gz at out, plus the manifest.json
// entry at the archive root. Returns file count, uncompressed bytes, and a
// stable content hash (of "path|size|mtime" tuples).
func writeArchive(out, src string, manifest *Manifest) (int, int64, string, error) {
	f, err := os.Create(out)
	if err != nil {
		return 0, 0, "", fmt.Errorf("create archive: %w", err)
	}
	defer f.Close()

	gz := gzip.NewWriter(f)
	defer gz.Close()
	tw := tar.NewWriter(gz)
	defer tw.Close()

	hasher := sha256.New()
	count := 0
	var totalBytes int64

	var skipped []string

	walkErr := filepath.Walk(src, func(path string, info os.FileInfo, walkErr error) error {
		// Stat / Lstat failures land here. Permission errors are non-fatal:
		// log + record + skip, so a single 0600 secret (e.g. an
		// .encryption.key owned by root) doesn't abort the whole backup.
		if walkErr != nil {
			if os.IsPermission(walkErr) {
				rel, _ := filepath.Rel(src, path)
				if rel == "" {
					rel = path
				}
				skipped = append(skipped, rel)
				log.Printf("backup: skipping (permission denied): %s", path)
				if info != nil && info.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
			return walkErr
		}
		if path == src {
			return nil
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		// Use forward slashes inside the archive so it's portable.
		rel = filepath.ToSlash(rel)

		if info.IsDir() {
			hdr := &tar.Header{
				Name:     "data/" + rel + "/",
				Mode:     int64(info.Mode().Perm()),
				ModTime:  info.ModTime(),
				Typeflag: tar.TypeDir,
			}
			return tw.WriteHeader(hdr)
		}
		if !info.Mode().IsRegular() {
			// Skip symlinks/sockets/devices — data dir should be JSON files.
			return nil
		}
		// Open the file BEFORE writing the tar header — if the file is
		// unreadable we want to skip it cleanly, not leave a header with no
		// body in the archive.
		fileR, err := os.Open(path)
		if err != nil {
			if os.IsPermission(err) {
				skipped = append(skipped, rel)
				log.Printf("backup: skipping (permission denied): %s", path)
				return nil
			}
			return err
		}
		defer fileR.Close()
		hdr := &tar.Header{
			Name:    "data/" + rel,
			Mode:    int64(info.Mode().Perm()),
			Size:    info.Size(),
			ModTime: info.ModTime(),
		}
		if err := tw.WriteHeader(hdr); err != nil {
			return err
		}
		n, err := io.Copy(tw, fileR)
		if err != nil {
			return err
		}
		count++
		totalBytes += n
		fmt.Fprintf(hasher, "%s|%d|%d\n", rel, info.Size(), info.ModTime().UnixNano())
		return nil
	})
	if walkErr != nil {
		return 0, 0, "", fmt.Errorf("walk source: %w", walkErr)
	}

	manifest.FileCount = count
	manifest.UncompressedB = totalBytes
	manifest.ContentSHA256 = hex.EncodeToString(hasher.Sum(nil))
	manifest.SkippedFiles = skipped

	mb, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return 0, 0, "", err
	}
	if err := tw.WriteHeader(&tar.Header{
		Name:    manifestFileName,
		Mode:    0o644,
		Size:    int64(len(mb)),
		ModTime: time.Now(),
	}); err != nil {
		return 0, 0, "", err
	}
	if _, err := tw.Write(mb); err != nil {
		return 0, 0, "", err
	}

	return count, totalBytes, manifest.ContentSHA256, nil
}

// List returns all backups currently in $HOME/redock_backup/, newest first.
// Reads the manifest from each archive on demand.
func List() ([]Info, error) {
	dir, err := BackupDir()
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	out := make([]Info, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), filenameExt) {
			continue
		}
		full := filepath.Join(dir, e.Name())
		info, err := readInfo(full)
		if err != nil {
			// Don't kill the listing for one bad archive; surface a stub.
			fi, _ := os.Stat(full)
			id := strings.TrimSuffix(e.Name(), filenameExt)
			stub := Info{ID: id, Filename: e.Name()}
			if fi != nil {
				stub.SizeBytes = fi.Size()
				stub.CreatedAt = fi.ModTime()
				stub.TriggerReason = "unreadable"
			}
			out = append(out, stub)
			continue
		}
		out = append(out, *info)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out, nil
}

func readInfo(path string) (*Info, error) {
	st, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	m, err := readManifest(path)
	if err != nil {
		return nil, err
	}
	return &Info{
		ID:            m.ID,
		Filename:      filepath.Base(path),
		CreatedAt:     m.CreatedAt,
		SizeBytes:     st.Size(),
		UncompressedB: m.UncompressedB,
		FileCount:     m.FileCount,
		SkippedCount:  len(m.SkippedFiles),
		RedockVersion: m.RedockVersion,
		TriggerReason: m.TriggerReason,
	}, nil
}

func readManifest(archivePath string) (*Manifest, error) {
	f, err := os.Open(archivePath)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return nil, err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if hdr.Name == manifestFileName {
			body, err := io.ReadAll(tr)
			if err != nil {
				return nil, err
			}
			var m Manifest
			if err := json.Unmarshal(body, &m); err != nil {
				return nil, err
			}
			return &m, nil
		}
	}
	return nil, fmt.Errorf("manifest not found in archive %s", archivePath)
}

// Path returns the absolute on-disk path of a backup by ID, or an error if
// the ID is malformed or the file doesn't exist. Resolves symlinks and
// guards against path traversal — only files under BackupDir are accepted.
func Path(id string) (string, error) {
	if id == "" || strings.ContainsAny(id, "/\\") || strings.Contains(id, "..") {
		return "", fmt.Errorf("invalid backup id")
	}
	dir, err := BackupDir()
	if err != nil {
		return "", err
	}
	candidate := filepath.Join(dir, id+filenameExt)
	if _, err := os.Stat(candidate); err != nil {
		return "", fmt.Errorf("backup %s not found", id)
	}
	return candidate, nil
}

// Import accepts a reader for a redock-created tar.gz backup, validates it
// (gzip + tar + manifest.json with non-empty ID), and stores it in BackupDir
// under its manifest ID. Returns the resulting Info or an error if the
// archive is malformed or its ID collides with an existing backup.
func Import(src io.Reader) (*Info, error) {
	dir, err := BackupDir()
	if err != nil {
		return nil, err
	}
	tmp, err := os.CreateTemp(dir, "import_*"+filenameExt)
	if err != nil {
		return nil, fmt.Errorf("create tmp file: %w", err)
	}
	tmpPath := tmp.Name()
	commited := false
	defer func() {
		if !commited {
			os.Remove(tmpPath)
		}
	}()
	if _, err := io.Copy(tmp, src); err != nil {
		tmp.Close()
		return nil, fmt.Errorf("write upload: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return nil, err
	}

	manifest, err := readManifest(tmpPath)
	if err != nil {
		return nil, fmt.Errorf("not a valid redock backup: %w", err)
	}
	if manifest.ID == "" {
		return nil, fmt.Errorf("uploaded archive has empty manifest ID")
	}
	if strings.ContainsAny(manifest.ID, "/\\") || strings.Contains(manifest.ID, "..") {
		return nil, fmt.Errorf("manifest ID contains illegal characters")
	}

	finalPath := filepath.Join(dir, manifest.ID+filenameExt)
	if _, err := os.Stat(finalPath); err == nil {
		return nil, fmt.Errorf("backup with ID %s already exists", manifest.ID)
	}
	if err := os.Rename(tmpPath, finalPath); err != nil {
		return nil, fmt.Errorf("finalize import: %w", err)
	}
	commited = true

	st, err := os.Stat(finalPath)
	if err != nil {
		return nil, err
	}

	pruneToConfiguredLimit()

	return &Info{
		ID:            manifest.ID,
		Filename:      filepath.Base(finalPath),
		CreatedAt:     manifest.CreatedAt,
		SizeBytes:     st.Size(),
		UncompressedB: manifest.UncompressedB,
		FileCount:     manifest.FileCount,
		SkippedCount:  len(manifest.SkippedFiles),
		RedockVersion: manifest.RedockVersion,
		TriggerReason: manifest.TriggerReason,
	}, nil
}

// Delete removes a backup by ID.
func Delete(id string) error {
	path, err := Path(id)
	if err != nil {
		return err
	}
	return os.Remove(path)
}

// Restore extracts a backup over $workDir/data/. Before extraction, the
// current data/ is itself snapshotted as "<id>_pre-restore" so the operation
// is reversible. The data dir is wiped and rewritten from the archive.
//
// Restore is atomic only at the directory-rename level: we extract into a
// temp sibling dir, then swap. If anything goes wrong before the swap, the
// existing data/ is untouched.
func Restore(id, workDir, currentVersion string) error {
	src, err := Path(id)
	if err != nil {
		return err
	}

	dataDir := SourceDataDir(workDir)
	if _, err := os.Stat(dataDir); err == nil {
		// Best-effort safety snapshot. If this fails we abort — better to
		// refuse the restore than risk losing the current state.
		if _, err := Create(workDir, currentVersion, "pre-restore"); err != nil {
			return fmt.Errorf("safety snapshot before restore failed: %w", err)
		}
	}

	stagingDir, err := os.MkdirTemp(filepath.Dir(dataDir), ".redock_restore_*")
	if err != nil {
		return fmt.Errorf("create staging dir: %w", err)
	}
	cleanup := stagingDir
	defer func() {
		if cleanup != "" {
			os.RemoveAll(cleanup)
		}
	}()

	if err := extractDataInto(src, stagingDir); err != nil {
		return fmt.Errorf("extract: %w", err)
	}

	// Swap: rename current dataDir aside, move staging into place, then drop the old.
	backupSidePath := dataDir + ".prev"
	_ = os.RemoveAll(backupSidePath)
	if _, err := os.Stat(dataDir); err == nil {
		if err := os.Rename(dataDir, backupSidePath); err != nil {
			return fmt.Errorf("move existing data dir aside: %w", err)
		}
	}
	if err := os.Rename(stagingDir, dataDir); err != nil {
		// Try to recover by putting the original back.
		_ = os.Rename(backupSidePath, dataDir)
		return fmt.Errorf("swap restored data dir: %w", err)
	}
	cleanup = "" // swap succeeded; staging now lives at dataDir
	_ = os.RemoveAll(backupSidePath)
	return nil
}

func extractDataInto(archivePath, dest string) error {
	f, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)

	if err := os.MkdirAll(dest, 0o755); err != nil {
		return err
	}

	const dataPrefix = "data/"
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		// Skip the manifest and anything outside data/.
		if hdr.Name == manifestFileName {
			continue
		}
		if !strings.HasPrefix(hdr.Name, dataPrefix) {
			continue
		}
		rel := strings.TrimPrefix(hdr.Name, dataPrefix)
		// Defense-in-depth: reject path traversal even though we wrote the archive ourselves.
		if rel == "" || strings.Contains(rel, "..") {
			continue
		}
		target := filepath.Join(dest, filepath.FromSlash(rel))
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, os.FileMode(hdr.Mode)&0o777); err != nil {
				return err
			}
		case tar.TypeReg, tar.TypeRegA:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(hdr.Mode)&0o777)
			if err != nil {
				return err
			}
			if _, err := io.Copy(out, tr); err != nil {
				out.Close()
				return err
			}
			out.Close()
		}
	}
	return nil
}
