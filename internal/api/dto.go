package api

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/apgul/thorsync/internal/adapter"
	"github.com/apgul/thorsync/internal/model"
	"github.com/apgul/thorsync/internal/store"
)

type gameDTO struct {
	ID                string    `json:"id"`
	Title             string    `json:"title"`
	Platform          string    `json:"platform"`
	SaveName          string    `json:"saveName"`
	Emulator          string    `json:"emulator"`
	UpdatedAt         time.Time `json:"updatedAt"`
	SourceDeviceID    string    `json:"sourceDeviceId"`
	SourceDeviceName  string    `json:"sourceDeviceName"`
	DeliveryState     string    `json:"deliveryState"`
	CurrentRevisionID string    `json:"currentRevisionId"`
	RevisionCount     int       `json:"revisionCount"`
	HasConflict       bool      `json:"hasConflict"`
	HealthMessage     string    `json:"healthMessage,omitempty"`
	Accent            string    `json:"accent"`
	ArtworkURL        string    `json:"artworkUrl,omitempty"`
}
type gameDetailDTO struct {
	gameDTO
	Revisions []revisionDTO `json:"revisions"`
	Bindings  []bindingDTO  `json:"bindings"`
}
type revisionDTO struct {
	ID               string           `json:"id"`
	GameID           string           `json:"gameId"`
	ShortHash        string           `json:"shortHash"`
	SourceDeviceID   string           `json:"sourceDeviceId"`
	SourceDeviceName string           `json:"sourceDeviceName"`
	SourceModifiedAt time.Time        `json:"sourceModifiedAt"`
	ObservedAt       time.Time        `json:"observedAt"`
	Size             int64            `json:"size"`
	Provenance       model.Provenance `json:"provenance"`
	Kind             string           `json:"kind"`
	State            string           `json:"state"`
	Note             string           `json:"note,omitempty"`
}
type bindingDTO struct {
	ID                 string     `json:"id"`
	EndpointID         string     `json:"endpointId"`
	EndpointName       string     `json:"endpointName"`
	RelativePath       string     `json:"relativePath"`
	Extension          string     `json:"extension"`
	BaselineRevisionID string     `json:"baselineRevisionId"`
	DeliveryState      string     `json:"deliveryState"`
	LastDeliveredAt    *time.Time `json:"lastDeliveredAt"`
	ProfileID          string     `json:"profileId"`
	ProfileName        string     `json:"profileName"`
	Format             string     `json:"format"`
	HasRTC             bool       `json:"hasRtc"`
}
type emulatorSettingsDTO struct {
	WindowsGBAProfileID string `json:"windowsGbaProfileId"`
	Configured          bool   `json:"configured"`
	DetectedProfileID   string `json:"detectedProfileId,omitempty"`
	AffectedBindings    int    `json:"affectedBindings"`
}
type unassignedDTO struct {
	ID                   string           `json:"id"`
	EndpointID           string           `json:"endpointId"`
	RelativePath         string           `json:"relativePath"`
	BlobHash             string           `json:"blobHash,omitempty"`
	Size                 int64            `json:"size"`
	SourceModifiedAt     *time.Time       `json:"sourceModifiedAt,omitempty"`
	ObservedAt           time.Time        `json:"observedAt"`
	Provenance           model.Provenance `json:"provenance"`
	State                string           `json:"state"`
	Detail               string           `json:"detail,omitempty"`
	SuggestedProfileID   string           `json:"suggestedProfileId,omitempty"`
	CompatibleProfileIDs []string         `json:"compatibleProfileIds"`
	ReviewOnly           bool             `json:"reviewOnly"`
}
type endpointDTO struct {
	ID           string     `json:"id"`
	Name         string     `json:"name"`
	Kind         string     `json:"kind"`
	Profile      string     `json:"profile"`
	Folder       string     `json:"folder"`
	Status       string     `json:"status"`
	LastSeenAt   *time.Time `json:"lastSeenAt"`
	PendingFiles int        `json:"pendingFiles"`
	Version      string     `json:"version,omitempty"`
}
type activityDTO struct {
	ID         string    `json:"id"`
	Type       string    `json:"type"`
	Title      string    `json:"title"`
	Detail     string    `json:"detail"`
	OccurredAt time.Time `json:"occurredAt"`
	GameID     string    `json:"gameId,omitempty"`
	GameTitle  string    `json:"gameTitle,omitempty"`
	DeviceName string    `json:"deviceName,omitempty"`
	Tone       string    `json:"tone"`
}
type conflictDTO struct {
	ID          string      `json:"id"`
	GameID      string      `json:"gameId"`
	GameTitle   string      `json:"gameTitle"`
	Platform    string      `json:"platform"`
	OpenedAt    time.Time   `json:"openedAt"`
	Reason      string      `json:"reason"`
	CurrentHead revisionDTO `json:"currentHead"`
	Branch      revisionDTO `json:"branch"`
}
type diagnosticDTO struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Detail    string    `json:"detail"`
	State     string    `json:"state"`
	CheckedAt time.Time `json:"checkedAt"`
	LatencyMS int64     `json:"latencyMs,omitempty"`
}

func toGameDTO(detail model.GameDetail) gameDTO {
	updated := detail.UpdatedAt
	if detail.ObservedAt != nil {
		updated = *detail.ObservedAt
	}
	saveName := "Not mapped"
	emulator := "Awaiting profile"
	if len(detail.Bindings) > 0 {
		saveName = filepath.Base(detail.Bindings[0].RelativePath)
		emulator = detail.Bindings[0].ProfileID
	}
	delivery, health := gameDelivery(detail)
	sourceName := endpointName(detail.SourceEndpointID)
	accent := "#e5b95c"
	if detail.Platform == model.PlatformNDS {
		accent = "#75a8ff"
	}
	return gameDTO{ID: detail.ID, Title: detail.Title, Platform: strings.ToUpper(string(detail.Platform)), SaveName: saveName, Emulator: emulator, UpdatedAt: updated, SourceDeviceID: detail.SourceEndpointID, SourceDeviceName: sourceName, DeliveryState: delivery, CurrentRevisionID: detail.CurrentRevisionID, RevisionCount: len(detail.Revisions), HasConflict: detail.ConflictCount > 0, HealthMessage: health, Accent: accent, ArtworkURL: detail.ArtworkURL}
}
func toGameDetailDTO(detail model.GameDetail) gameDetailDTO {
	result := gameDetailDTO{gameDTO: toGameDTO(detail), Revisions: []revisionDTO{}, Bindings: []bindingDTO{}}
	for _, revision := range detail.Revisions {
		result.Revisions = append(result.Revisions, toRevisionDTO(revision))
	}
	for _, binding := range detail.Bindings {
		ext := strings.ToLower(filepath.Ext(binding.RelativePath))
		profileName := binding.ProfileID
		format := ""
		if profile, ok := adapter.Profile(binding.ProfileID); ok {
			profileName = profile.Name
			format = profile.Format
		}
		hasRTC := bindingRevisionHasRTC(detail, binding)
		state := "paused"
		var deliveredAt *time.Time
		latestAction := latestObservationAction(detail, binding.EndpointID, binding.RelativePath)
		if latestAction == "missing" {
			state = "missing"
		} else if latestAction == "quarantined" {
			state = "paused"
		} else if binding.LastDeployedRevisionID != "" && binding.LastDeployedRevisionID == detail.CurrentRevisionID {
			state = "delivered"
		} else {
			for _, operation := range detail.Operations {
				if operation.TargetEndpointID != binding.EndpointID || operation.RelativePath != binding.RelativePath || operation.RevisionID != detail.CurrentRevisionID {
					continue
				}
				if operation.State == "pending" || operation.State == "written" {
					state = "syncing"
				}
				if operation.State == "delivered" {
					value := operation.UpdatedAt
					deliveredAt = &value
				}
				break
			}
		}
		result.Bindings = append(result.Bindings, bindingDTO{ID: binding.ID, EndpointID: binding.EndpointID, EndpointName: endpointName(binding.EndpointID), RelativePath: binding.RelativePath, Extension: ext, BaselineRevisionID: binding.LastDeployedRevisionID, DeliveryState: state, LastDeliveredAt: deliveredAt, ProfileID: binding.ProfileID, ProfileName: profileName, Format: format, HasRTC: hasRTC})
	}
	return result
}

func bindingRevisionHasRTC(detail model.GameDetail, binding model.SaveBinding) bool {
	revisionID := binding.LastDeployedRevisionID
	if revisionID != "" {
		for _, revision := range detail.Revisions {
			if revision.ID == revisionID {
				return revision.HasRTC
			}
		}
	}
	for _, revision := range detail.Revisions {
		if revision.ID == detail.CurrentRevisionID {
			return revision.HasRTC
		}
	}
	return false
}

func toEmulatorSettingsDTO(settings model.EmulatorSettings, unassigned []store.UnassignedFile) emulatorSettingsDTO {
	return emulatorSettingsDTO{
		WindowsGBAProfileID: settings.WindowsGBAProfileID,
		Configured:          settings.WindowsGBAConfigured,
		DetectedProfileID:   detectedWindowsGBAProfile(unassigned),
		AffectedBindings:    settings.AffectedBindings,
	}
}

func toUnassignedDTO(item store.UnassignedFile, windowsGBAProfileID string) unassignedDTO {
	reviewOnly := strings.HasPrefix(item.Detail, "Quarantined:")
	result := unassignedDTO{
		ID:                   item.ID,
		EndpointID:           item.EndpointID,
		RelativePath:         item.RelativePath,
		BlobHash:             item.BlobHash,
		Size:                 item.Size,
		SourceModifiedAt:     item.SourceModifiedAt,
		ObservedAt:           item.ObservedAt,
		Provenance:           item.Provenance,
		State:                item.State,
		Detail:               item.Detail,
		CompatibleProfileIDs: []string{},
		ReviewOnly:           reviewOnly,
	}
	matches := adapter.MatchingProfiles(item.EndpointID, model.PlatformGBA, item.RelativePath, item.Size)
	matches = append(matches, adapter.MatchingProfiles(item.EndpointID, model.PlatformNDS, item.RelativePath, item.Size)...)
	for _, profile := range matches {
		result.CompatibleProfileIDs = append(result.CompatibleProfileIDs, profile.ID)
	}
	if reviewOnly {
		result.CompatibleProfileIDs = []string{}
		return result
	}
	if len(matches) == 1 {
		result.SuggestedProfileID = matches[0].ID
	} else if item.EndpointID == "windows" {
		for _, profile := range matches {
			if profile.ID == windowsGBAProfileID {
				result.SuggestedProfileID = windowsGBAProfileID
				break
			}
		}
	}
	return result
}

func detectedWindowsGBAProfile(items []store.UnassignedFile) string {
	detected := ""
	for _, item := range items {
		if item.EndpointID != "windows" {
			continue
		}
		matches := adapter.MatchingProfiles(item.EndpointID, model.PlatformGBA, item.RelativePath, item.Size)
		if len(matches) != 1 {
			continue
		}
		if detected != "" && detected != matches[0].ID {
			return ""
		}
		detected = matches[0].ID
	}
	return detected
}

func gameDelivery(detail model.GameDetail) (string, string) {
	if detail.ConflictCount > 0 || detail.Status == "conflict" {
		return "paused", "Divergent save histories need resolution"
	}
	for _, binding := range detail.Bindings {
		switch latestObservationAction(detail, binding.EndpointID, binding.RelativePath) {
		case "missing":
			return "missing", "A mapped save is missing from a device"
		case "quarantined":
			return "paused", "An incomplete device save was archived; delivery is paused"
		}
	}
	for _, operation := range detail.Operations {
		if operation.RevisionID != detail.CurrentRevisionID {
			continue
		}
		switch operation.State {
		case "failed":
			return "paused", "Delivery failed and is safe to retry"
		case "pending", "written":
			return "syncing", "Waiting for Syncthing to confirm device delivery"
		}
	}
	if detail.CurrentRevisionID == "" {
		return "paused", "No save revision captured yet"
	}
	if len(detail.Bindings) == 0 {
		return "paused", "No device save paths are mapped"
	}
	for _, binding := range detail.Bindings {
		if binding.LastDeployedRevisionID != detail.CurrentRevisionID {
			return "paused", "Automatic delivery is paused or awaiting setup"
		}
	}
	return "delivered", "Current revision delivered to every mapped device"
}

func latestObservationAction(detail model.GameDetail, endpointID, path string) string {
	for _, observation := range detail.Observations {
		if observation.EndpointID == endpointID && observation.RelativePath == path {
			return observation.Action
		}
	}
	return ""
}
func toRevisionDTO(item model.Revision) revisionDTO {
	modified := item.ObservedAt
	if item.SourceModifiedAt != nil {
		modified = *item.SourceModifiedAt
	}
	hash := item.ContentHash
	if hash == "" {
		hash = item.BlobHash
	}
	if len(hash) > 12 {
		hash = hash[:12]
	}
	kind := item.Kind
	switch kind {
	case "captured":
		kind = "capture"
	case "promote":
		kind = "promotion"
	}
	return revisionDTO{ID: item.ID, GameID: item.GameID, ShortHash: hash, SourceDeviceID: item.SourceEndpointID, SourceDeviceName: endpointName(item.SourceEndpointID), SourceModifiedAt: modified, ObservedAt: item.ObservedAt, Size: item.Size, Provenance: item.Provenance, Kind: kind, State: item.State}
}
func toEndpointDTO(item model.Endpoint) endpointDTO {
	status := "offline"
	if item.Online {
		status = "healthy"
	} else if item.State != "offline" && item.State != "unknown" {
		status = "degraded"
	}
	return endpointDTO{ID: item.ID, Name: item.Name, Kind: item.ID, Profile: profileSummary(item.ID), Folder: item.FolderID, Status: status, LastSeenAt: item.LastSeenAt, PendingFiles: 0}
}
func toActivityDTO(item model.Activity) activityDTO {
	kind := item.Kind
	if kind == "revision" {
		kind = "capture"
	}
	tone := "neutral"
	switch kind {
	case "delivery", "restore", "snapshot":
		tone = "positive"
	case "conflict", "missing", "quarantine":
		tone = "warning"
	case "error":
		tone = "danger"
	}
	return activityDTO{ID: item.ID, Type: kind, Title: activityTitle(kind), Detail: item.Summary, OccurredAt: item.CreatedAt, GameID: item.GameID, GameTitle: item.GameTitle, DeviceName: endpointName(item.EndpointID), Tone: tone}
}
func (a *API) toConflictDTO(ctx context.Context, item model.Conflict) (conflictDTO, error) {
	detail, err := a.store.GetGame(ctx, item.GameID)
	if err != nil {
		return conflictDTO{}, err
	}
	var head, branch model.Revision
	for _, revision := range detail.Revisions {
		if revision.ID == item.HeadRevisionID {
			head = revision
		}
		if revision.ID == item.BranchRevisionID {
			branch = revision
		}
	}
	return conflictDTO{ID: item.ID, GameID: item.GameID, GameTitle: detail.Title, Platform: strings.ToUpper(string(detail.Platform)), OpenedAt: item.CreatedAt, Reason: item.Reason, CurrentHead: toRevisionDTO(head), Branch: toRevisionDTO(branch)}, nil
}
func endpointName(id string) string {
	switch id {
	case "thor":
		return "AYN Thor"
	case "windows":
		return "Windows PC"
	case "":
		return "ThorSync server"
	default:
		return id
	}
}
func profileSummary(id string) string {
	if id == "thor" {
		return "RetroArch mGBA / melonDS DS"
	}
	return "mGBA / VBA-M / melonDS"
}
func activityTitle(kind string) string {
	switch kind {
	case "capture":
		return "Save captured"
	case "delivery":
		return "Save delivered"
	case "conflict":
		return "Conflict needs attention"
	case "restore":
		return "Revision restored"
	case "snapshot":
		return "Manual snapshot created"
	case "missing":
		return "Save missing"
	case "quarantine":
		return "File quarantined"
	default:
		return fmt.Sprintf("ThorSync %s", kind)
	}
}
