package store

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/apgul/thorsync/internal/archive"
	"github.com/apgul/thorsync/internal/config"
	"github.com/apgul/thorsync/internal/model"
	_ "modernc.org/sqlite"
)

func TestPayloadContentHashKeepsLegacyBatteryIdentity(t *testing.T) {
	battery := hash('a')
	rtc := hash('b')
	if got := PayloadContentHash(battery, ""); got != battery {
		t.Fatalf("battery-only content hash=%q, want legacy hash %q", got, battery)
	}
	first := PayloadContentHash(battery, rtc)
	if first == battery || first == rtc || len(first) != 64 {
		t.Fatalf("component manifest hash=%q", first)
	}
	if got := PayloadContentHash(battery, rtc); got != first {
		t.Fatalf("manifest hash is not stable: %q != %q", got, first)
	}
	if got := PayloadContentHash(battery, hash('c')); got == first {
		t.Fatal("RTC-only change did not change logical content identity")
	}
}

func TestRawThorEditInheritsRTCFromExactDeployedBaseline(t *testing.T) {
	ctx := context.Background()
	db, _ := newTestStore(t)
	game, _ := db.CreateGame(ctx, "Pokemon Emerald", model.PlatformGBA, "A0B0C0D0", "")
	windows, _ := db.UpsertBinding(ctx, model.SaveBinding{GameID: game.ID, EndpointID: "windows", ProfileID: "windows-mgba", RelativePath: "Pokemon Emerald.sav"})
	thor, _ := db.UpsertBinding(ctx, model.SaveBinding{GameID: game.ID, EndpointID: "thor", ProfileID: "thor-mgba", RelativePath: "mGBA/Pokemon Emerald.srm"})

	baseBattery := archive.Blob{Hash: hash('1'), Size: 128 * 1024}
	baseRTC := archive.Blob{Hash: hash('2'), Size: 16}
	base, err := db.RecordIngest(ctx, IngestParams{
		Binding: windows, ObservedBlob: archive.Blob{Hash: hash('3'), Size: 128*1024 + 16},
		BatteryBlob: baseBattery, RTCBlob: &baseRTC, ObservedAt: time.Now().UTC(), Provenance: model.ProvenanceConfirmed,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err = db.MarkBindingDeployed(ctx, thor.ID, base.RevisionID); err != nil {
		t.Fatal(err)
	}
	thor, _ = db.FindBinding(ctx, "thor", "mGBA/Pokemon Emerald.srm")

	updatedBattery := archive.Blob{Hash: hash('4'), Size: 128 * 1024}
	updated, err := db.RecordIngest(ctx, IngestParams{
		Binding: thor, ObservedBlob: updatedBattery, BatteryBlob: updatedBattery,
		ObservedAt: time.Now().UTC().Add(time.Second), Provenance: model.ProvenanceConfirmed,
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.State != "head" {
		t.Fatalf("raw edit state=%s, want head", updated.State)
	}
	payload, err := db.RevisionPayload(ctx, updated.RevisionID)
	if err != nil {
		t.Fatal(err)
	}
	if payload.BatteryBlobHash != updatedBattery.Hash || payload.RTCBlobHash != baseRTC.Hash || payload.RTCSize != 16 {
		t.Fatalf("inherited payload=%#v", payload)
	}
	if updated.ContentHash != PayloadContentHash(updatedBattery.Hash, baseRTC.Hash) {
		t.Fatalf("content hash=%s", updated.ContentHash)
	}
}

func TestRTCOnlyChangeCreatesImmutableRevision(t *testing.T) {
	ctx := context.Background()
	db, _ := newTestStore(t)
	game, _ := db.CreateGame(ctx, "Pokemon Ruby", model.PlatformGBA, "01020304", "")
	windows, _ := db.UpsertBinding(ctx, model.SaveBinding{GameID: game.ID, EndpointID: "windows", ProfileID: "windows-mgba", RelativePath: "Pokemon Ruby.sav"})
	battery := archive.Blob{Hash: hash('5'), Size: 128 * 1024}
	rtcA := archive.Blob{Hash: hash('6'), Size: 16}
	first, err := db.RecordIngest(ctx, IngestParams{Binding: windows, ObservedBlob: archive.Blob{Hash: hash('7'), Size: 128*1024 + 16}, BatteryBlob: battery, RTCBlob: &rtcA, ObservedAt: time.Now().UTC(), Provenance: model.ProvenanceConfirmed})
	if err != nil {
		t.Fatal(err)
	}
	windows, _ = db.FindBinding(ctx, "windows", "Pokemon Ruby.sav")
	rtcB := archive.Blob{Hash: hash('8'), Size: 16}
	second, err := db.RecordIngest(ctx, IngestParams{Binding: windows, ObservedBlob: archive.Blob{Hash: hash('9'), Size: 128*1024 + 16}, BatteryBlob: battery, RTCBlob: &rtcB, ObservedAt: time.Now().UTC().Add(time.Second), Provenance: model.ProvenanceConfirmed})
	if err != nil {
		t.Fatal(err)
	}
	if second.RevisionID == first.RevisionID || second.State != "head" || second.ContentHash == first.ContentHash {
		t.Fatalf("RTC-only update was not a new head: first=%#v second=%#v", first, second)
	}
	detail, err := db.GetGame(ctx, game.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(detail.Revisions) != 2 || detail.CurrentRevisionID != second.RevisionID {
		t.Fatalf("unexpected history after RTC-only update: %#v", detail)
	}
	if detail.Revisions[0].BlobHash != battery.Hash || detail.Observations[0].BlobHash != hash('9') {
		t.Fatalf("canonical revision or exact physical observation was lost: revision=%#v observation=%#v", detail.Revisions[0], detail.Observations[0])
	}
	payload, err := db.RevisionPayload(ctx, second.RevisionID)
	if err != nil {
		t.Fatal(err)
	}
	if payload.BatteryBlobHash != battery.Hash || payload.RTCBlobHash != rtcB.Hash {
		t.Fatalf("RTC-only payload=%#v", payload)
	}
}

func TestConfigureWindowsGBAProfilePreservesBindingBaseline(t *testing.T) {
	ctx := context.Background()
	db, _ := newTestStore(t)
	game, _ := db.CreateGame(ctx, "Golden Sun", model.PlatformGBA, "5566AABB", "")
	windows, _ := db.UpsertBinding(ctx, model.SaveBinding{GameID: game.ID, EndpointID: "windows", ProfileID: "windows-vbam", RelativePath: "Golden Sun.sav"})
	head, err := db.RecordIngest(ctx, IngestParams{Binding: windows, Blob: archive.Blob{Hash: hash('m'), Size: 64 * 1024}, ObservedAt: time.Now().UTC(), Provenance: model.ProvenanceConfirmed})
	if err != nil {
		t.Fatal(err)
	}
	changed, err := db.ConfigureWindowsGBAProfile(ctx, "windows-mgba", true)
	if err != nil {
		t.Fatal(err)
	}
	if len(changed) != 1 || changed[0].ProfileID != "windows-mgba" || changed[0].LastDeployedRevisionID != head.RevisionID {
		t.Fatalf("changed bindings=%#v", changed)
	}
	configured, err := db.FindBinding(ctx, "windows", "Golden Sun.sav")
	if err != nil {
		t.Fatal(err)
	}
	if configured.ProfileID != "windows-mgba" || configured.LastDeployedRevisionID != head.RevisionID {
		t.Fatalf("configured binding=%#v", configured)
	}
	settings, err := db.EmulatorSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if settings.WindowsGBAProfileID != "windows-mgba" || !settings.WindowsGBAConfigured || settings.AffectedBindings != 0 {
		t.Fatalf("settings=%#v", settings)
	}
}

func TestOperationIdentityIncludesProfileAndMaterializedHash(t *testing.T) {
	ctx := context.Background()
	db, _ := newTestStore(t)
	game, _ := db.CreateGame(ctx, "Pokemon LeafGreen", model.PlatformGBA, "AA55CC33", "")
	windows, _ := db.UpsertBinding(ctx, model.SaveBinding{GameID: game.ID, EndpointID: "windows", ProfileID: "windows-mgba", RelativePath: "Pokemon LeafGreen.sav"})
	battery := archive.Blob{Hash: hash('n'), Size: 128 * 1024}
	head, err := db.RecordIngest(ctx, IngestParams{Binding: windows, Blob: battery, ObservedAt: time.Now().UTC(), Provenance: model.ProvenanceConfirmed})
	if err != nil {
		t.Fatal(err)
	}
	wrappedA := archive.Blob{Hash: hash('o'), Size: 128*1024 + 16}
	first, err := db.CreateOperation(ctx, game.ID, head.RevisionID, "windows", windows.RelativePath, "windows-mgba", wrappedA)
	if err != nil {
		t.Fatal(err)
	}
	replay, err := db.CreateOperation(ctx, game.ID, head.RevisionID, "windows", windows.RelativePath, "windows-mgba", wrappedA)
	if err != nil || replay.ID != first.ID {
		t.Fatalf("identical materialization was not idempotent: first=%#v replay=%#v err=%v", first, replay, err)
	}
	wrappedB := archive.Blob{Hash: hash('p'), Size: 128*1024 + 16}
	changedBytes, err := db.CreateOperation(ctx, game.ID, head.RevisionID, "windows", windows.RelativePath, "windows-mgba", wrappedB)
	if err != nil {
		t.Fatal(err)
	}
	changedProfile, err := db.CreateOperation(ctx, game.ID, head.RevisionID, "windows", windows.RelativePath, "windows-vbam", battery)
	if err != nil {
		t.Fatal(err)
	}
	if first.ID == changedBytes.ID || changedBytes.ID == changedProfile.ID || changedProfile.ProfileID != "windows-vbam" || changedBytes.BlobHash != wrappedB.Hash {
		t.Fatalf("operation identity collapsed distinct targets: %#v %#v %#v", first, changedBytes, changedProfile)
	}
}

func TestRawStaleEditInheritsBaselineRTCNotCurrentHeadRTC(t *testing.T) {
	ctx := context.Background()
	db, _ := newTestStore(t)
	game, _ := db.CreateGame(ctx, "Pokemon Sapphire", model.PlatformGBA, "0BADF00D", "")
	windows, _ := db.UpsertBinding(ctx, model.SaveBinding{GameID: game.ID, EndpointID: "windows", ProfileID: "windows-mgba", RelativePath: "Pokemon Sapphire.sav"})
	thor, _ := db.UpsertBinding(ctx, model.SaveBinding{GameID: game.ID, EndpointID: "thor", ProfileID: "thor-mgba", RelativePath: "mGBA/Pokemon Sapphire.srm"})

	batteryA := archive.Blob{Hash: hash('g'), Size: 128 * 1024}
	rtcBaseline := archive.Blob{Hash: hash('h'), Size: 16}
	base, err := db.RecordIngest(ctx, IngestParams{Binding: windows, ObservedBlob: archive.Blob{Hash: hash('i'), Size: 128*1024 + 16}, BatteryBlob: batteryA, RTCBlob: &rtcBaseline, ObservedAt: time.Now().UTC(), Provenance: model.ProvenanceConfirmed})
	if err != nil {
		t.Fatal(err)
	}
	if err = db.MarkBindingDeployed(ctx, thor.ID, base.RevisionID); err != nil {
		t.Fatal(err)
	}

	// Windows changes only its RTC, so the game's current head now carries a
	// different RTC than the exact revision last delivered to the offline Thor.
	windows, _ = db.FindBinding(ctx, "windows", "Pokemon Sapphire.sav")
	rtcCurrent := archive.Blob{Hash: hash('j'), Size: 16}
	current, err := db.RecordIngest(ctx, IngestParams{Binding: windows, ObservedBlob: archive.Blob{Hash: hash('k'), Size: 128*1024 + 16}, BatteryBlob: batteryA, RTCBlob: &rtcCurrent, ObservedAt: time.Now().UTC().Add(time.Second), Provenance: model.ProvenanceConfirmed})
	if err != nil || current.State != "head" {
		t.Fatalf("RTC head=%#v err=%v", current, err)
	}

	thor, _ = db.FindBinding(ctx, "thor", "mGBA/Pokemon Sapphire.srm")
	batteryStale := archive.Blob{Hash: hash('l'), Size: 128 * 1024}
	branch, err := db.RecordIngest(ctx, IngestParams{Binding: thor, ObservedBlob: batteryStale, BatteryBlob: batteryStale, ObservedAt: time.Now().UTC().Add(2 * time.Second), Provenance: model.ProvenanceConfirmed})
	if err != nil {
		t.Fatal(err)
	}
	if branch.State != "branch" {
		t.Fatalf("stale edit state=%s, want branch", branch.State)
	}
	branchRevision, err := db.Revision(ctx, branch.RevisionID)
	if err != nil {
		t.Fatal(err)
	}
	if branchRevision.ParentRevisionID != base.RevisionID {
		t.Fatalf("stale branch parent=%s, want its deployed baseline %s", branchRevision.ParentRevisionID, base.RevisionID)
	}
	payload, err := db.RevisionPayload(ctx, branch.RevisionID)
	if err != nil {
		t.Fatal(err)
	}
	if payload.RTCBlobHash != rtcBaseline.Hash || payload.RTCBlobHash == rtcCurrent.Hash {
		t.Fatalf("stale branch inherited unrelated head RTC: %#v", payload)
	}
}

func TestRestorePromotionAndSnapshotCopyCompletePayload(t *testing.T) {
	ctx := context.Background()
	db, _ := newTestStore(t)
	game, _ := db.CreateGame(ctx, "Pokemon FireRed", model.PlatformGBA, "1122AABB", "")
	windows, _ := db.UpsertBinding(ctx, model.SaveBinding{GameID: game.ID, EndpointID: "windows", ProfileID: "windows-mgba", RelativePath: "Pokemon FireRed.sav"})

	firstBattery := archive.Blob{Hash: hash('a'), Size: 128 * 1024}
	firstRTC := archive.Blob{Hash: hash('b'), Size: 16}
	first, err := db.RecordIngest(ctx, IngestParams{Binding: windows, ObservedBlob: archive.Blob{Hash: hash('c'), Size: 128*1024 + 16}, BatteryBlob: firstBattery, RTCBlob: &firstRTC, ObservedAt: time.Now().UTC(), Provenance: model.ProvenanceConfirmed})
	if err != nil {
		t.Fatal(err)
	}
	windows, _ = db.FindBinding(ctx, "windows", "Pokemon FireRed.sav")
	secondBattery := archive.Blob{Hash: hash('d'), Size: 128 * 1024}
	secondRTC := archive.Blob{Hash: hash('e'), Size: 16}
	second, err := db.RecordIngest(ctx, IngestParams{Binding: windows, ObservedBlob: archive.Blob{Hash: hash('f'), Size: 128*1024 + 16}, BatteryBlob: secondBattery, RTCBlob: &secondRTC, ObservedAt: time.Now().UTC().Add(time.Second), Provenance: model.ProvenanceConfirmed})
	if err != nil {
		t.Fatal(err)
	}

	restored, err := db.CreateHeadFromExisting(ctx, HeadMutation{GameID: game.ID, SourceRevisionID: first.RevisionID, ExpectedHeadID: second.RevisionID, IdempotencyKey: "rtc-restore", Actor: "admin@example.com", Kind: "restore"})
	if err != nil {
		t.Fatal(err)
	}
	assertPayload(t, db, restored.ID, firstBattery, firstRTC)

	snapshot, err := db.CreateSnapshot(ctx, game.ID, restored.ID, "rtc-snapshot", "admin@example.com")
	if err != nil {
		t.Fatal(err)
	}
	assertPayload(t, db, snapshot.ID, firstBattery, firstRTC)

	promoted, err := db.CreateHeadFromExisting(ctx, HeadMutation{GameID: game.ID, SourceRevisionID: second.RevisionID, ExpectedHeadID: snapshot.ID, IdempotencyKey: "rtc-promote", Actor: "admin@example.com", Kind: "promote"})
	if err != nil {
		t.Fatal(err)
	}
	assertPayload(t, db, promoted.ID, secondBattery, secondRTC)
}

func TestFeatureMigrationBackfillsRowsCreatedDuringOldImageRollback(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	cfg := config.Config{DataDir: filepath.Join(root, "data"), ArchiveDir: filepath.Join(root, "archive"), ThorDir: filepath.Join(root, "thor"), WindowsDir: filepath.Join(root, "windows"), SyncthingThorFolderID: "thor-folder", SyncthingWindowsFolderID: "windows-folder"}
	db, err := Open(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	game, _ := db.CreateGame(ctx, "Legacy Write", model.PlatformGBA, "CAFEBABE", "")
	binding, _ := db.UpsertBinding(ctx, model.SaveBinding{GameID: game.ID, EndpointID: "thor", ProfileID: "thor-mgba", RelativePath: "Legacy Write.srm"})
	revision, err := db.RecordIngest(ctx, IngestParams{Binding: binding, Blob: archive.Blob{Hash: hash('0'), Size: 64 * 1024}, ObservedAt: time.Now().UTC(), Provenance: model.ProvenanceConfirmed})
	if err != nil {
		t.Fatal(err)
	}
	// This models the immediately previous image creating a revision while the
	// additive table is invisible to it after a temporary image rollback.
	if _, err = db.DB().ExecContext(ctx, "DELETE FROM revision_payloads WHERE revision_id=?", revision.RevisionID); err != nil {
		t.Fatal(err)
	}
	if err = db.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := Open(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	payload, err := reopened.RevisionPayload(ctx, revision.RevisionID)
	if err != nil {
		t.Fatal(err)
	}
	if payload.BatteryBlobHash != revision.ContentHash || payload.ContentHash != revision.ContentHash {
		t.Fatalf("rollback revision was not backfilled: %#v", payload)
	}
	var rows int
	if err = reopened.DB().QueryRowContext(ctx, "SELECT COUNT(*) FROM revision_payloads WHERE revision_id=?", revision.RevisionID).Scan(&rows); err != nil || rows != 1 {
		t.Fatalf("persisted payload rows=%d err=%v, want 1", rows, err)
	}
	var version int
	if err = reopened.DB().QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil || version != 1 {
		t.Fatalf("user_version=%d err=%v, want 1", version, err)
	}
}

func TestAdditivePayloadMigrationBacksUpAndKeepsV1Readable(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	cfg := config.Config{DataDir: filepath.Join(root, "data"), ArchiveDir: filepath.Join(root, "archive"), ThorDir: filepath.Join(root, "thor"), WindowsDir: filepath.Join(root, "windows"), SyncthingThorFolderID: "thor-folder", SyncthingWindowsFolderID: "windows-folder"}
	db, err := Open(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	game, _ := db.CreateGame(ctx, "Legacy Database", model.PlatformGBA, "DEADC0DE", "")
	binding, _ := db.UpsertBinding(ctx, model.SaveBinding{GameID: game.ID, EndpointID: "thor", ProfileID: "thor-mgba", RelativePath: "Legacy Database.srm"})
	legacyBlob := archive.Blob{Hash: hash('z'), Size: 64 * 1024}
	revision, err := db.RecordIngest(ctx, IngestParams{Binding: binding, Blob: legacyBlob, ObservedAt: time.Now().UTC(), Provenance: model.ProvenanceConfirmed})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.DB().ExecContext(ctx, "DELETE FROM settings WHERE key=?", revisionPayloadsMigrationKey); err != nil {
		t.Fatal(err)
	}
	if _, err = db.DB().ExecContext(ctx, "DROP TABLE revision_payloads"); err != nil {
		t.Fatal(err)
	}
	if err = db.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := Open(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	payload, err := reopened.RevisionPayload(ctx, revision.RevisionID)
	if err != nil || payload.BatteryBlobHash != legacyBlob.Hash {
		t.Fatalf("legacy revision payload=%#v err=%v", payload, err)
	}
	if err = reopened.Close(); err != nil {
		t.Fatal(err)
	}

	backups, err := filepath.Glob(filepath.Join(cfg.DataDir, "migration-backups", "thorsync-before-revision-payloads-*.db"))
	if err != nil || len(backups) != 1 {
		t.Fatalf("feature backups=%v err=%v", backups, err)
	}
	legacy, err := sql.Open("sqlite", "file:"+filepath.ToSlash(backups[0])+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	defer legacy.Close()
	var version, revisions int
	if err = legacy.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		t.Fatal(err)
	}
	if err = legacy.QueryRowContext(ctx, "SELECT COUNT(*) FROM revisions").Scan(&revisions); err != nil {
		t.Fatal(err)
	}
	if version != 1 || revisions != 1 {
		t.Fatalf("backup user_version=%d revisions=%d", version, revisions)
	}
	if _, err = os.Stat(backups[0]); err != nil {
		t.Fatal(err)
	}
}

func assertPayload(t *testing.T, db *Store, revisionID string, battery, rtc archive.Blob) {
	t.Helper()
	payload, err := db.RevisionPayload(context.Background(), revisionID)
	if err != nil {
		t.Fatal(err)
	}
	if payload.BatteryBlobHash != battery.Hash || payload.BatterySize != battery.Size || payload.RTCBlobHash != rtc.Hash || payload.RTCSize != rtc.Size || payload.ContentHash != PayloadContentHash(battery.Hash, rtc.Hash) {
		t.Fatalf("revision %s payload=%#v", revisionID, payload)
	}
}
