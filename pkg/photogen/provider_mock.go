package photogen

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

// mockProvider serves a canned listing and reads bytes off the local disk. Its only job is
// to make the sync plumbing testable without a real upstream, and to keep SyncProvider
// honest by giving it a second implementation.
//
// Fetch opens a real file rather than returning a stub, so the whole transfer path runs:
// temp file, copy, chmod, rename, the skip-if-present check and the mtime that check
// depends on.
type mockProvider struct {
	cfg     *MockSyncConfig
	fixture mockFixture
}

// mockFixture is the JSON listing the mock provider is configured with. Its shape is the
// provider interface's types, written out: an album and its assets.
type mockFixture struct {
	Album struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	} `json:"album"`
	Assets []mockAsset `json:"assets"`
}

type mockAsset struct {
	ID        string   `json:"id"`
	FileName  string   `json:"file_name"`
	Caption   string   `json:"caption"`
	Size      int64    `json:"size"`
	Checksum  string   `json:"checksum"`
	UpdatedAt string   `json:"updated_at"` // RFC3339, or "" for unknown
	Warnings  []string `json:"warnings"`
	// EditedFile, when set, marks the asset edited and is the file in media_dir that Fetch
	// serves in place of file_name, the way Immich serves an edited rendition. While that
	// file does not exist, Fetch serves file_name instead, as Immich does before it has
	// rendered an edit.
	EditedFile string `json:"edited_file"`
	DateTaken  string `json:"date_taken"` // RFC3339, or "" for unknown
}

// errMockFail is what the fail: switch produces. The switch exists mainly for the prune
// rule: "do not prune on a provider failure" is the behavior most worth a test, and testing
// it needs a provider that can fail on demand.
var errMockFail = errors.New("mock provider failure (sync.mock.fail)")

// newMockProvider loads the listing fixture. The mock: block is optional in the struct but
// required by validation, so a nil here means the config was built by hand.
func newMockProvider(cfg *MockSyncConfig) (SyncProvider, error) {
	if cfg == nil {
		return nil, errors.New("provider mock requires a sync.mock block")
	}
	fixture, err := loadJSON[mockFixture](cfg.AssetsPath)
	if err != nil {
		return nil, fmt.Errorf("sync.mock.assets: %w", err)
	}
	if _, err := os.Stat(cfg.MediaDir); err != nil {
		return nil, fmt.Errorf("sync.mock.media_dir: %q does not exist", cfg.MediaDir)
	}
	return &mockProvider{cfg: cfg, fixture: fixture}, nil
}

func (p *mockProvider) Album(_ context.Context, _ string) (SyncAlbum, error) {
	return SyncAlbum{Name: p.fixture.Album.Name, Description: p.fixture.Album.Description}, nil
}

func (p *mockProvider) Assets(_ context.Context, _ string) ([]SyncAsset, error) {
	if p.cfg.Fail == "list" {
		return nil, errMockFail
	}
	assets := make([]SyncAsset, 0, len(p.fixture.Assets))
	for _, a := range p.fixture.Assets {
		updated, err := parseMockTime(a.ID, "updated_at", a.UpdatedAt)
		if err != nil {
			return nil, err
		}
		taken, err := parseMockTime(a.ID, "date_taken", a.DateTaken)
		if err != nil {
			return nil, err
		}
		assets = append(assets, SyncAsset{
			ID:        a.ID,
			FileName:  a.FileName,
			Caption:   a.Caption,
			Size:      a.Size,
			Checksum:  a.Checksum,
			UpdatedAt: updated,
			Warnings:  a.Warnings,
			Edited:    a.EditedFile != "",
			DateTaken: taken,
		})
	}
	return assets, nil
}

// parseMockTime reads an optional RFC3339 fixture field; "" is the zero time.
func parseMockTime(id, field, value string) (time.Time, error) {
	if value == "" {
		return time.Time{}, nil
	}
	t, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Time{}, fmt.Errorf("sync.mock.assets: asset %s: bad %s %q: %w", id, field, value, err)
	}
	return t, nil
}

func (p *mockProvider) Fetch(_ context.Context, a SyncAsset) (io.ReadCloser, error) {
	if p.cfg.Fail == "fetch" {
		return nil, errMockFail
	}
	if a.Edited {
		f, err := os.Open(filepath.Join(p.cfg.MediaDir, filepath.Base(p.editedFile(a.ID))))
		if !errors.Is(err, fs.ErrNotExist) {
			return f, err
		}
	}
	return os.Open(filepath.Join(p.cfg.MediaDir, filepath.Base(a.FileName)))
}

// editedFile returns the file an edited asset is served from. SyncAsset carries only the
// fact of the edit, so the fixture is looked up again by ID.
func (p *mockProvider) editedFile(id string) string {
	for _, a := range p.fixture.Assets {
		if a.ID == id {
			return a.EditedFile
		}
	}
	return ""
}
