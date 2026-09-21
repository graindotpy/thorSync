package adapter

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/apgul/thorsync/internal/model"
)

var allowedSizes = map[model.Platform]map[int64]struct{}{
	model.PlatformGBA: {
		512: {}, 8 * 1024: {}, 32 * 1024: {}, 64 * 1024: {}, 128 * 1024: {},
	},
	model.PlatformNDS: {
		512: {}, 8 * 1024: {}, 64 * 1024: {}, 128 * 1024: {}, 256 * 1024: {},
		512 * 1024: {}, 1024 * 1024: {}, 2 * 1024 * 1024: {}, 4 * 1024 * 1024: {}, 8 * 1024 * 1024: {},
	},
}

var profiles = map[string]model.EmulatorProfile{
	"thor-mgba":       {ID: "thor-mgba", Name: "RetroArch mGBA", EndpointID: "thor", Platform: model.PlatformGBA, Extension: ".srm", Format: "raw-battery"},
	"thor-melonds-ds": {ID: "thor-melonds-ds", Name: "RetroArch melonDS DS", EndpointID: "thor", Platform: model.PlatformNDS, Extension: ".srm", Format: "raw-battery"},
	"windows-vbam":    {ID: "windows-vbam", Name: "VisualBoyAdvance-M", EndpointID: "windows", Platform: model.PlatformGBA, Extension: ".sav", Format: "raw-battery"},
	"windows-mgba":    {ID: "windows-mgba", Name: "mGBA", EndpointID: "windows", Platform: model.PlatformGBA, Extension: ".sav", Format: "raw-battery+opaque-rtc"},
	"windows-melonds": {ID: "windows-melonds", Name: "melonDS", EndpointID: "windows", Platform: model.PlatformNDS, Extension: ".sav", Format: "raw-battery"},
}

var profileOrder = []string{"thor-mgba", "thor-melonds-ds", "windows-vbam", "windows-mgba", "windows-melonds"}

const gbaRTCSize = 16

// Save is the canonical, emulator-independent representation of a GBA save.
// RTC is deliberately opaque: adapters only separate, retain, and reattach it.
type Save struct {
	Battery []byte
	RTC     []byte
}

func Profiles() []model.EmulatorProfile {
	result := make([]model.EmulatorProfile, 0, len(profiles))
	for _, id := range profileOrder {
		result = append(result, profiles[id])
	}
	return result
}

func Profile(id string) (model.EmulatorProfile, bool) {
	profile, ok := profiles[id]
	return profile, ok
}

func Validate(profileID string, platform model.Platform, relativePath string, size int64) error {
	profile, err := validateRequest(profileID, platform, relativePath)
	if err != nil {
		return err
	}
	if isAllowedSize(platform, size) {
		return nil
	}
	if profile.ID == "windows-mgba" && isAllowedSize(platform, size-gbaRTCSize) {
		return nil
	}
	if _, ok := allowedSizes[platform]; !ok {
		return fmt.Errorf("unsupported platform %q", platform)
	}
	if !isAllowedSize(platform, size) {
		return fmt.Errorf("unsupported %s save size: %d bytes", platform, size)
	}
	return nil
}

// MatchingProfiles returns every profile which can own the supplied save,
// preserving the stable order used by Profiles. A raw Windows GBA .sav is
// intentionally ambiguous between VBA-M and mGBA; callers must not guess.
func MatchingProfiles(endpointID string, platform model.Platform, relativePath string, size int64) []model.EmulatorProfile {
	var result []model.EmulatorProfile
	for _, profile := range Profiles() {
		if profile.EndpointID != endpointID || profile.Platform != platform {
			continue
		}
		if Validate(profile.ID, platform, relativePath, size) == nil {
			result = append(result, profile)
		}
	}
	return result
}

// SuggestProfile returns a profile only when the endpoint, platform, path, and
// size identify exactly one. It refuses ambiguous raw Windows GBA saves.
func SuggestProfile(endpointID string, platform model.Platform, relativePath string, size int64) (model.EmulatorProfile, bool) {
	matches := MatchingProfiles(endpointID, platform, relativePath, size)
	if len(matches) != 1 {
		return model.EmulatorProfile{}, false
	}
	return matches[0], true
}

// Decode converts a validated GBA save into independent battery and RTC byte
// slices. Standalone mGBA's optional 16-byte RTC footer is never interpreted.
func Decode(profileID string, platform model.Platform, relativePath string, data []byte) (Save, error) {
	if platform != model.PlatformGBA {
		return Save{}, fmt.Errorf("save codec does not support platform %s", platform)
	}
	if err := Validate(profileID, platform, relativePath, int64(len(data))); err != nil {
		return Save{}, err
	}

	batteryEnd := len(data)
	if profileID == "windows-mgba" && !isAllowedSize(model.PlatformGBA, int64(len(data))) {
		batteryEnd -= gbaRTCSize
	}
	decoded := Save{Battery: append([]byte(nil), data[:batteryEnd]...)}
	if batteryEnd != len(data) {
		decoded.RTC = append([]byte(nil), data[batteryEnd:]...)
	}
	return decoded, nil
}

// Encode renders a canonical GBA save for a target profile. Raw profiles emit
// only battery bytes. Standalone mGBA appends RTC when the canonical save has
// one, and otherwise emits a valid raw save.
func Encode(profileID string, platform model.Platform, relativePath string, save Save) ([]byte, error) {
	profile, err := validateRequest(profileID, platform, relativePath)
	if err != nil {
		return nil, err
	}
	if platform != model.PlatformGBA {
		return nil, fmt.Errorf("save codec does not support platform %s", platform)
	}
	if !isAllowedSize(model.PlatformGBA, int64(len(save.Battery))) {
		return nil, fmt.Errorf("unsupported %s battery size: %d bytes", platform, len(save.Battery))
	}
	if len(save.RTC) != 0 && len(save.RTC) != gbaRTCSize {
		return nil, fmt.Errorf("unsupported GBA RTC size: %d bytes", len(save.RTC))
	}

	capacity := len(save.Battery)
	if profile.ID == "windows-mgba" {
		capacity += len(save.RTC)
	}
	encoded := make([]byte, 0, capacity)
	encoded = append(encoded, save.Battery...)
	if profile.ID == "windows-mgba" {
		encoded = append(encoded, save.RTC...)
	}
	return encoded, nil
}

func validateRequest(profileID string, platform model.Platform, relativePath string) (model.EmulatorProfile, error) {
	profile, ok := Profile(profileID)
	if !ok {
		return model.EmulatorProfile{}, fmt.Errorf("unsupported emulator profile %q", profileID)
	}
	if profile.Platform != platform {
		return model.EmulatorProfile{}, fmt.Errorf("profile %s does not support platform %s", profileID, platform)
	}
	if strings.ToLower(filepath.Ext(relativePath)) != profile.Extension {
		return model.EmulatorProfile{}, fmt.Errorf("%s expects %s saves", profile.Name, profile.Extension)
	}
	return profile, nil
}

func isAllowedSize(platform model.Platform, size int64) bool {
	_, ok := allowedSizes[platform][size]
	return ok
}

func IsSavePath(path string) bool {
	lower := strings.ToLower(path)
	if strings.Contains(lower, ".sync-conflict-") {
		return true
	}
	ext := strings.ToLower(filepath.Ext(lower))
	return ext == ".srm" || ext == ".sav"
}

// OriginalConflictPath maps Syncthing's conflict artifact name back to the
// configured live save path while preserving directories and the extension.
// For example game.sync-conflict-20260920-120000-ABC1234.sav becomes game.sav.
func OriginalConflictPath(path string) (string, bool) {
	normalized := filepath.ToSlash(path)
	lower := strings.ToLower(normalized)
	marker := strings.LastIndex(lower, ".sync-conflict-")
	if marker < 0 {
		return "", false
	}
	ext := filepath.Ext(normalized)
	if ext == "" || marker >= len(normalized)-len(ext) {
		return "", false
	}
	original := normalized[:marker] + ext
	if original == ext || strings.HasSuffix(original, "/"+ext) {
		return "", false
	}
	return original, true
}

// IsUnexpectedSidecar identifies files which commonly accompany battery
// saves but which ThorSync must never deliver as if they were save RAM. They
// are surfaced for review instead of being silently ignored.
func IsUnexpectedSidecar(path string) bool {
	lower := strings.ToLower(filepath.ToSlash(path))
	if strings.Contains(lower, ".sync-conflict-") {
		return false
	}
	ext := strings.ToLower(filepath.Ext(lower))
	if ext == ".rtc" || ext == ".dsv" || ext == ".bak" || ext == ".tmp" || ext == ".state" {
		return true
	}
	return strings.Contains(lower, ".sav.") || strings.Contains(lower, ".srm.")
}
