package rtc_server

import (
	"embed"
	"io/fs"
	"log"
	"os"
	"path/filepath"
)

// galeneStatic is Galène's web client, copied from the pinned module version
// (see galeneVersion) so it ships inside the single binary.
//
//go:embed galene_static
var galeneStatic embed.FS

// galeneVersion is the pinned github.com/jech/galene version; it must match
// go.mod and the files in galene_static/. It also stamps the extracted copy.
const galeneVersion = "v0.0.0-20261023205516-e2bc84f26d4f"

// extractStatic writes the embedded web client to dir. Galène serves static
// files from a real directory (os.OpenRoot), so they are unpacked once per
// version; an up-to-date copy is left alone.
func extractStatic(dir string) error {
	stamp := filepath.Join(dir, ".redock-version")
	if b, err := os.ReadFile(stamp); err == nil && string(b) == galeneVersion {
		return nil
	}
	if err := os.RemoveAll(dir); err != nil {
		return err
	}
	sub, err := fs.Sub(galeneStatic, "galene_static")
	if err != nil {
		return err
	}
	err = fs.WalkDir(sub, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		dst := filepath.Join(dir, filepath.FromSlash(p))
		if d.IsDir() {
			return os.MkdirAll(dst, 0o755)
		}
		data, err := fs.ReadFile(sub, p)
		if err != nil {
			return err
		}
		return os.WriteFile(dst, data, 0o644)
	})
	if err != nil {
		return err
	}
	return os.WriteFile(stamp, []byte(galeneVersion), 0o644)
}

// joinUI is Redock's own room-join page (a Vue build, web/dist/rtc-join),
// handed over by the main package which embeds the web bundle. It is served
// by Galène under /redock/ so it runs same-origin with the SFU.
var joinUI fs.FS

// SetJoinUI registers the join page bundle (call before Init).
func SetJoinUI(f fs.FS) { joinUI = f }

// extractJoinUI writes the join page into Galène's static root. It is small
// and rewritten on every start, so it always matches the running binary.
func extractJoinUI(dir string) error {
	if err := os.RemoveAll(dir); err != nil {
		return err
	}
	if joinUI == nil {
		return nil // e.g. tests
	}
	if _, err := fs.Stat(joinUI, "join.html"); err != nil {
		// A binary built without the web bundle: Galène's own client still works.
		log.Printf("rtc_server: join page not bundled (%v); use Galène's client", err)
		return nil
	}
	return fs.WalkDir(joinUI, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		dst := filepath.Join(dir, filepath.FromSlash(p))
		if d.IsDir() {
			return os.MkdirAll(dst, 0o755)
		}
		data, err := fs.ReadFile(joinUI, p)
		if err != nil {
			return err
		}
		return os.WriteFile(dst, data, 0o644)
	})
}
