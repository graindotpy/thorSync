package api

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"os"
	pathpkg "path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/apgul/thorsync/internal/archive"
	"github.com/apgul/thorsync/internal/auth"
	"github.com/apgul/thorsync/internal/broker"
	"github.com/apgul/thorsync/internal/catalog"
	"github.com/apgul/thorsync/internal/events"
	"github.com/apgul/thorsync/internal/ids"
	"github.com/apgul/thorsync/internal/model"
	"github.com/apgul/thorsync/internal/store"
	"github.com/apgul/thorsync/internal/syncthing"
)

type API struct {
	store           *store.Store
	archive         *archive.Archive
	broker          *broker.Service
	syncthing       *syncthing.Client
	hub             *events.Hub
	artworkDir      string
	requiredDirs    []string
	started         time.Time
	thorFolderID    string
	windowsFolderID string
	authCheck       func(context.Context) error
}

func (a *API) SetAuthenticationCheck(check func(context.Context) error) {
	a.authCheck = check
}

func New(db *store.Store, archiveStore *archive.Archive, brokerService *broker.Service, syncClient *syncthing.Client, hub *events.Hub, artworkDir, thorFolderID, windowsFolderID string, requiredDirs []string) *API {
	return &API{store: db, archive: archiveStore, broker: brokerService, syncthing: syncClient, hub: hub, artworkDir: artworkDir, requiredDirs: append([]string{artworkDir}, requiredDirs...), started: time.Now().UTC(), thorFolderID: thorFolderID, windowsFolderID: windowsFolderID}
}

func (a *API) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health/live", a.live)
	mux.HandleFunc("GET /health/ready", a.ready)
	mux.HandleFunc("GET /api/v1/games", a.games)
	mux.HandleFunc("POST /api/v1/games", a.createGame)
	mux.HandleFunc("GET /api/v1/games/{id}", a.game)
	mux.HandleFunc("PUT /api/v1/games/{id}/bindings", a.upsertBinding)
	mux.HandleFunc("POST /api/v1/games/{id}/restore", a.restore)
	mux.HandleFunc("POST /api/v1/games/{id}/snapshot", a.snapshot)
	mux.HandleFunc("POST /api/v1/games/{id}/artwork", a.uploadArtwork)
	mux.HandleFunc("GET /api/v1/endpoints", a.endpoints)
	mux.HandleFunc("POST /api/v1/endpoints/{id}/scan", a.scanEndpoint)
	mux.HandleFunc("POST /api/v1/endpoints/{id}/pause", a.pauseEndpoint)
	mux.HandleFunc("POST /api/v1/endpoints/{id}/resume", a.resumeEndpoint)
	mux.HandleFunc("GET /api/v1/profiles", a.profiles)
	mux.HandleFunc("GET /api/v1/activity", a.activity)
	mux.HandleFunc("GET /api/v1/conflicts", a.conflicts)
	mux.HandleFunc("POST /api/v1/conflicts/{id}/promote", a.promote)
	mux.HandleFunc("GET /api/v1/unassigned", a.unassigned)
	mux.HandleFunc("GET /api/v1/archive/usage", a.archiveUsage)
	mux.HandleFunc("GET /api/v1/diagnostics", a.diagnostics)
	mux.HandleFunc("GET /api/v1/onboarding", a.onboarding)
	mux.HandleFunc("POST /api/v1/onboarding/folders", a.configureFolders)
	mux.HandleFunc("POST /api/v1/onboarding/complete", a.completeOnboarding)
	mux.HandleFunc("POST /api/v1/settings/propagation", a.propagation)
	mux.HandleFunc("GET /api/v1/settings/emulators", a.emulatorSettings)
	mux.HandleFunc("PUT /api/v1/settings/emulators/windows-gba", a.configureWindowsGBAProfile)
	mux.HandleFunc("POST /api/v1/imports/retroarch", a.importRetroArch)
	mux.HandleFunc("POST /api/v1/imports/rom-hashes", a.importROMHashes)
	mux.HandleFunc("GET /api/v1/events", a.stream)
	return requestLogger(mux)
}

func (a *API) live(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "uptimeSeconds": int(time.Since(a.started).Seconds())})
}
func (a *API) ready(w http.ResponseWriter, r *http.Request) {
	if err := a.store.Ready(r.Context()); err != nil {
		writeError(w, http.StatusServiceUnavailable, "database not ready")
		return
	}
	for _, path := range a.requiredDirs {
		if err := ensureWritable(path); err != nil {
			writeError(w, http.StatusServiceUnavailable, "required storage is not writable")
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}

func (a *API) games(w http.ResponseWriter, r *http.Request) {
	platform := model.Platform(strings.ToLower(r.URL.Query().Get("platform")))
	items, err := a.store.ListGames(r.Context(), r.URL.Query().Get("search"), platform)
	if err != nil {
		internalError(w, err)
		return
	}
	result := make([]gameDTO, 0, len(items))
	for _, item := range items {
		detail, _ := a.store.GetGame(r.Context(), item.ID)
		result = append(result, toGameDTO(detail))
	}
	writeJSON(w, http.StatusOK, result)
}

func (a *API) createGame(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Title    string         `json:"title"`
		Platform model.Platform `json:"platform"`
		CRC32    string         `json:"crc32"`
		SHA1     string         `json:"sha1"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	game, err := a.store.CreateGame(r.Context(), req.Title, req.Platform, req.CRC32, req.SHA1)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, game)
}

func (a *API) game(w http.ResponseWriter, r *http.Request) {
	detail, err := a.store.GetGame(r.Context(), r.PathValue("id"))
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "game not found")
		return
	}
	if err != nil {
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toGameDetailDTO(detail))
}

func (a *API) upsertBinding(w http.ResponseWriter, r *http.Request) {
	var req struct {
		EndpointID     string `json:"endpointId"`
		ProfileID      string `json:"profileId"`
		RelativePath   string `json:"relativePath"`
		EmulatorClosed bool   `json:"emulatorClosed"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	existing, err := a.store.ListBindings(r.Context(), r.PathValue("id"))
	if err != nil {
		internalError(w, err)
		return
	}
	requestedPath := pathpkg.Clean(strings.ReplaceAll(strings.TrimSpace(req.RelativePath), "\\", "/"))
	var previous *model.SaveBinding
	for _, binding := range existing {
		if binding.EndpointID == req.EndpointID {
			copy := binding
			previous = &copy
			if (binding.ProfileID != req.ProfileID || binding.RelativePath != requestedPath) && !req.EmulatorClosed {
				writeError(w, http.StatusBadRequest, "confirm that all emulators are closed")
				return
			}
		}
	}
	preCaptured := false
	if previous != nil && (previous.ProfileID != req.ProfileID || previous.RelativePath != requestedPath) {
		result, _ := a.captureBindingNow(*previous, "Captured before emulator profile or path change")
		preCaptured = result.RevisionID != ""
	}
	binding, err := a.store.UpsertBinding(r.Context(), model.SaveBinding{GameID: r.PathValue("id"), EndpointID: req.EndpointID, ProfileID: req.ProfileID, RelativePath: req.RelativePath})
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if previous != nil && previous.ProfileID != binding.ProfileID && previous.RelativePath == binding.RelativePath && preCaptured {
		if err = a.rematerializeBindingNow(binding); err != nil {
			a.hub.Publish(events.Event{Type: "profile-reprocess-error", GameID: binding.GameID, Message: err.Error()})
		}
	} else if _, captureErr := a.captureBindingNow(binding, "Captured after manual save mapping or profile change"); captureErr != nil {
		a.broker.Schedule(context.Background(), broker.CaptureInput{EndpointID: binding.EndpointID, RelativePath: binding.RelativePath, Provenance: model.ProvenanceUnknown, Detail: "Retry after manual save mapping or profile change"})
	}
	writeJSON(w, http.StatusOK, binding)
}

func (a *API) captureBindingNow(binding model.SaveBinding, detail string) (store.IngestResult, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return a.broker.Capture(ctx, broker.CaptureInput{EndpointID: binding.EndpointID, RelativePath: binding.RelativePath, Provenance: model.ProvenanceUnknown, Detail: detail})
}

func (a *API) rematerializeBindingNow(binding model.SaveBinding) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return a.broker.RematerializeBinding(ctx, binding)
}

type mutationRequest struct {
	RevisionID     string `json:"revisionId"`
	ExpectedHeadID string `json:"expectedHeadId"`
	EmulatorClosed bool   `json:"emulatorClosed"`
	IdempotencyKey string `json:"idempotencyKey"`
}

func (a *API) restore(w http.ResponseWriter, r *http.Request) {
	var req mutationRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if !req.EmulatorClosed {
		writeError(w, http.StatusBadRequest, "confirm that all emulators are closed")
		return
	}
	key, ok := idempotencyKey(r, req.IdempotencyKey)
	if !ok {
		writeError(w, http.StatusBadRequest, "a matching Idempotency-Key is required")
		return
	}
	revision, err := a.broker.Restore(r.Context(), r.PathValue("id"), req.RevisionID, req.ExpectedHeadID, key, actor(r), "restore")
	if err != nil {
		mutationError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, revision)
}

func (a *API) snapshot(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ExpectedHeadID string `json:"expectedHeadId"`
		IdempotencyKey string `json:"idempotencyKey"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	key, ok := idempotencyKey(r, req.IdempotencyKey)
	if !ok {
		writeError(w, http.StatusBadRequest, "a matching Idempotency-Key is required")
		return
	}
	revision, err := a.store.CreateSnapshot(r.Context(), r.PathValue("id"), req.ExpectedHeadID, key, actor(r))
	if err != nil {
		mutationError(w, err)
		return
	}
	a.hub.Publish(events.Event{Type: "snapshot", GameID: r.PathValue("id"), Message: "Manual snapshot created"})
	writeJSON(w, http.StatusCreated, revision)
}

func (a *API) promote(w http.ResponseWriter, r *http.Request) {
	conflict, err := a.store.Conflict(r.Context(), r.PathValue("id"))
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "conflict not found")
		return
	}
	if err != nil {
		internalError(w, err)
		return
	}
	if conflict.State != "open" {
		writeError(w, http.StatusConflict, "conflict is already resolved")
		return
	}
	var req mutationRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if !req.EmulatorClosed {
		writeError(w, http.StatusBadRequest, "confirm that all emulators are closed")
		return
	}
	key, ok := idempotencyKey(r, req.IdempotencyKey)
	if !ok {
		writeError(w, http.StatusBadRequest, "a matching Idempotency-Key is required")
		return
	}
	if req.RevisionID == "" {
		req.RevisionID = conflict.BranchRevisionID
	}
	revision, err := a.broker.Restore(r.Context(), conflict.GameID, req.RevisionID, req.ExpectedHeadID, key, actor(r), "promote")
	if err != nil {
		mutationError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, revision)
}

func (a *API) endpoints(w http.ResponseWriter, r *http.Request) {
	items, err := a.store.ListEndpoints(r.Context())
	if err != nil {
		internalError(w, err)
		return
	}
	result := make([]endpointDTO, 0, len(items))
	for _, item := range items {
		result = append(result, toEndpointDTO(item))
	}
	writeJSON(w, http.StatusOK, result)
}
func (a *API) profiles(w http.ResponseWriter, r *http.Request) {
	items, err := a.store.Profiles(r.Context())
	if err != nil {
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, items)
}
func (a *API) scanEndpoint(w http.ResponseWriter, r *http.Request) {
	folder, ok := a.folderForEndpoint(r.PathValue("id"))
	if !ok {
		writeError(w, http.StatusNotFound, "endpoint not found")
		return
	}
	if err := a.syncthing.Scan(r.Context(), folder); err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "scan-requested"})
}
func (a *API) pauseEndpoint(w http.ResponseWriter, r *http.Request)  { a.setPause(w, r, true) }
func (a *API) resumeEndpoint(w http.ResponseWriter, r *http.Request) { a.setPause(w, r, false) }
func (a *API) setPause(w http.ResponseWriter, r *http.Request, paused bool) {
	folder, ok := a.folderForEndpoint(r.PathValue("id"))
	if !ok {
		writeError(w, http.StatusNotFound, "endpoint not found")
		return
	}
	if err := a.syncthing.SetFolderPaused(r.Context(), folder, paused); err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"paused": paused})
}
func (a *API) folderForEndpoint(id string) (string, bool) {
	switch id {
	case "thor":
		return a.thorFolderID, true
	case "windows":
		return a.windowsFolderID, true
	default:
		return "", false
	}
}

func (a *API) activity(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	items, err := a.store.ListActivity(r.Context(), limit)
	if err != nil {
		internalError(w, err)
		return
	}
	result := make([]activityDTO, 0, len(items))
	for _, item := range items {
		result = append(result, toActivityDTO(item))
	}
	writeJSON(w, http.StatusOK, result)
}

func (a *API) conflicts(w http.ResponseWriter, r *http.Request) {
	items, err := a.store.ListConflicts(r.Context(), r.URL.Query().Get("state"))
	if err != nil {
		internalError(w, err)
		return
	}
	result := make([]conflictDTO, 0, len(items))
	for _, item := range items {
		dto, err := a.toConflictDTO(r.Context(), item)
		if err == nil {
			result = append(result, dto)
		}
	}
	writeJSON(w, http.StatusOK, result)
}
func (a *API) unassigned(w http.ResponseWriter, r *http.Request) {
	items, err := a.store.ListUnassigned(r.Context())
	if err != nil {
		internalError(w, err)
		return
	}
	settings, err := a.store.EmulatorSettings(r.Context())
	if err != nil {
		internalError(w, err)
		return
	}
	result := make([]unassignedDTO, 0, len(items))
	for _, item := range items {
		result = append(result, toUnassignedDTO(item, settings.WindowsGBAProfileID))
	}
	writeJSON(w, http.StatusOK, result)
}

func (a *API) archiveUsage(w http.ResponseWriter, r *http.Request) {
	capacity, err := a.archive.Capacity()
	if err != nil {
		internalError(w, err)
		return
	}
	counts, err := a.store.Counts(r.Context())
	if err != nil {
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"usedBytes": capacity.UsedBytes, "quotaBytes": capacity.SoftQuotaBytes, "freeBytes": capacity.FreeBytes, "reserveBytes": capacity.ReserveBytes, "blobCount": counts.BlobCount, "revisionCount": counts.RevisionCount, "state": capacity.State})
}

func (a *API) diagnostics(w http.ResponseWriter, r *http.Request) {
	now := time.Now().UTC()
	checks := []diagnosticDTO{}
	dbState := "healthy"
	dbDetail := "SQLite schema and connection are ready"
	if err := a.store.Ready(r.Context()); err != nil {
		dbState = "degraded"
		dbDetail = err.Error()
	}
	checks = append(checks, diagnosticDTO{"database", "Database", dbDetail, dbState, now, 0})
	capacity, err := a.archive.Capacity()
	archiveState := "healthy"
	archiveDetail := "Archive is writable and has capacity"
	if err != nil {
		archiveState = "degraded"
		archiveDetail = err.Error()
	} else if capacity.State != "healthy" {
		archiveState = "degraded"
		archiveDetail = "Archive capacity is " + capacity.State
	}
	checks = append(checks, diagnosticDTO{"archive", "Revision archive", archiveDetail, archiveState, now, 0})
	start := time.Now()
	syncState := "healthy"
	syncDetail := "Syncthing API is reachable"
	if err := a.syncthing.Health(r.Context()); err != nil {
		syncState = "degraded"
		syncDetail = err.Error()
	}
	checks = append(checks, diagnosticDTO{"syncthing", "Syncthing", syncDetail, syncState, now, time.Since(start).Milliseconds()})
	if a.authCheck != nil {
		start = time.Now()
		authState := "healthy"
		authDetail := "Cloudflare Access signing keys are available"
		if err := a.authCheck(r.Context()); err != nil {
			authState = "degraded"
			authDetail = "Cloudflare signing keys are temporarily unavailable"
		}
		checks = append(checks, diagnosticDTO{"cloudflare-access", "Cloudflare Access", authDetail, authState, now, time.Since(start).Milliseconds()})
	}
	writeJSON(w, http.StatusOK, checks)
}

func (a *API) onboarding(w http.ResponseWriter, r *http.Request) {
	complete, _ := a.store.Setting(r.Context(), "onboarding_complete")
	propagation, _ := a.store.Setting(r.Context(), "propagation_enabled")
	endpoints, _ := a.store.ListEndpoints(r.Context())
	counts, _ := a.store.Counts(r.Context())
	storageWritable := true
	for _, path := range a.requiredDirs {
		if ensureWritable(path) != nil {
			storageWritable = false
			break
		}
	}
	syncConnected := a.syncthing.Health(r.Context()) == nil
	configured := len(endpoints) >= 2 && endpoints[0].DeviceID != "" && endpoints[1].DeviceID != ""
	step := 1
	if syncConnected {
		step = 2
	}
	if storageWritable {
		step = 3
	}
	if configured {
		step = 4
	}
	if counts.UnassignedCount == 0 {
		step = 5
	}
	if complete == "true" {
		step = 7
	}
	writeJSON(w, http.StatusOK, map[string]any{"complete": complete == "true", "currentStep": step, "syncthingConnected": syncConnected, "storageWritable": storageWritable, "endpointsConfigured": configured, "inventoryComplete": counts.UnassignedCount == 0, "propagationEnabled": propagation == "true"})
}

func (a *API) configureFolders(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ThorDeviceID    string `json:"thorDeviceId"`
		WindowsDeviceID string `json:"windowsDeviceId"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.ThorDeviceID == "" || req.WindowsDeviceID == "" {
		writeError(w, http.StatusBadRequest, "both Syncthing device IDs are required")
		return
	}
	if err := a.syncthing.EnsureFolder(r.Context(), a.thorFolderID, "ThorSync - Thor", "/var/syncthing/sync/thor", req.ThorDeviceID); err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	if err := a.syncthing.EnsureFolder(r.Context(), a.windowsFolderID, "ThorSync - Windows", "/var/syncthing/sync/windows", req.WindowsDeviceID); err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	_ = a.store.UpdateEndpoint(r.Context(), "thor", req.ThorDeviceID, "configured", false)
	_ = a.store.UpdateEndpoint(r.Context(), "windows", req.WindowsDeviceID, "configured", false)
	writeJSON(w, http.StatusOK, map[string]string{"status": "configured"})
}

func (a *API) completeOnboarding(w http.ResponseWriter, r *http.Request) {
	var req struct {
		EnableDelivery *bool `json:"enableDelivery"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	open, err := a.store.ListConflicts(r.Context(), "open")
	if err != nil {
		internalError(w, err)
		return
	}
	if len(open) > 0 {
		writeError(w, http.StatusConflict, "resolve imported save conflicts before enabling propagation")
		return
	}
	if err = a.store.SetSetting(r.Context(), "onboarding_complete", "true"); err != nil {
		internalError(w, err)
		return
	}
	enabled := req.EnableDelivery == nil || *req.EnableDelivery
	propagationValue := "false"
	if enabled {
		propagationValue = "true"
	}
	if err = a.store.SetSetting(r.Context(), "propagation_enabled", propagationValue); err != nil {
		internalError(w, err)
		return
	}
	a.hub.Publish(events.Event{Type: "health.updated", Message: "Onboarding complete"})
	writeJSON(w, http.StatusOK, map[string]bool{"complete": true, "propagationEnabled": enabled})
}

func (a *API) propagation(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Enabled bool `json:"enabled"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.Enabled {
		open, err := a.store.ListConflicts(r.Context(), "open")
		if err != nil {
			internalError(w, err)
			return
		}
		if len(open) > 0 {
			writeError(w, http.StatusConflict, "resolve conflicts before enabling propagation")
			return
		}
	}
	value := "false"
	if req.Enabled {
		value = "true"
	}
	if err := a.store.SetSetting(r.Context(), "propagation_enabled", value); err != nil {
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"enabled": req.Enabled})
}

func (a *API) emulatorSettings(w http.ResponseWriter, r *http.Request) {
	settings, err := a.store.EmulatorSettings(r.Context())
	if err != nil {
		internalError(w, err)
		return
	}
	unassigned, err := a.store.ListUnassigned(r.Context())
	if err != nil {
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toEmulatorSettingsDTO(settings, unassigned))
}

func (a *API) configureWindowsGBAProfile(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ProfileID       string `json:"profileId"`
		EmulatorClosed  bool   `json:"emulatorClosed"`
		ApplyToExisting bool   `json:"applyToExisting"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if !req.EmulatorClosed {
		writeError(w, http.StatusBadRequest, "confirm that all emulators are closed")
		return
	}
	if req.ProfileID != "windows-mgba" && req.ProfileID != "windows-vbam" {
		writeError(w, http.StatusBadRequest, "profileId must be windows-mgba or windows-vbam")
		return
	}
	before, err := a.store.ListBindingsByPlatformEndpoint(r.Context(), model.PlatformGBA, "windows")
	if err != nil {
		internalError(w, err)
		return
	}
	preCaptured := map[string]bool{}
	if req.ApplyToExisting {
		for _, binding := range before {
			if binding.ProfileID == req.ProfileID {
				continue
			}
			result, _ := a.captureBindingNow(binding, "Captured before Windows GBA emulator profile change")
			preCaptured[binding.ID] = result.RevisionID != ""
		}
	}
	_, err = a.store.ConfigureWindowsGBAProfile(r.Context(), req.ProfileID, req.ApplyToExisting)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	bindings, err := a.store.ListBindingsByPlatformEndpoint(r.Context(), model.PlatformGBA, "windows")
	if err != nil {
		internalError(w, err)
		return
	}
	beforeByID := map[string]model.SaveBinding{}
	for _, binding := range before {
		beforeByID[binding.ID] = binding
	}
	if req.ApplyToExisting {
		for _, binding := range bindings {
			previous, existed := beforeByID[binding.ID]
			profileChanged := existed && previous.ProfileID != binding.ProfileID
			if profileChanged && preCaptured[binding.ID] {
				if rematerializeErr := a.rematerializeBindingNow(binding); rematerializeErr != nil {
					a.hub.Publish(events.Event{Type: "profile-reprocess-error", GameID: binding.GameID, Message: rematerializeErr.Error()})
				}
				continue
			}
			if _, captureErr := a.captureBindingNow(binding, "Reprocessed after Windows GBA emulator profile change"); captureErr != nil {
				a.broker.Schedule(context.Background(), broker.CaptureInput{EndpointID: binding.EndpointID, RelativePath: binding.RelativePath, Provenance: model.ProvenanceUnknown, Detail: "Retry after Windows GBA emulator profile change"})
			}
		}
	}
	settings, err := a.store.EmulatorSettings(r.Context())
	if err != nil {
		internalError(w, err)
		return
	}
	unassigned, err := a.store.ListUnassigned(r.Context())
	if err != nil {
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toEmulatorSettingsDTO(settings, unassigned))
}

func (a *API) importRetroArch(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(8 << 20); err != nil {
		writeError(w, http.StatusBadRequest, "invalid multipart upload")
		return
	}
	file, _, err := r.FormFile("playlist")
	if err != nil {
		writeError(w, http.StatusBadRequest, "playlist file is required")
		return
	}
	defer file.Close()
	var playlist struct {
		Items []struct {
			Path   string `json:"path"`
			Label  string `json:"label"`
			CRC32  string `json:"crc32"`
			DBName string `json:"db_name"`
		} `json:"items"`
	}
	if err = json.NewDecoder(io.LimitReader(file, 8<<20)).Decode(&playlist); err != nil {
		writeError(w, http.StatusBadRequest, "invalid RetroArch playlist")
		return
	}
	created := 0
	for _, item := range playlist.Items {
		platform := platformFrom(item.Path, item.DBName)
		if platform == "" {
			continue
		}
		crc := strings.TrimSpace(strings.Split(item.CRC32, "|")[0])
		title := strings.TrimSpace(item.Label)
		if entry, ok := catalog.Lookup(string(platform), crc, ""); ok {
			title = entry.Title
		}
		if title == "" {
			title = strings.TrimSuffix(filepath.Base(item.Path), filepath.Ext(item.Path))
		}
		if _, err := a.store.CreateGame(r.Context(), title, platform, crc, ""); err == nil {
			created++
		}
	}
	writeJSON(w, http.StatusOK, map[string]int{"imported": created})
}

func (a *API) importROMHashes(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Records []struct {
			Filename string `json:"filename"`
			Size     int64  `json:"size"`
			CRC32    string `json:"crc32"`
			SHA1     string `json:"sha1"`
		} `json:"records"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	created := 0
	for _, record := range req.Records {
		platform := platformFrom(record.Filename, "")
		if platform == "" {
			continue
		}
		title := strings.TrimSuffix(filepath.Base(record.Filename), filepath.Ext(record.Filename))
		if entry, ok := catalog.Lookup(string(platform), record.CRC32, record.SHA1); ok {
			title = entry.Title
		}
		if _, err := a.store.CreateGame(r.Context(), title, platform, record.CRC32, record.SHA1); err == nil {
			created++
		}
	}
	writeJSON(w, http.StatusOK, map[string]int{"imported": created})
}

func platformFrom(path, db string) model.Platform {
	lower := strings.ToLower(path + " " + db)
	if strings.Contains(lower, "game boy advance") || strings.HasSuffix(strings.ToLower(path), ".gba") {
		return model.PlatformGBA
	}
	if strings.Contains(lower, "nintendo ds") || strings.HasSuffix(strings.ToLower(path), ".nds") {
		return model.PlatformNDS
	}
	return ""
}

func (a *API) uploadArtwork(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(6 << 20); err != nil {
		writeError(w, http.StatusBadRequest, "invalid artwork upload")
		return
	}
	file, header, err := r.FormFile("artwork")
	if err != nil {
		writeError(w, http.StatusBadRequest, "artwork file is required")
		return
	}
	defer file.Close()
	path, err := saveArtwork(a.artworkDir, r.PathValue("id"), file, header)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err = a.store.SetArtwork(r.Context(), r.PathValue("id"), path); err != nil {
		_ = os.Remove(path)
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "game not found")
			return
		}
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"artworkUrl": "/artwork/" + filepath.Base(path)})
}

func saveArtwork(dir, gameID string, file multipart.File, header *multipart.FileHeader) (string, error) {
	if !validObjectID(gameID) {
		return "", errors.New("invalid game ID")
	}
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return "", err
	}
	prefix := make([]byte, 512)
	n, err := io.ReadFull(file, prefix)
	if err != nil && err != io.ErrUnexpectedEOF {
		return "", err
	}
	mime := http.DetectContentType(prefix[:n])
	extensions := map[string]string{"image/png": ".png", "image/jpeg": ".jpg", "image/webp": ".webp"}
	ext, ok := extensions[mime]
	if !ok {
		return "", fmt.Errorf("artwork must be PNG, JPEG, or WebP")
	}
	if header.Size > 5<<20 {
		return "", fmt.Errorf("artwork exceeds 5 MiB")
	}
	if _, err = file.Seek(0, io.SeekStart); err != nil {
		return "", err
	}
	temp := filepath.Join(dir, "."+ids.New()+".tmp")
	out, err := os.OpenFile(temp, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o640)
	if err != nil {
		return "", err
	}
	written, copyErr := io.Copy(out, io.LimitReader(file, (5<<20)+1))
	syncErr := out.Sync()
	closeErr := out.Close()
	if copyErr != nil || syncErr != nil || closeErr != nil || written > 5<<20 {
		_ = os.Remove(temp)
		return "", fmt.Errorf("could not store artwork safely")
	}
	target := filepath.Join(dir, gameID+ext)
	if err = os.Rename(temp, target); err != nil {
		_ = os.Remove(temp)
		return "", err
	}
	return target, nil
}

func validObjectID(value string) bool {
	if len(value) != 32 {
		return false
	}
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == 16 && strings.ToLower(value) == value
}

func (a *API) stream(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming unsupported")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache, no-store")
	w.Header().Set("X-Accel-Buffering", "no")
	channel, unsubscribe := a.hub.Subscribe()
	defer unsubscribe()
	_, _ = fmt.Fprint(w, "event: ready\ndata: {\"type\":\"health.updated\"}\n\n")
	flusher.Flush()
	ticker := time.NewTicker(20 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case payload := <-channel:
			_, _ = fmt.Fprintf(w, "data: %s\n\n", payload)
			flusher.Flush()
		case <-ticker.C:
			_, _ = fmt.Fprint(w, ": keepalive\n\n")
			flusher.Flush()
		}
	}
}

func actor(r *http.Request) string {
	if principal, ok := auth.PrincipalFromContext(r.Context()); ok {
		return principal.Email
	}
	return "local-admin"
}
func idempotencyKey(r *http.Request, body string) (string, bool) {
	header := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	body = strings.TrimSpace(body)
	return header, header != "" && (body == "" || body == header)
}
func mutationError(w http.ResponseWriter, err error) {
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, err.Error())
	} else if errors.Is(err, store.ErrConflict) {
		writeError(w, http.StatusConflict, err.Error())
	} else {
		internalError(w, err)
	}
}
func ensureWritable(path string) error {
	if err := os.MkdirAll(path, 0o750); err != nil {
		return err
	}
	probe := filepath.Join(path, ".write-probe-"+ids.New())
	if err := os.WriteFile(probe, []byte("ok"), 0o600); err != nil {
		return err
	}
	return os.Remove(probe)
}
func decodeJSON(w http.ResponseWriter, r *http.Request, target any) bool {
	decoder := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON request")
		return false
	}
	return true
}
func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
func internalError(w http.ResponseWriter, err error) {
	slog.Error("request failed", "error", err)
	writeError(w, http.StatusInternalServerError, "internal server error")
}
func requestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		slog.Debug("http request", "method", r.Method, "path", r.URL.Path, "duration", time.Since(start))
	})
}

// ShutdownDependencyCheck is useful to callers that want a bounded readiness
// probe without coupling restarts to temporary dependency failures.
func (a *API) ShutdownDependencyCheck(ctx context.Context) error { return a.store.Ready(ctx) }
