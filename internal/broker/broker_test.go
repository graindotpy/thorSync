package broker

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/apgul/thorsync/internal/archive"
	"github.com/apgul/thorsync/internal/config"
	"github.com/apgul/thorsync/internal/events"
	"github.com/apgul/thorsync/internal/model"
	"github.com/apgul/thorsync/internal/store"
)

func TestPropagationAndEchoDoNotCreateSecondRevision(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	cfg := config.Config{DataDir: filepath.Join(root, "data"), ArchiveDir: filepath.Join(root, "archive"), ThorDir: filepath.Join(root, "thor"), WindowsDir: filepath.Join(root, "windows"), SyncthingThorFolderID: "thor-folder", SyncthingWindowsFolderID: "windows-folder"}
	for _, dir := range []string{cfg.ThorDir, cfg.WindowsDir} {
		if err := os.MkdirAll(dir, 0o750); err != nil {
			t.Fatal(err)
		}
	}
	db, err := store.Open(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.SetSetting(ctx, "propagation_enabled", "true"); err != nil {
		t.Fatal(err)
	}
	game, _ := db.CreateGame(ctx, "Golden Sun", model.PlatformGBA, "CAFEBABE", "")
	_, _ = db.UpsertBinding(ctx, model.SaveBinding{GameID: game.ID, EndpointID: "thor", ProfileID: "thor-mgba", RelativePath: "Golden Sun.srm"})
	_, _ = db.UpsertBinding(ctx, model.SaveBinding{GameID: game.ID, EndpointID: "windows", ProfileID: "windows-vbam", RelativePath: "Golden Sun.sav"})
	content := bytes.Repeat([]byte{0x42}, 64*1024)
	if err := os.WriteFile(filepath.Join(cfg.ThorDir, "Golden Sun.srm"), content, 0o640); err != nil {
		t.Fatal(err)
	}
	archiveStore := archive.New(cfg.ArchiveDir, 5<<30, 0)
	service := New(db, archiveStore, events.New())
	result, err := service.Capture(ctx, CaptureInput{EndpointID: "thor", RelativePath: "Golden Sun.srm", Provenance: model.ProvenanceConfirmed})
	if err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(cfg.WindowsDir, "Golden Sun.sav")
	if got, err := os.ReadFile(target); err != nil || !bytes.Equal(got, content) {
		t.Fatalf("target mismatch: err=%v", err)
	}
	echo, err := service.Capture(ctx, CaptureInput{EndpointID: "windows", RelativePath: "Golden Sun.sav", Provenance: model.ProvenanceConfirmed})
	if err != nil {
		t.Fatal(err)
	}
	if !echo.Echo || echo.RevisionID != result.RevisionID {
		t.Fatalf("propagated file was not recognized as echo: %#v", echo)
	}
	windowsBinding, _ := db.FindBinding(ctx, "windows", "Golden Sun.sav")
	if windowsBinding.LastDeployedRevisionID != "" {
		t.Fatalf("local hub observation advanced remote baseline: %s", windowsBinding.LastDeployedRevisionID)
	}
	if count, err := db.CompleteEndpointDeliveries(ctx, "windows"); err != nil || count != 1 {
		t.Fatalf("remote completion count=%d err=%v", count, err)
	}
	windowsBinding, _ = db.FindBinding(ctx, "windows", "Golden Sun.sav")
	if windowsBinding.LastDeployedRevisionID != result.RevisionID {
		t.Fatalf("remote completion did not advance baseline: %#v", windowsBinding)
	}
	detail, _ := db.GetGame(ctx, game.ID)
	if len(detail.Revisions) != 1 {
		t.Fatalf("echo created %d revisions", len(detail.Revisions))
	}
}

func TestSafeJoinRejectsTraversal(t *testing.T) {
	for _, path := range []string{"../../escape.sav", "/absolute.sav", `C:\\absolute.sav`, `\\\\server\\share\\save.sav`} {
		if _, err := safeJoin(t.TempDir(), path); err == nil {
			t.Errorf("expected %q to be rejected", path)
		}
	}
}

func TestSyncthingConflictArtifactMapsToLiveBindingAndIsArchived(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	cfg := config.Config{DataDir: filepath.Join(root, "data"), ArchiveDir: filepath.Join(root, "archive"), ThorDir: filepath.Join(root, "thor"), WindowsDir: filepath.Join(root, "windows"), SyncthingThorFolderID: "thor-folder", SyncthingWindowsFolderID: "windows-folder"}
	for _, dir := range []string{cfg.ThorDir, cfg.WindowsDir} {
		if err := os.MkdirAll(dir, 0o750); err != nil {
			t.Fatal(err)
		}
	}
	db, err := store.Open(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	game, _ := db.CreateGame(ctx, "Advance Wars 2", model.PlatformGBA, "1122AABB", "")
	_, _ = db.UpsertBinding(ctx, model.SaveBinding{GameID: game.ID, EndpointID: "thor", ProfileID: "thor-mgba", RelativePath: "Advance Wars 2.srm"})
	_, _ = db.UpsertBinding(ctx, model.SaveBinding{GameID: game.ID, EndpointID: "windows", ProfileID: "windows-vbam", RelativePath: "Advance Wars 2.sav"})
	service := New(db, archive.New(cfg.ArchiveDir, 5<<30, 0), events.New())
	base := bytes.Repeat([]byte{0x10}, 64*1024)
	if err = os.WriteFile(filepath.Join(cfg.ThorDir, "Advance Wars 2.srm"), base, 0o640); err != nil {
		t.Fatal(err)
	}
	if _, err = service.Capture(ctx, CaptureInput{EndpointID: "thor", RelativePath: "Advance Wars 2.srm", Provenance: model.ProvenanceConfirmed}); err != nil {
		t.Fatal(err)
	}
	artifact := "Advance Wars 2.sync-conflict-20260920-120000-ABC1234.sav"
	branchBytes := bytes.Repeat([]byte{0x20}, 64*1024)
	artifactPath := filepath.Join(cfg.WindowsDir, artifact)
	if err = os.WriteFile(artifactPath, branchBytes, 0o640); err != nil {
		t.Fatal(err)
	}
	result, err := service.Capture(ctx, CaptureInput{EndpointID: "windows", RelativePath: artifact, Provenance: model.ProvenanceConfirmed})
	if err != nil {
		t.Fatal(err)
	}
	if result.State != "branch" {
		t.Fatalf("state=%s, want branch", result.State)
	}
	if _, err = os.Stat(artifactPath); !os.IsNotExist(err) {
		t.Fatalf("conflict artifact was not removed after archival: %v", err)
	}
	detail, err := db.GetGame(ctx, game.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(detail.Revisions) != 2 || detail.ConflictCount != 1 || detail.Observations[0].RelativePath != artifact {
		t.Fatalf("conflict was not preserved correctly: revisions=%d conflicts=%d observations=%#v", len(detail.Revisions), detail.ConflictCount, detail.Observations)
	}
	if _, err = os.Stat(archive.New(cfg.ArchiveDir, 5<<30, 0).Path(detail.Revisions[0].BlobHash)); err != nil {
		t.Fatalf("branch bytes missing from archive: %v", err)
	}
}

func TestZeroByteSaveLeavesPersistentQuarantineRecord(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	cfg := config.Config{DataDir: filepath.Join(root, "data"), ArchiveDir: filepath.Join(root, "archive"), ThorDir: filepath.Join(root, "thor"), WindowsDir: filepath.Join(root, "windows"), SyncthingThorFolderID: "thor-folder", SyncthingWindowsFolderID: "windows-folder"}
	if err := os.MkdirAll(cfg.ThorDir, 0o750); err != nil {
		t.Fatal(err)
	}
	db, err := store.Open(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err = os.WriteFile(filepath.Join(cfg.ThorDir, "empty.srm"), nil, 0o640); err != nil {
		t.Fatal(err)
	}
	service := New(db, archive.New(cfg.ArchiveDir, 5<<30, 0), events.New())
	if _, err = service.Capture(ctx, CaptureInput{EndpointID: "thor", RelativePath: "empty.srm"}); err == nil {
		t.Fatal("expected zero-byte capture to fail safely")
	}
	items, err := db.ListUnassigned(ctx)
	if err != nil || len(items) != 1 || items[0].Size != 0 || !strings.Contains(items[0].Detail, "zero-byte") {
		t.Fatalf("quarantine record missing: %#v err=%v", items, err)
	}
}
