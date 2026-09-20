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
	"windows-melonds": {ID: "windows-melonds", Name: "melonDS", EndpointID: "windows", Platform: model.PlatformNDS, Extension: ".sav", Format: "raw-battery"},
}

func Profiles() []model.EmulatorProfile {
	result := make([]model.EmulatorProfile, 0, len(profiles))
	for _, id := range []string{"thor-mgba", "thor-melonds-ds", "windows-vbam", "windows-melonds"} {
		result = append(result, profiles[id])
	}
	return result
}

func Profile(id string) (model.EmulatorProfile, bool) {
	profile, ok := profiles[id]
	return profile, ok
}

func Validate(profileID string, platform model.Platform, relativePath string, size int64) error {
	profile, ok := Profile(profileID)
	if !ok {
		return fmt.Errorf("unsupported emulator profile %q", profileID)
	}
	if profile.Platform != platform {
		return fmt.Errorf("profile %s does not support platform %s", profileID, platform)
	}
	if strings.ToLower(filepath.Ext(relativePath)) != profile.Extension {
		return fmt.Errorf("%s expects %s saves", profile.Name, profile.Extension)
	}
	if _, ok := allowedSizes[platform][size]; !ok {
		return fmt.Errorf("unsupported %s save size: %d bytes", platform, size)
	}
	return nil
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
