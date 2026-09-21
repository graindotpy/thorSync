package api

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/apgul/thorsync/internal/archive"
	"github.com/apgul/thorsync/internal/broker"
	"github.com/apgul/thorsync/internal/config"
	"github.com/apgul/thorsync/internal/events"
	"github.com/apgul/thorsync/internal/model"
	"github.com/apgul/thorsync/internal/store"
)

func TestSaveArtworkRejectsPathTraversal(t *testing.T) {
	file, err := os.CreateTemp(t.TempDir(), "artwork")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if _, err = saveArtwork(t.TempDir(), "../../outside", file, &multipart.FileHeader{}); err == nil {
		t.Fatal("expected traversal game ID to be rejected")
	}
	for _, id := range []string{"", "ABCDEF", "0000000000000000000000000000000g", "00000000000000000000000000000000/"} {
		if validObjectID(id) {
			t.Errorf("invalid object ID accepted: %q", id)
		}
	}
}

func TestSaveArtworkStaysInsideArtworkDirectory(t *testing.T) {
	dir := t.TempDir()
	file, err := os.CreateTemp(t.TempDir(), "cover-*.png")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	// PNG signature plus enough bytes for content sniffing. Decoding is left to
	// the browser; the server only accepts a conservative image MIME allowlist.
	payload := append([]byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}, make([]byte, 512)...)
	if _, err = file.Write(payload); err != nil {
		t.Fatal(err)
	}
	if _, err = file.Seek(0, 0); err != nil {
		t.Fatal(err)
	}
	path, err := saveArtwork(dir, "0123456789abcdef0123456789abcdef", file, &multipart.FileHeader{Size: int64(len(payload))})
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Dir(path) != dir || filepath.Ext(path) != ".png" {
		t.Fatalf("artwork escaped destination: %s", path)
	}
}

func TestGameDeliveryReflectsPendingAndMissingEndpoints(t *testing.T) {
	now := time.Now().UTC()
	detail := model.GameDetail{
		Game: model.Game{ID: "game", CurrentRevisionID: "head", Status: "healthy", UpdatedAt: now},
		Bindings: []model.SaveBinding{
			{EndpointID: "thor", RelativePath: "game.srm", LastDeployedRevisionID: "head"},
			{EndpointID: "windows", RelativePath: "game.sav", LastDeployedRevisionID: "base"},
		},
		Operations: []model.BrokerOperation{{RevisionID: "head", TargetEndpointID: "windows", RelativePath: "game.sav", State: "written"}},
	}
	state, _ := gameDelivery(detail)
	if state != "syncing" {
		t.Fatalf("state=%s, want syncing", state)
	}
	detail.Observations = []model.Observation{{EndpointID: "windows", RelativePath: "game.sav", Action: "missing", ObservedAt: now}}
	state, _ = gameDelivery(detail)
	if state != "missing" {
		t.Fatalf("state=%s, want missing", state)
	}
}

func TestRevisionDTOUsesLogicalContentHash(t *testing.T) {
	dto := toRevisionDTO(model.Revision{
		BlobHash:    "physical-blob-hash",
		ContentHash: "logical-content-hash",
		ObservedAt:  time.Now().UTC(),
	})
	if dto.ShortHash != "logical-cont" {
		t.Fatalf("short hash=%q, want logical content hash prefix", dto.ShortHash)
	}
}

func TestGameDetailBindingIncludesProfileMetadata(t *testing.T) {
	detail := model.GameDetail{
		Game: model.Game{ID: "game", CurrentRevisionID: "rtc"},
		Bindings: []model.SaveBinding{
			{ID: "mgba", EndpointID: "windows", ProfileID: "windows-mgba", RelativePath: "game.sav", LastDeployedRevisionID: "rtc"},
			{ID: "vbam", EndpointID: "windows", ProfileID: "windows-vbam", RelativePath: "other.sav", LastDeployedRevisionID: "raw"},
		},
		Revisions: []model.Revision{{ID: "rtc", HasRTC: true}, {ID: "raw", HasRTC: false}},
	}
	dto := toGameDetailDTO(detail)
	if len(dto.Bindings) != 2 {
		t.Fatalf("bindings=%d, want 2", len(dto.Bindings))
	}
	if got := dto.Bindings[0]; got.ProfileID != "windows-mgba" || got.ProfileName != "mGBA" || got.Format != "raw-battery+opaque-rtc" || !got.HasRTC {
		t.Fatalf("unexpected mGBA binding DTO: %#v", got)
	}
	if got := dto.Bindings[1]; got.ProfileID != "windows-vbam" || got.ProfileName != "VisualBoyAdvance-M" || got.Format != "raw-battery" || got.HasRTC {
		t.Fatalf("unexpected VBA-M binding DTO: %#v", got)
	}
}

func TestUnassignedDTOUsesMatchingProfilesAndConfiguredDefault(t *testing.T) {
	raw := store.UnassignedFile{EndpointID: "windows", RelativePath: "game.sav", Size: 64 * 1024}
	dto := toUnassignedDTO(raw, "windows-mgba")
	wantCompatible := []string{"windows-vbam", "windows-mgba", "windows-melonds"}
	if !reflect.DeepEqual(dto.CompatibleProfileIDs, wantCompatible) {
		t.Fatalf("compatible profiles=%v, want %v", dto.CompatibleProfileIDs, wantCompatible)
	}
	if dto.SuggestedProfileID != "windows-mgba" {
		t.Fatalf("suggestion=%q, want configured mGBA default", dto.SuggestedProfileID)
	}
	if got := toUnassignedDTO(raw, "windows-vbam").SuggestedProfileID; got != "windows-vbam" {
		t.Fatalf("suggestion=%q, want configured VBA-M default", got)
	}

	wrapped := store.UnassignedFile{EndpointID: "windows", RelativePath: "game.sav", Size: 64*1024 + 16}
	dto = toUnassignedDTO(wrapped, "windows-vbam")
	if !reflect.DeepEqual(dto.CompatibleProfileIDs, []string{"windows-mgba"}) || dto.SuggestedProfileID != "windows-mgba" {
		t.Fatalf("wrapped mGBA detection failed: %#v", dto)
	}
	if got := detectedWindowsGBAProfile([]store.UnassignedFile{raw, wrapped}); got != "windows-mgba" {
		t.Fatalf("detected profile=%q, want windows-mgba", got)
	}
}

func TestEmulatorSettingsRoutesValidateAndMigrateBindings(t *testing.T) {
	api, db := newTestAPI(t)
	ctx := context.Background()
	game, err := db.CreateGame(ctx, "Advance Wars", model.PlatformGBA, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.UpsertBinding(ctx, model.SaveBinding{GameID: game.ID, EndpointID: "windows", ProfileID: "windows-vbam", RelativePath: "Advance Wars.sav"}); err != nil {
		t.Fatal(err)
	}

	response := serveJSON(t, api.Routes(), http.MethodGet, "/api/v1/settings/emulators", nil)
	if response.Code != http.StatusOK {
		t.Fatalf("GET status=%d body=%s", response.Code, response.Body.String())
	}
	var before emulatorSettingsDTO
	if err = json.Unmarshal(response.Body.Bytes(), &before); err != nil {
		t.Fatal(err)
	}
	if before.WindowsGBAProfileID != "windows-mgba" || before.Configured || before.AffectedBindings != 1 {
		t.Fatalf("unexpected initial settings: %#v", before)
	}

	response = serveJSON(t, api.Routes(), http.MethodPut, "/api/v1/settings/emulators/windows-gba", map[string]any{
		"profileId": "windows-mgba", "emulatorClosed": false, "applyToExisting": true,
	})
	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "emulators are closed") {
		t.Fatalf("unsafe change status=%d body=%s", response.Code, response.Body.String())
	}
	response = serveJSON(t, api.Routes(), http.MethodPut, "/api/v1/settings/emulators/windows-gba", map[string]any{
		"profileId": "unsupported", "emulatorClosed": true, "applyToExisting": true,
	})
	if response.Code != http.StatusBadRequest {
		t.Fatalf("invalid profile status=%d body=%s", response.Code, response.Body.String())
	}

	response = serveJSON(t, api.Routes(), http.MethodPut, "/api/v1/settings/emulators/windows-gba", map[string]any{
		"profileId": "windows-mgba", "emulatorClosed": true, "applyToExisting": true,
	})
	if response.Code != http.StatusOK {
		t.Fatalf("migration status=%d body=%s", response.Code, response.Body.String())
	}
	var after emulatorSettingsDTO
	if err = json.Unmarshal(response.Body.Bytes(), &after); err != nil {
		t.Fatal(err)
	}
	if after.WindowsGBAProfileID != "windows-mgba" || !after.Configured || after.AffectedBindings != 0 {
		t.Fatalf("unexpected migrated settings: %#v", after)
	}
	binding, err := db.FindBinding(ctx, "windows", "Advance Wars.sav")
	if err != nil || binding.ProfileID != "windows-mgba" {
		t.Fatalf("binding was not migrated: %#v, %v", binding, err)
	}
}

func TestEmulatorSettingsRecapturesExistingWrappedMGBAQuarantine(t *testing.T) {
	api, db := newTestAPI(t)
	ctx := context.Background()
	game, err := db.CreateGame(ctx, "Pokemon Lazarus", model.PlatformGBA, "", "")
	if err != nil {
		t.Fatal(err)
	}
	windows, err := db.UpsertBinding(ctx, model.SaveBinding{GameID: game.ID, EndpointID: "windows", ProfileID: "windows-vbam", RelativePath: "Pokemon Lazarus.sav"})
	if err != nil {
		t.Fatal(err)
	}
	battery := bytes.Repeat([]byte{0x21}, 128*1024)
	rtc := bytes.Repeat([]byte{0x43}, 16)
	batteryBlob, err := api.archive.PutBytes(battery)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.RecordIngest(ctx, store.IngestParams{Binding: windows, ObservedBlob: batteryBlob, BatteryBlob: batteryBlob}); err != nil {
		t.Fatal(err)
	}
	windowsRoot, err := db.RootForEndpoint(ctx, "windows")
	if err != nil {
		t.Fatal(err)
	}
	if err = os.MkdirAll(windowsRoot, 0o750); err != nil {
		t.Fatal(err)
	}
	wrapped := append(append([]byte(nil), battery...), rtc...)
	if err = os.WriteFile(filepath.Join(windowsRoot, "Pokemon Lazarus.sav"), wrapped, 0o640); err != nil {
		t.Fatal(err)
	}

	response := serveJSON(t, api.Routes(), http.MethodPut, "/api/v1/settings/emulators/windows-gba", map[string]any{
		"profileId": "windows-mgba", "emulatorClosed": true, "applyToExisting": true,
	})
	if response.Code != http.StatusOK {
		t.Fatalf("migration status=%d body=%s", response.Code, response.Body.String())
	}
	windows, err = db.FindBinding(ctx, "windows", "Pokemon Lazarus.sav")
	if err != nil || windows.ProfileID != "windows-mgba" {
		t.Fatalf("binding was not switched to mGBA: %#v err=%v", windows, err)
	}
	detail, err := db.GetGame(ctx, game.ID)
	if err != nil || len(detail.Revisions) != 2 || !detail.Revisions[0].HasRTC {
		t.Fatalf("wrapped file was not recaptured with RTC: revisions=%#v err=%v", detail.Revisions, err)
	}
	if items, listErr := db.ListUnassigned(ctx); listErr != nil || len(items) != 0 {
		t.Fatalf("successful recapture did not clear quarantine: %#v err=%v", items, listErr)
	}
}

func TestBindingRemapRequiresClosedEmulatorButInitialMappingDoesNot(t *testing.T) {
	api, db := newTestAPI(t)
	game, err := db.CreateGame(context.Background(), "Metroid Fusion", model.PlatformGBA, "", "")
	if err != nil {
		t.Fatal(err)
	}
	path := "/api/v1/games/" + game.ID + "/bindings"
	response := serveJSON(t, api.Routes(), http.MethodPut, path, map[string]any{
		"endpointId": "windows", "profileId": "windows-vbam", "relativePath": "Metroid Fusion.sav",
	})
	if response.Code != http.StatusOK {
		t.Fatalf("initial mapping status=%d body=%s", response.Code, response.Body.String())
	}
	response = serveJSON(t, api.Routes(), http.MethodPut, path, map[string]any{
		"endpointId": "windows", "profileId": "windows-mgba", "relativePath": "Metroid Fusion.sav",
	})
	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "emulators are closed") {
		t.Fatalf("unsafe remap status=%d body=%s", response.Code, response.Body.String())
	}
	response = serveJSON(t, api.Routes(), http.MethodPut, path, map[string]any{
		"endpointId": "windows", "profileId": "windows-mgba", "relativePath": "Metroid Fusion.sav", "emulatorClosed": true,
	})
	if response.Code != http.StatusOK {
		t.Fatalf("confirmed remap status=%d body=%s", response.Code, response.Body.String())
	}
}

func newTestAPI(t *testing.T) (*API, *store.Store) {
	t.Helper()
	root := t.TempDir()
	cfg := config.Config{
		DataDir:                  filepath.Join(root, "data"),
		ArchiveDir:               filepath.Join(root, "archive"),
		ThorDir:                  filepath.Join(root, "thor"),
		WindowsDir:               filepath.Join(root, "windows"),
		SyncthingThorFolderID:    "thor-folder",
		SyncthingWindowsFolderID: "windows-folder",
	}
	db, err := store.Open(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	hub := events.New()
	archiveStore := archive.New(cfg.ArchiveDir, 5<<30, 0)
	brokerService := broker.New(db, archiveStore, hub)
	t.Cleanup(func() {
		brokerService.Close()
		_ = db.Close()
	})
	return New(db, archiveStore, brokerService, nil, hub, filepath.Join(root, "artwork"), cfg.SyncthingThorFolderID, cfg.SyncthingWindowsFolderID, nil), db
}

func serveJSON(t *testing.T, handler http.Handler, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var payload bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&payload).Encode(body); err != nil {
			t.Fatal(err)
		}
	}
	request := httptest.NewRequest(method, path, &payload)
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}
