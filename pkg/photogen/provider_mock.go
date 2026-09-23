package photogen

import (
	"context"
	"errors"
	"fmt"
	"io"
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
	IsVideo   bool     `json:"is_video"`
	Warnings  []string `json:"warnings"`
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

func (p *mockProvider) Name() string { return mockProviderName }

func (p *mockProvider) Album(_ context.Context, _ string) (SyncAlbum, error) {
	return SyncAlbum{Name: p.fixture.Album.Name, Description: p.fixture.Album.Description}, nil
}

func (p *mockProvider) Assets(_ context.Context, _ string) ([]SyncAsset, error) {
	if p.cfg.Fail == "list" {
		return nil, errMockFail
	}
	assets := make([]SyncAsset, 0, len(p.fixture.Assets))
	for _, a := range p.fixture.Assets {
		var updated time.Time
		if a.UpdatedAt != "" {
			t, err := time.Parse(time.RFC3339, a.UpdatedAt)
			if err != nil {
				return nil, fmt.Errorf("sync.mock.assets: asset %s: bad updated_at %q: %w", a.ID, a.UpdatedAt, err)
			}
			updated = t
		}
		assets = append(assets, SyncAsset{
			ID:        a.ID,
			FileName:  a.FileName,
			Caption:   a.Caption,
			Size:      a.Size,
			Checksum:  a.Checksum,
			UpdatedAt: updated,
			IsVideo:   a.IsVideo,
			Warnings:  a.Warnings,
		})
	}
	return assets, nil
}

func (p *mockProvider) Fetch(_ context.Context, a SyncAsset) (io.ReadCloser, error) {
	if p.cfg.Fail == "fetch" {
		return nil, errMockFail
	}
	return os.Open(filepath.Join(p.cfg.MediaDir, filepath.Base(a.FileName)))
}
