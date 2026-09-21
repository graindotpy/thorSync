package broker

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
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

func TestMGBAWrappedRoundTripPreservesRTCAndSkipsEquivalentThorWrite(t *testing.T) {
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
	if err = db.SetSetting(ctx, "propagation_enabled", "true"); err != nil {
		t.Fatal(err)
	}
	game, err := db.CreateGame(ctx, "Pokemon Lazarus", model.PlatformGBA, "", "")
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.UpsertBinding(ctx, model.SaveBinding{GameID: game.ID, EndpointID: "thor", ProfileID: "thor-mgba", RelativePath: "mGBA/Pokemon Lazarus.srm"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.UpsertBinding(ctx, model.SaveBinding{GameID: game.ID, EndpointID: "windows", ProfileID: "windows-mgba", RelativePath: "Pokemon Lazarus.sav"})
	if err != nil {
		t.Fatal(err)
	}

	archiveStore := archive.New(cfg.ArchiveDir, 5<<30, 0)
	service := New(db, archiveStore, events.New())
	windowsPath := filepath.Join(cfg.WindowsDir, "Pokemon Lazarus.sav")
	thorPath := filepath.Join(cfg.ThorDir, "mGBA", "Pokemon Lazarus.srm")
	battery1 := bytes.Repeat([]byte{0x31}, 128*1024)
	rtc1 := []byte{0x00, 0x01, 0x7f, 0x80, 0xfe, 0xff, 0x10, 0x20, 0x30, 0x40, 0x50, 0x60, 0x70, 0x90, 0xa0, 0xb0}
	wrapped1 := append(append([]byte(nil), battery1...), rtc1...)
	if err = os.WriteFile(windowsPath, wrapped1, 0o640); err != nil {
		t.Fatal(err)
	}
	first, err := service.Capture(ctx, CaptureInput{EndpointID: "windows", RelativePath: "Pokemon Lazarus.sav", Provenance: model.ProvenanceConfirmed})
	if err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(thorPath); err != nil || !bytes.Equal(got, battery1) {
		t.Fatalf("Thor did not receive raw battery RAM: len=%d err=%v", len(got), err)
	}
	firstRevision, err := db.Revision(ctx, first.RevisionID)
	if err != nil {
		t.Fatal(err)
	}
	batteryDigest := sha256.Sum256(battery1)
	physicalDigest := sha256.Sum256(wrapped1)
	if firstRevision.BlobHash != hex.EncodeToString(batteryDigest[:]) || firstRevision.Size != int64(len(battery1)) || !firstRevision.HasRTC {
		t.Fatalf("revision did not retain canonical battery identity: %#v", firstRevision)
	}
	if _, err = os.Stat(archiveStore.Path(hex.EncodeToString(physicalDigest[:]))); err != nil {
		t.Fatalf("original wrapped occurrence was not archived: %v", err)
	}
	if _, err = service.Capture(ctx, CaptureInput{EndpointID: "thor", RelativePath: "mGBA/Pokemon Lazarus.srm", Provenance: model.ProvenanceConfirmed, ConfirmDelivery: true}); err != nil {
		t.Fatal(err)
	}

	// A raw edit from RetroArch inherits RTC only from this binding's exact
	// last-deployed revision, then reconstructs a valid standalone mGBA file.
	battery2 := append([]byte(nil), battery1...)
	battery2[42] ^= 0xff
	if err = os.WriteFile(thorPath, battery2, 0o640); err != nil {
		t.Fatal(err)
	}
	second, err := service.Capture(ctx, CaptureInput{EndpointID: "thor", RelativePath: "mGBA/Pokemon Lazarus.srm", Provenance: model.ProvenanceConfirmed})
	if err != nil {
		t.Fatal(err)
	}
	if second.State != "head" {
		t.Fatalf("Thor edit state=%s, want head", second.State)
	}
	if got, err := os.ReadFile(windowsPath); err != nil || !bytes.Equal(got, append(append([]byte(nil), battery2...), rtc1...)) {
		t.Fatalf("mGBA reconstruction lost battery or RTC bytes: len=%d err=%v", len(got), err)
	}
	if _, err = service.Capture(ctx, CaptureInput{EndpointID: "windows", RelativePath: "Pokemon Lazarus.sav", Provenance: model.ProvenanceConfirmed, ConfirmDelivery: true}); err != nil {
		t.Fatal(err)
	}

	// An RTC-only mGBA edit is a new logical revision, but RetroArch's physical
	// representation is already identical and must not be rewritten.
	thorBefore, err := os.Stat(thorPath)
	if err != nil {
		t.Fatal(err)
	}
	rtc2 := append([]byte(nil), rtc1...)
	rtc2[15] ^= 0xff
	wrapped2 := append(append([]byte(nil), battery2...), rtc2...)
	if err = os.WriteFile(windowsPath, wrapped2, 0o640); err != nil {
		t.Fatal(err)
	}
	third, err := service.Capture(ctx, CaptureInput{EndpointID: "windows", RelativePath: "Pokemon Lazarus.sav", Provenance: model.ProvenanceConfirmed})
	if err != nil {
		t.Fatal(err)
	}
	if third.RevisionID == second.RevisionID || third.ContentHash == second.ContentHash {
		t.Fatal("RTC-only edit did not create a distinct logical revision")
	}
	thorAfter, err := os.Stat(thorPath)
	if err != nil {
		t.Fatal(err)
	}
	if !thorAfter.ModTime().Equal(thorBefore.ModTime()) {
		t.Fatalf("representation-equivalent Thor save was unnecessarily rewritten: before=%s after=%s", thorBefore.ModTime(), thorAfter.ModTime())
	}
	thorBinding, err := db.FindBinding(ctx, "thor", "mGBA/Pokemon Lazarus.srm")
	if err != nil || thorBinding.LastDeployedRevisionID == third.RevisionID {
		t.Fatalf("Thor baseline advanced before remote confirmation: %#v err=%v", thorBinding, err)
	}
	if delivered, completeErr := db.CompleteEndpointDeliveries(ctx, "thor"); completeErr != nil || delivered != 1 {
		t.Fatalf("Thor representation-equivalent delivery was not confirmed: delivered=%d err=%v", delivered, completeErr)
	}
	thorBinding, err = db.FindBinding(ctx, "thor", "mGBA/Pokemon Lazarus.srm")
	if err != nil || thorBinding.LastDeployedRevisionID != third.RevisionID {
		t.Fatalf("Thor logical baseline was not advanced after confirmation: %#v err=%v", thorBinding, err)
	}

	// A broker restart and repeat observation must not create another revision.
	service.Close()
	service = New(db, archiveStore, events.New())
	repeat, err := service.Capture(ctx, CaptureInput{EndpointID: "windows", RelativePath: "Pokemon Lazarus.sav", Provenance: model.ProvenanceConfirmed})
	if err != nil {
		t.Fatal(err)
	}
	if !repeat.Echo || repeat.RevisionID != third.RevisionID {
		t.Fatalf("repeat capture after restart was not deduplicated: %#v", repeat)
	}
	detail, err := db.GetGame(ctx, game.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(detail.Revisions) != 3 {
		t.Fatalf("round trip created %d revisions, want 3", len(detail.Revisions))
	}
}

func TestMatchingHubFileWaitsForRemoteDeliveryConfirmation(t *testing.T) {
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
	game, _ := db.CreateGame(ctx, "Metroid Fusion", model.PlatformGBA, "", "")
	thor, _ := db.UpsertBinding(ctx, model.SaveBinding{GameID: game.ID, EndpointID: "thor", ProfileID: "thor-mgba", RelativePath: "Metroid Fusion.srm"})
	_, _ = db.UpsertBinding(ctx, model.SaveBinding{GameID: game.ID, EndpointID: "windows", ProfileID: "windows-mgba", RelativePath: "Metroid Fusion.sav"})
	archiveStore := archive.New(cfg.ArchiveDir, 5<<30, 0)
	battery := bytes.Repeat([]byte{0x6a}, 64*1024)
	blob, err := archiveStore.PutBytes(battery)
	if err != nil {
		t.Fatal(err)
	}
	head, err := db.RecordIngest(ctx, store.IngestParams{Binding: thor, ObservedBlob: blob, BatteryBlob: blob})
	if err != nil {
		t.Fatal(err)
	}
	// This models a restart after the target rename but before the journal was
	// marked written: hub bytes match, but the remote endpoint is unconfirmed.
	if err = os.WriteFile(filepath.Join(cfg.WindowsDir, "Metroid Fusion.sav"), battery, 0o640); err != nil {
		t.Fatal(err)
	}
	service := New(db, archiveStore, events.New())
	if err = service.PropagateRevision(ctx, game.ID, head.RevisionID, "thor"); err != nil {
		t.Fatal(err)
	}
	windows, err := db.FindBinding(ctx, "windows", "Metroid Fusion.sav")
	if err != nil {
		t.Fatal(err)
	}
	if windows.LastDeployedRevisionID != "" {
		t.Fatalf("unconfirmed target advanced baseline to %s", windows.LastDeployedRevisionID)
	}
	detail, err := db.GetGame(ctx, game.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(detail.Operations) != 1 || detail.Operations[0].State != "written" {
		t.Fatalf("matching hub target should await remote confirmation: %#v", detail.Operations)
	}
}

func TestRestoreWithSameBytesDoesNotAdvanceOfflineBinding(t *testing.T) {
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
	game, _ := db.CreateGame(ctx, "Golden Sun", model.PlatformGBA, "", "")
	thor, _ := db.UpsertBinding(ctx, model.SaveBinding{GameID: game.ID, EndpointID: "thor", ProfileID: "thor-mgba", RelativePath: "Golden Sun.srm"})
	windows, _ := db.UpsertBinding(ctx, model.SaveBinding{GameID: game.ID, EndpointID: "windows", ProfileID: "windows-mgba", RelativePath: "Golden Sun.sav"})
	archiveStore := archive.New(cfg.ArchiveDir, 5<<30, 0)
	batteryA := bytes.Repeat([]byte{0x1a}, 64*1024)
	batteryB := bytes.Repeat([]byte{0x2b}, 64*1024)
	blobA, _ := archiveStore.PutBytes(batteryA)
	blobB, _ := archiveStore.PutBytes(batteryB)
	first, err := db.RecordIngest(ctx, store.IngestParams{Binding: thor, ObservedBlob: blobA, BatteryBlob: blobA})
	if err != nil {
		t.Fatal(err)
	}
	if err = db.MarkBindingDeployed(ctx, windows.ID, first.RevisionID); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(cfg.WindowsDir, "Golden Sun.sav"), batteryA, 0o640); err != nil {
		t.Fatal(err)
	}
	thor, _ = db.FindBinding(ctx, "thor", "Golden Sun.srm")
	second, err := db.RecordIngest(ctx, store.IngestParams{Binding: thor, ObservedBlob: blobB, BatteryBlob: blobB})
	if err != nil {
		t.Fatal(err)
	}
	restored, err := db.CreateHeadFromExisting(ctx, store.HeadMutation{GameID: game.ID, SourceRevisionID: first.RevisionID, ExpectedHeadID: second.RevisionID, IdempotencyKey: "restore-a", Actor: "test", Kind: "restore"})
	if err != nil {
		t.Fatal(err)
	}
	service := New(db, archiveStore, events.New())
	if err = service.PropagateRevision(ctx, game.ID, restored.ID, ""); err != nil {
		t.Fatal(err)
	}
	windows, _ = db.FindBinding(ctx, "windows", "Golden Sun.sav")
	if windows.LastDeployedRevisionID != first.RevisionID {
		t.Fatalf("offline restore advanced baseline from %s to %s", first.RevisionID, windows.LastDeployedRevisionID)
	}
	detail, err := db.GetGame(ctx, game.ID)
	if err != nil {
		t.Fatal(err)
	}
	var windowsOperationState string
	for _, operation := range detail.Operations {
		if operation.TargetEndpointID == "windows" && operation.RevisionID == restored.ID {
			windowsOperationState = operation.State
		}
	}
	if windowsOperationState != "written" {
		t.Fatalf("offline restore operation state=%q, want written", windowsOperationState)
	}
}

func TestRematerializeBindingSwitchesProfilesWithoutLosingRTC(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	cfg := config.Config{DataDir: filepath.Join(root, "data"), ArchiveDir: filepath.Join(root, "archive"), ThorDir: filepath.Join(root, "thor"), WindowsDir: filepath.Join(root, "windows"), SyncthingThorFolderID: "thor-folder", SyncthingWindowsFolderID: "windows-folder"}
	if err := os.MkdirAll(cfg.WindowsDir, 0o750); err != nil {
		t.Fatal(err)
	}
	db, err := store.Open(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	game, _ := db.CreateGame(ctx, "Pokemon Emerald", model.PlatformGBA, "", "")
	windows, _ := db.UpsertBinding(ctx, model.SaveBinding{GameID: game.ID, EndpointID: "windows", ProfileID: "windows-mgba", RelativePath: "Pokemon Emerald.sav"})
	archiveStore := archive.New(cfg.ArchiveDir, 5<<30, 0)
	battery := bytes.Repeat([]byte{0x72}, 128*1024)
	rtc := bytes.Repeat([]byte{0x83}, 16)
	wrapped := append(append([]byte(nil), battery...), rtc...)
	physical, _ := archiveStore.PutBytes(wrapped)
	batteryBlob, _ := archiveStore.PutBytes(battery)
	rtcBlob, _ := archiveStore.PutBytes(rtc)
	head, err := db.RecordIngest(ctx, store.IngestParams{Binding: windows, ObservedBlob: physical, BatteryBlob: batteryBlob, RTCBlob: &rtcBlob})
	if err != nil {
		t.Fatal(err)
	}
	livePath := filepath.Join(cfg.WindowsDir, "Pokemon Emerald.sav")
	if err = os.WriteFile(livePath, wrapped, 0o640); err != nil {
		t.Fatal(err)
	}
	service := New(db, archiveStore, events.New())

	if _, err = db.ConfigureWindowsGBAProfile(ctx, "windows-vbam", true); err != nil {
		t.Fatal(err)
	}
	windows, _ = db.FindBinding(ctx, "windows", "Pokemon Emerald.sav")
	if err = service.RematerializeBinding(ctx, windows); err != nil {
		t.Fatal(err)
	}
	if got, readErr := os.ReadFile(livePath); readErr != nil || !bytes.Equal(got, battery) {
		t.Fatalf("VBA-M rematerialization was not raw battery RAM: len=%d err=%v", len(got), readErr)
	}
	payload, err := db.RevisionPayload(ctx, head.RevisionID)
	if err != nil || payload.RTCBlobHash != rtcBlob.Hash {
		t.Fatalf("profile switch lost archived RTC payload: %#v err=%v", payload, err)
	}

	if _, err = db.ConfigureWindowsGBAProfile(ctx, "windows-mgba", true); err != nil {
		t.Fatal(err)
	}
	windows, _ = db.FindBinding(ctx, "windows", "Pokemon Emerald.sav")
	if err = service.RematerializeBinding(ctx, windows); err != nil {
		t.Fatal(err)
	}
	if got, readErr := os.ReadFile(livePath); readErr != nil || !bytes.Equal(got, wrapped) {
		t.Fatalf("mGBA rematerialization did not restore RTC wrapper: len=%d err=%v", len(got), readErr)
	}

	// Reusing the prior VBA-M operation identity must still rematerialize after
	// switching back, even though its journal row is already written.
	if _, err = db.ConfigureWindowsGBAProfile(ctx, "windows-vbam", true); err != nil {
		t.Fatal(err)
	}
	windows, _ = db.FindBinding(ctx, "windows", "Pokemon Emerald.sav")
	if err = service.RematerializeBinding(ctx, windows); err != nil {
		t.Fatal(err)
	}
	if got, readErr := os.ReadFile(livePath); readErr != nil || !bytes.Equal(got, battery) {
		t.Fatalf("reused VBA-M operation did not rematerialize: len=%d err=%v", len(got), readErr)
	}
}

func TestMGBAProfileChangeRetriesAndClearsWrappedQuarantine(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	cfg := config.Config{DataDir: filepath.Join(root, "data"), ArchiveDir: filepath.Join(root, "archive"), ThorDir: filepath.Join(root, "thor"), WindowsDir: filepath.Join(root, "windows"), SyncthingThorFolderID: "thor-folder", SyncthingWindowsFolderID: "windows-folder"}
	if err := os.MkdirAll(cfg.WindowsDir, 0o750); err != nil {
		t.Fatal(err)
	}
	db, err := store.Open(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	game, _ := db.CreateGame(ctx, "Pokemon Lazarus", model.PlatformGBA, "", "")
	_, _ = db.UpsertBinding(ctx, model.SaveBinding{GameID: game.ID, EndpointID: "windows", ProfileID: "windows-vbam", RelativePath: "Pokemon Lazarus.sav"})
	wrapped := append(bytes.Repeat([]byte{0x66}, 128*1024), bytes.Repeat([]byte{0x77}, 16)...)
	if err = os.WriteFile(filepath.Join(cfg.WindowsDir, "Pokemon Lazarus.sav"), wrapped, 0o640); err != nil {
		t.Fatal(err)
	}
	service := New(db, archive.New(cfg.ArchiveDir, 5<<30, 0), events.New())
	if _, err = service.Capture(ctx, CaptureInput{EndpointID: "windows", RelativePath: "Pokemon Lazarus.sav"}); err == nil {
		t.Fatal("VBA-M binding accepted wrapped mGBA save")
	}
	if items, err := db.ListUnassigned(ctx); err != nil || len(items) != 1 {
		t.Fatalf("expected one quarantine before profile change: %#v err=%v", items, err)
	}
	if _, err = db.ConfigureWindowsGBAProfile(ctx, "windows-mgba", true); err != nil {
		t.Fatal(err)
	}
	if _, err = service.Capture(ctx, CaptureInput{EndpointID: "windows", RelativePath: "Pokemon Lazarus.sav"}); err != nil {
		t.Fatal(err)
	}
	if items, err := db.ListUnassigned(ctx); err != nil || len(items) != 0 {
		t.Fatalf("successful mGBA recapture did not clear quarantine: %#v err=%v", items, err)
	}
}

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
	_, _ = db.UpsertBinding(ctx, model.SaveBinding{GameID: game.ID, EndpointID: "windows", ProfileID: "windows-mgba", RelativePath: "Advance Wars 2.sav"})
	service := New(db, archive.New(cfg.ArchiveDir, 5<<30, 0), events.New())
	base := bytes.Repeat([]byte{0x10}, 64*1024)
	if err = os.WriteFile(filepath.Join(cfg.ThorDir, "Advance Wars 2.srm"), base, 0o640); err != nil {
		t.Fatal(err)
	}
	if _, err = service.Capture(ctx, CaptureInput{EndpointID: "thor", RelativePath: "Advance Wars 2.srm", Provenance: model.ProvenanceConfirmed}); err != nil {
		t.Fatal(err)
	}
	artifact := "Advance Wars 2.sync-conflict-20260920-120000-ABC1234.sav"
	branchBattery := bytes.Repeat([]byte{0x20}, 64*1024)
	branchRTC := []byte{0x00, 0x11, 0x22, 0x33, 0x44, 0x55, 0x66, 0x77, 0x88, 0x99, 0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff}
	branchBytes := append(append([]byte(nil), branchBattery...), branchRTC...)
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
	payload, err := db.RevisionPayload(ctx, result.RevisionID)
	if err != nil {
		t.Fatal(err)
	}
	batteryDigest := sha256.Sum256(branchBattery)
	rtcDigest := sha256.Sum256(branchRTC)
	physicalDigest := sha256.Sum256(branchBytes)
	if payload.BatteryBlobHash != hex.EncodeToString(batteryDigest[:]) || payload.BatterySize != int64(len(branchBattery)) || payload.RTCBlobHash != hex.EncodeToString(rtcDigest[:]) || payload.RTCSize != int64(len(branchRTC)) {
		t.Fatalf("wrapped conflict payload was not preserved: %#v", payload)
	}
	archiveStore := archive.New(cfg.ArchiveDir, 5<<30, 0)
	if _, err = os.Stat(archiveStore.Path(hex.EncodeToString(physicalDigest[:]))); err != nil {
		t.Fatalf("original wrapped conflict occurrence missing from archive: %v", err)
	}
}

func TestSyncthingConflictArtifactMatchingHeadStillBecomesBranchAndIsRemoved(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	cfg := config.Config{DataDir: filepath.Join(root, "data"), ArchiveDir: filepath.Join(root, "archive"), ThorDir: filepath.Join(root, "thor"), WindowsDir: filepath.Join(root, "windows"), SyncthingThorFolderID: "thor-folder", SyncthingWindowsFolderID: "windows-folder"}
	if err := os.MkdirAll(cfg.WindowsDir, 0o750); err != nil {
		t.Fatal(err)
	}
	db, err := store.Open(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	game, _ := db.CreateGame(ctx, "Pokemon Pinball", model.PlatformGBA, "", "")
	_, _ = db.UpsertBinding(ctx, model.SaveBinding{GameID: game.ID, EndpointID: "windows", ProfileID: "windows-mgba", RelativePath: "Pokemon Pinball.sav"})
	service := New(db, archive.New(cfg.ArchiveDir, 5<<30, 0), events.New())
	wrapped := append(bytes.Repeat([]byte{0x3c}, 64*1024), bytes.Repeat([]byte{0x4d}, 16)...)
	livePath := filepath.Join(cfg.WindowsDir, "Pokemon Pinball.sav")
	if err = os.WriteFile(livePath, wrapped, 0o640); err != nil {
		t.Fatal(err)
	}
	if _, err = service.Capture(ctx, CaptureInput{EndpointID: "windows", RelativePath: "Pokemon Pinball.sav"}); err != nil {
		t.Fatal(err)
	}
	artifact := "Pokemon Pinball.sync-conflict-20260921-120000-ABC1234.sav"
	artifactPath := filepath.Join(cfg.WindowsDir, artifact)
	if err = os.WriteFile(artifactPath, wrapped, 0o640); err != nil {
		t.Fatal(err)
	}
	result, err := service.Capture(ctx, CaptureInput{EndpointID: "windows", RelativePath: artifact})
	if err != nil {
		t.Fatal(err)
	}
	if result.State != "branch" {
		t.Fatalf("same-content conflict state=%q, want branch", result.State)
	}
	if _, err = os.Stat(artifactPath); !os.IsNotExist(err) {
		t.Fatalf("same-content conflict artifact was not removed: %v", err)
	}
	detail, err := db.GetGame(ctx, game.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(detail.Revisions) != 2 || detail.ConflictCount != 1 {
		t.Fatalf("same-content conflict was not retained as a branch: revisions=%d conflicts=%d", len(detail.Revisions), detail.ConflictCount)
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
