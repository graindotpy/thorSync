package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/apgul/thorsync/internal/archive"
	"github.com/apgul/thorsync/internal/config"
	"github.com/apgul/thorsync/internal/model"
)

func newTestStore(t *testing.T) (*Store, config.Config) {
	t.Helper()
	root := t.TempDir()
	cfg := config.Config{DataDir: filepath.Join(root, "data"), ArchiveDir: filepath.Join(root, "archive"), ThorDir: filepath.Join(root, "thor"), WindowsDir: filepath.Join(root, "windows"), SyncthingThorFolderID: "thor-folder", SyncthingWindowsFolderID: "windows-folder"}
	db, err := Open(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db, cfg
}

func TestDivergentEndpointCreatesBranchWithoutMovingHead(t *testing.T) {
	ctx := context.Background()
	db, _ := newTestStore(t)
	game, err := db.CreateGame(ctx, "Advance Wars", model.PlatformGBA, "AABBCCDD", "")
	if err != nil {
		t.Fatal(err)
	}
	thor, err := db.UpsertBinding(ctx, model.SaveBinding{GameID: game.ID, EndpointID: "thor", ProfileID: "thor-mgba", RelativePath: "Advance Wars.srm"})
	if err != nil {
		t.Fatal(err)
	}
	windows, err := db.UpsertBinding(ctx, model.SaveBinding{GameID: game.ID, EndpointID: "windows", ProfileID: "windows-vbam", RelativePath: "Advance Wars.sav"})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	first, err := db.RecordIngest(ctx, IngestParams{Binding: thor, Blob: archive.Blob{Hash: hash('a'), Size: 64 * 1024}, ObservedAt: now, Provenance: model.ProvenanceConfirmed})
	if err != nil {
		t.Fatal(err)
	}
	second, err := db.RecordIngest(ctx, IngestParams{Binding: windows, Blob: archive.Blob{Hash: hash('b'), Size: 64 * 1024}, ObservedAt: now.Add(time.Second), Provenance: model.ProvenanceConfirmed})
	if err != nil {
		t.Fatal(err)
	}
	if second.State != "branch" {
		t.Fatalf("state=%s, want branch", second.State)
	}
	detail, err := db.GetGame(ctx, game.ID)
	if err != nil {
		t.Fatal(err)
	}
	if detail.CurrentRevisionID != first.RevisionID {
		t.Fatalf("head moved to branch: got %s want %s", detail.CurrentRevisionID, first.RevisionID)
	}
	if detail.ConflictCount != 1 {
		t.Fatalf("conflicts=%d, want 1", detail.ConflictCount)
	}
}

func TestGameWithoutRevisionCanBeListedAndDeduplicatedByHash(t *testing.T) {
	ctx := context.Background()
	db, _ := newTestStore(t)
	created, err := db.CreateGame(ctx, "No Save Yet", model.PlatformGBA, "ABCDEF01", "")
	if err != nil {
		t.Fatal(err)
	}
	duplicate, err := db.CreateGame(ctx, "Duplicate Title", model.PlatformGBA, "ABCDEF01", "")
	if err != nil {
		t.Fatal(err)
	}
	if duplicate.ID != created.ID {
		t.Fatalf("hash import created duplicate games: %s != %s", duplicate.ID, created.ID)
	}
	games, err := db.ListGames(ctx, "", "")
	if err != nil || len(games) != 1 || games[0].CurrentRevisionID != "" {
		t.Fatalf("unconfigured game list=%#v err=%v", games, err)
	}
}

func TestSameBlobCreatesObservationNotRevision(t *testing.T) {
	ctx := context.Background()
	db, _ := newTestStore(t)
	game, _ := db.CreateGame(ctx, "Mario Kart DS", model.PlatformNDS, "11223344", "")
	thor, _ := db.UpsertBinding(ctx, model.SaveBinding{GameID: game.ID, EndpointID: "thor", ProfileID: "thor-melonds-ds", RelativePath: "Mario Kart DS.srm"})
	windows, _ := db.UpsertBinding(ctx, model.SaveBinding{GameID: game.ID, EndpointID: "windows", ProfileID: "windows-melonds", RelativePath: "Mario Kart DS.sav"})
	blob := archive.Blob{Hash: hash('c'), Size: 512 * 1024}
	first, err := db.RecordIngest(ctx, IngestParams{Binding: thor, Blob: blob, ObservedAt: time.Now().UTC(), Provenance: model.ProvenanceConfirmed})
	if err != nil {
		t.Fatal(err)
	}
	second, err := db.RecordIngest(ctx, IngestParams{Binding: windows, Blob: blob, ObservedAt: time.Now().UTC(), Provenance: model.ProvenanceConfirmed})
	if err != nil {
		t.Fatal(err)
	}
	if !second.Echo || second.RevisionID != first.RevisionID {
		t.Fatalf("duplicate was not linked to existing revision: %#v", second)
	}
	detail, _ := db.GetGame(ctx, game.ID)
	if len(detail.Revisions) != 1 || len(detail.Observations) != 2 {
		t.Fatalf("got %d revisions and %d observations", len(detail.Revisions), len(detail.Observations))
	}
}

func TestRepeatedConflictArtifactDoesNotDuplicateRevision(t *testing.T) {
	ctx := context.Background()
	db, _ := newTestStore(t)
	game, _ := db.CreateGame(ctx, "Castlevania", model.PlatformGBA, "DEADBEEF", "")
	thor, _ := db.UpsertBinding(ctx, model.SaveBinding{GameID: game.ID, EndpointID: "thor", ProfileID: "thor-mgba", RelativePath: "Castlevania.srm"})
	windows, _ := db.UpsertBinding(ctx, model.SaveBinding{GameID: game.ID, EndpointID: "windows", ProfileID: "windows-vbam", RelativePath: "Castlevania.sav"})
	_, _ = db.RecordIngest(ctx, IngestParams{Binding: thor, Blob: archive.Blob{Hash: hash('5'), Size: 64 * 1024}, ObservedAt: time.Now().UTC(), Provenance: model.ProvenanceConfirmed})
	first, err := db.RecordIngest(ctx, IngestParams{Binding: windows, Blob: archive.Blob{Hash: hash('6'), Size: 64 * 1024}, ObservedAt: time.Now().UTC().Add(time.Second), Provenance: model.ProvenanceConfirmed, ForceConflict: true})
	if err != nil {
		t.Fatal(err)
	}
	repeat, err := db.RecordIngest(ctx, IngestParams{Binding: windows, Blob: archive.Blob{Hash: hash('6'), Size: 64 * 1024}, ObservedAt: time.Now().UTC().Add(2 * time.Second), Provenance: model.ProvenanceConfirmed, ForceConflict: true})
	if err != nil {
		t.Fatal(err)
	}
	detail, _ := db.GetGame(ctx, game.ID)
	if repeat.RevisionID != first.RevisionID || len(detail.Revisions) != 2 || detail.ConflictCount != 1 || len(detail.Observations) != 3 {
		t.Fatalf("duplicate conflict was not deduplicated: first=%#v repeat=%#v revisions=%d conflicts=%d observations=%d", first, repeat, len(detail.Revisions), detail.ConflictCount, len(detail.Observations))
	}
}

func TestRestoreIsNewIdempotentHead(t *testing.T) {
	ctx := context.Background()
	db, _ := newTestStore(t)
	game, _ := db.CreateGame(ctx, "Metroid Fusion", model.PlatformGBA, "55667788", "")
	thor, _ := db.UpsertBinding(ctx, model.SaveBinding{GameID: game.ID, EndpointID: "thor", ProfileID: "thor-mgba", RelativePath: "Metroid Fusion.srm"})
	first, _ := db.RecordIngest(ctx, IngestParams{Binding: thor, Blob: archive.Blob{Hash: hash('d'), Size: 64 * 1024}, ObservedAt: time.Now().UTC(), Provenance: model.ProvenanceConfirmed})
	updatedBinding, _ := db.FindBinding(ctx, "thor", "Metroid Fusion.srm")
	second, _ := db.RecordIngest(ctx, IngestParams{Binding: updatedBinding, Blob: archive.Blob{Hash: hash('e'), Size: 64 * 1024}, ObservedAt: time.Now().UTC().Add(time.Second), Provenance: model.ProvenanceConfirmed})
	restored, err := db.CreateHeadFromExisting(ctx, HeadMutation{GameID: game.ID, SourceRevisionID: first.RevisionID, ExpectedHeadID: second.RevisionID, IdempotencyKey: "restore-key", Actor: "admin@example.com", Kind: "restore"})
	if err != nil {
		t.Fatal(err)
	}
	again, err := db.CreateHeadFromExisting(ctx, HeadMutation{GameID: game.ID, SourceRevisionID: first.RevisionID, ExpectedHeadID: second.RevisionID, IdempotencyKey: "restore-key", Actor: "admin@example.com", Kind: "restore"})
	if err != nil {
		t.Fatal(err)
	}
	if restored.ID != again.ID || restored.ParentRevisionID != second.RevisionID || restored.RestoredFromID != first.RevisionID {
		t.Fatalf("bad restore lineage: %#v %#v", restored, again)
	}
}

func TestOfflineEndpointKeepsOldBaselineAndCreatesBranch(t *testing.T) {
	ctx := context.Background()
	db, _ := newTestStore(t)
	game, _ := db.CreateGame(ctx, "Fire Emblem", model.PlatformGBA, "10203040", "")
	thor, _ := db.UpsertBinding(ctx, model.SaveBinding{GameID: game.ID, EndpointID: "thor", ProfileID: "thor-mgba", RelativePath: "Fire Emblem.srm"})
	windows, _ := db.UpsertBinding(ctx, model.SaveBinding{GameID: game.ID, EndpointID: "windows", ProfileID: "windows-vbam", RelativePath: "Fire Emblem.sav"})

	base, err := db.RecordIngest(ctx, IngestParams{Binding: thor, Blob: archive.Blob{Hash: hash('f'), Size: 64 * 1024}, ObservedAt: time.Now().UTC(), Provenance: model.ProvenanceConfirmed})
	if err != nil {
		t.Fatal(err)
	}
	// Both physical devices have the base revision.
	if err = db.MarkBindingDeployed(ctx, windows.ID, base.RevisionID); err != nil {
		t.Fatal(err)
	}
	thor, _ = db.FindBinding(ctx, "thor", "Fire Emblem.srm")
	windows, _ = db.FindBinding(ctx, "windows", "Fire Emblem.sav")

	newHead, err := db.RecordIngest(ctx, IngestParams{Binding: thor, Blob: archive.Blob{Hash: hash('1'), Size: 64 * 1024}, ObservedAt: time.Now().UTC().Add(time.Second), Provenance: model.ProvenanceConfirmed})
	if err != nil || newHead.State != "head" {
		t.Fatalf("new head: %#v, %v", newHead, err)
	}
	// Windows remained offline, so its independently edited save still
	// descends from base rather than the new head.
	branch, err := db.RecordIngest(ctx, IngestParams{Binding: windows, Blob: archive.Blob{Hash: hash('2'), Size: 64 * 1024}, ObservedAt: time.Now().UTC().Add(2 * time.Second), Provenance: model.ProvenanceConfirmed})
	if err != nil {
		t.Fatal(err)
	}
	if branch.State != "branch" {
		t.Fatalf("offline edit state=%s, want branch", branch.State)
	}
	detail, _ := db.GetGame(ctx, game.ID)
	if detail.CurrentRevisionID != newHead.RevisionID || detail.ConflictCount != 1 {
		t.Fatalf("head/conflict changed incorrectly: %#v", detail.Game)
	}
}

func TestOperationJournalIsIdempotentAndCompletionAdvancesBaseline(t *testing.T) {
	ctx := context.Background()
	db, _ := newTestStore(t)
	game, _ := db.CreateGame(ctx, "WarioWare", model.PlatformGBA, "50607080", "")
	thor, _ := db.UpsertBinding(ctx, model.SaveBinding{GameID: game.ID, EndpointID: "thor", ProfileID: "thor-mgba", RelativePath: "WarioWare.srm"})
	_, _ = db.UpsertBinding(ctx, model.SaveBinding{GameID: game.ID, EndpointID: "windows", ProfileID: "windows-vbam", RelativePath: "WarioWare.sav"})
	revision, err := db.RecordIngest(ctx, IngestParams{Binding: thor, Blob: archive.Blob{Hash: hash('3'), Size: 64 * 1024}, ObservedAt: time.Now().UTC(), Provenance: model.ProvenanceConfirmed})
	if err != nil {
		t.Fatal(err)
	}
	op, err := db.CreateOperation(ctx, game.ID, revision.RevisionID, "windows", "WarioWare.sav", hash('3'))
	if err != nil {
		t.Fatal(err)
	}
	again, err := db.CreateOperation(ctx, game.ID, revision.RevisionID, "windows", "WarioWare.sav", hash('3'))
	if err != nil || again.ID != op.ID {
		t.Fatalf("operation was not idempotent: %#v %#v %v", op, again, err)
	}
	if err = db.UpdateOperation(ctx, op.ID, "written", ""); err != nil {
		t.Fatal(err)
	}
	before, _ := db.FindBinding(ctx, "windows", "WarioWare.sav")
	if before.LastDeployedRevisionID != "" {
		t.Fatalf("hub write advanced physical baseline early: %s", before.LastDeployedRevisionID)
	}
	count, err := db.CompleteEndpointDeliveries(ctx, "windows")
	if err != nil || count != 1 {
		t.Fatalf("completion count=%d err=%v", count, err)
	}
	after, _ := db.FindBinding(ctx, "windows", "WarioWare.sav")
	if after.LastDeployedRevisionID != revision.RevisionID {
		t.Fatalf("completed baseline=%s want %s", after.LastDeployedRevisionID, revision.RevisionID)
	}
	if count, err = db.CompleteEndpointDeliveries(ctx, "windows"); err != nil || count != 0 {
		t.Fatalf("completion replay count=%d err=%v", count, err)
	}
}

func TestRepeatedDeletionReconciliationIsDeduplicated(t *testing.T) {
	ctx := context.Background()
	db, _ := newTestStore(t)
	game, _ := db.CreateGame(ctx, "Kirby", model.PlatformGBA, "90A0B0C0", "")
	_, _ = db.UpsertBinding(ctx, model.SaveBinding{GameID: game.ID, EndpointID: "thor", ProfileID: "thor-mgba", RelativePath: "Kirby.srm"})
	if err := db.RecordDeletion(ctx, "thor", "Kirby.srm", nil, model.ProvenanceUnknown); err != nil {
		t.Fatal(err)
	}
	if err := db.RecordDeletion(ctx, "thor", "Kirby.srm", nil, model.ProvenanceUnknown); err != nil {
		t.Fatal(err)
	}
	detail, err := db.GetGame(ctx, game.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(detail.Observations) != 1 || detail.Observations[0].Action != "missing" {
		t.Fatalf("repeated missing observations were not deduplicated: %#v", detail.Observations)
	}
}

func TestManualSnapshotPreservesBytesAndDeployedBaselines(t *testing.T) {
	ctx := context.Background()
	db, _ := newTestStore(t)
	game, _ := db.CreateGame(ctx, "Minish Cap", model.PlatformGBA, "0A0B0C0D", "")
	thor, _ := db.UpsertBinding(ctx, model.SaveBinding{GameID: game.ID, EndpointID: "thor", ProfileID: "thor-mgba", RelativePath: "Minish Cap.srm"})
	windows, _ := db.UpsertBinding(ctx, model.SaveBinding{GameID: game.ID, EndpointID: "windows", ProfileID: "windows-vbam", RelativePath: "Minish Cap.sav"})
	head, err := db.RecordIngest(ctx, IngestParams{Binding: thor, Blob: archive.Blob{Hash: hash('4'), Size: 64 * 1024}, ObservedAt: time.Now().UTC(), Provenance: model.ProvenanceConfirmed})
	if err != nil {
		t.Fatal(err)
	}
	if err = db.MarkBindingDeployed(ctx, windows.ID, head.RevisionID); err != nil {
		t.Fatal(err)
	}
	snapshot, err := db.CreateSnapshot(ctx, game.ID, head.RevisionID, "snapshot-key", "admin@example.com")
	if err != nil {
		t.Fatal(err)
	}
	again, err := db.CreateSnapshot(ctx, game.ID, head.RevisionID, "snapshot-key", "admin@example.com")
	if err != nil || again.ID != snapshot.ID {
		t.Fatalf("snapshot replay was not idempotent: %#v %#v %v", snapshot, again, err)
	}
	if snapshot.ParentRevisionID != head.RevisionID || snapshot.BlobHash != hash('4') || snapshot.Kind != "snapshot" {
		t.Fatalf("snapshot lineage or bytes changed: %#v", snapshot)
	}
	for _, endpoint := range []struct{ id, path string }{{"thor", "Minish Cap.srm"}, {"windows", "Minish Cap.sav"}} {
		binding, err := db.FindBinding(ctx, endpoint.id, endpoint.path)
		if err != nil || binding.LastDeployedRevisionID != snapshot.ID {
			t.Fatalf("%s baseline=%s err=%v, want snapshot", endpoint.id, binding.LastDeployedRevisionID, err)
		}
	}
}

func hash(value byte) string {
	result := make([]byte, 64)
	for i := range result {
		result[i] = value
	}
	return string(result)
}
