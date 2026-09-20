package adapter

import (
	"testing"

	"github.com/apgul/thorsync/internal/model"
)

func TestSupportedProfilesAndSizes(t *testing.T) {
	tests := []struct {
		profile  string
		platform model.Platform
		path     string
		size     int64
	}{
		{"thor-mgba", model.PlatformGBA, "Pokemon Emerald.srm", 128 * 1024},
		{"windows-vbam", model.PlatformGBA, "Pokemon Emerald.sav", 128 * 1024},
		{"thor-melonds-ds", model.PlatformNDS, "New Super Mario Bros.srm", 512 * 1024},
		{"windows-melonds", model.PlatformNDS, "New Super Mario Bros.sav", 512 * 1024},
	}
	for _, test := range tests {
		if err := Validate(test.profile, test.platform, test.path, test.size); err != nil {
			t.Errorf("Validate(%s): %v", test.profile, err)
		}
	}
}

func TestRejectsMismatchedOrAmbiguousSave(t *testing.T) {
	for _, test := range []struct {
		name, profile, path string
		platform            model.Platform
		size                int64
	}{
		{"wrong extension", "thor-mgba", "game.sav", model.PlatformGBA, 64 * 1024},
		{"wrong platform", "thor-mgba", "game.srm", model.PlatformNDS, 64 * 1024},
		{"unsupported size", "windows-vbam", "game.sav", model.PlatformGBA, 12345},
		{"unknown profile", "unknown", "game.sav", model.PlatformGBA, 64 * 1024},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := Validate(test.profile, test.platform, test.path, test.size); err == nil {
				t.Fatal("expected validation failure")
			}
		})
	}
}

func TestUnexpectedSidecarsAreSeparatedFromSaves(t *testing.T) {
	for _, path := range []string{"game.rtc", "game.sav.bak", "game.dsv", "slot.state"} {
		if !IsUnexpectedSidecar(path) || IsSavePath(path) {
			t.Errorf("%q was not classified as a sidecar only", path)
		}
	}
	if IsUnexpectedSidecar("game.sync-conflict-20260920.sav") {
		t.Fatal("Syncthing conflict saves must remain save candidates")
	}
}

func TestOriginalConflictPath(t *testing.T) {
	got, ok := OriginalConflictPath("nested/Pokemon.sync-conflict-20260920-120000-ABC1234.sav")
	if !ok || got != "nested/Pokemon.sav" {
		t.Fatalf("got %q, %v", got, ok)
	}
	if _, ok := OriginalConflictPath("nested/Pokemon.sav"); ok {
		t.Fatal("ordinary save was treated as conflict artifact")
	}
}
