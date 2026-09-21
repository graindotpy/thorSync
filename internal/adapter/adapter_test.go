package adapter

import (
	"bytes"
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
		{"windows-mgba", model.PlatformGBA, "Pokemon Emerald.sav", 128*1024 + gbaRTCSize},
		{"thor-melonds-ds", model.PlatformNDS, "New Super Mario Bros.srm", 512 * 1024},
		{"windows-melonds", model.PlatformNDS, "New Super Mario Bros.sav", 512 * 1024},
	}
	for _, test := range tests {
		if err := Validate(test.profile, test.platform, test.path, test.size); err != nil {
			t.Errorf("Validate(%s): %v", test.profile, err)
		}
	}
}

func TestWindowsMGBAProfileAcceptsOnlyRawOrExactRTCSize(t *testing.T) {
	for _, rawSize := range []int64{512, 8 * 1024, 32 * 1024, 64 * 1024, 128 * 1024} {
		for _, size := range []int64{rawSize, rawSize + gbaRTCSize} {
			if err := Validate("windows-mgba", model.PlatformGBA, "game.sav", size); err != nil {
				t.Fatalf("valid mGBA size %d rejected: %v", size, err)
			}
		}
		for _, size := range []int64{rawSize + gbaRTCSize - 1, rawSize + gbaRTCSize + 1} {
			if err := Validate("windows-mgba", model.PlatformGBA, "game.sav", size); err == nil {
				t.Fatalf("near-miss mGBA size %d accepted", size)
			}
		}
		if err := Validate("windows-vbam", model.PlatformGBA, "game.sav", rawSize+gbaRTCSize); err == nil {
			t.Fatalf("VBA-M accepted %d-byte mGBA RTC-wrapped save", rawSize+gbaRTCSize)
		}
	}
	if err := Validate("windows-mgba", model.PlatformGBA, "game.sav", 12345+gbaRTCSize); err == nil {
		t.Fatal("unsupported battery size with an RTC footer was accepted")
	}
}

func TestProfilesRetainVBAMAndAddWindowsMGBA(t *testing.T) {
	if profile, ok := Profile("windows-vbam"); !ok || profile.Format != "raw-battery" {
		t.Fatalf("VBA-M profile missing or changed: %#v, %v", profile, ok)
	}
	profile, ok := Profile("windows-mgba")
	if !ok {
		t.Fatal("standalone mGBA profile missing")
	}
	if profile.EndpointID != "windows" || profile.Platform != model.PlatformGBA || profile.Extension != ".sav" {
		t.Fatalf("unexpected mGBA profile: %#v", profile)
	}
}

func TestDecodeWindowsMGBASplitsOpaqueRTCFooter(t *testing.T) {
	battery := bytes.Repeat([]byte{0x5a}, 64*1024)
	rtc := []byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15}
	input := append(append([]byte(nil), battery...), rtc...)

	decoded, err := Decode("windows-mgba", model.PlatformGBA, "nested/game.SAV", input)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(decoded.Battery, battery) || !bytes.Equal(decoded.RTC, rtc) {
		t.Fatal("mGBA save was not split without byte changes")
	}
	input[0], input[len(input)-1] = 0, 0
	if decoded.Battery[0] != 0x5a || decoded.RTC[len(decoded.RTC)-1] != 15 {
		t.Fatal("decoded components alias the source buffer")
	}
}

func TestDecodeRawSaveHasNoRTC(t *testing.T) {
	input := bytes.Repeat([]byte{0xa5}, 32*1024)
	for _, profileID := range []string{"thor-mgba", "windows-vbam", "windows-mgba"} {
		path := "game.sav"
		if profileID == "thor-mgba" {
			path = "game.srm"
		}
		decoded, err := Decode(profileID, model.PlatformGBA, path, input)
		if err != nil {
			t.Fatalf("Decode(%s): %v", profileID, err)
		}
		if !bytes.Equal(decoded.Battery, input) || len(decoded.RTC) != 0 {
			t.Fatalf("Decode(%s) returned %#v", profileID, decoded)
		}
	}
}

func TestEncodeTargetsAreDeterministic(t *testing.T) {
	battery := bytes.Repeat([]byte{0x3c}, 8*1024)
	rtc := bytes.Repeat([]byte{0xd7}, gbaRTCSize)
	save := Save{Battery: battery, RTC: rtc}

	combined, err := Encode("windows-mgba", model.PlatformGBA, "game.sav", save)
	if err != nil {
		t.Fatal(err)
	}
	wantCombined := append(append([]byte(nil), battery...), rtc...)
	if !bytes.Equal(combined, wantCombined) {
		t.Fatal("standalone mGBA output did not combine battery and RTC")
	}
	combined[0] = 0
	if save.Battery[0] != 0x3c {
		t.Fatal("encoded output aliases canonical battery bytes")
	}

	for _, target := range []struct{ profileID, path string }{
		{"thor-mgba", "game.srm"},
		{"windows-vbam", "game.sav"},
	} {
		encoded, err := Encode(target.profileID, model.PlatformGBA, target.path, save)
		if err != nil {
			t.Fatalf("Encode(%s): %v", target.profileID, err)
		}
		if !bytes.Equal(encoded, battery) {
			t.Fatalf("%s did not emit raw battery bytes", target.profileID)
		}
	}

	rawMGBA, err := Encode("windows-mgba", model.PlatformGBA, "game.sav", Save{Battery: battery})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(rawMGBA, battery) {
		t.Fatal("mGBA with no RTC did not emit a raw save")
	}
}

func TestCodecRejectsInvalidMetadataAndComponentSizes(t *testing.T) {
	validBattery := bytes.Repeat([]byte{1}, 64*1024)
	for _, test := range []struct {
		name, profile, path string
		platform            model.Platform
		save                Save
	}{
		{"unknown profile", "unknown", "game.sav", model.PlatformGBA, Save{Battery: validBattery}},
		{"wrong extension", "windows-mgba", "game.srm", model.PlatformGBA, Save{Battery: validBattery}},
		{"wrong platform", "windows-mgba", "game.sav", model.PlatformNDS, Save{Battery: validBattery}},
		{"unsupported battery", "windows-mgba", "game.sav", model.PlatformGBA, Save{Battery: []byte{1}}},
		{"invalid RTC", "windows-mgba", "game.sav", model.PlatformGBA, Save{Battery: validBattery, RTC: []byte{1}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := Encode(test.profile, test.platform, test.path, test.save); err == nil {
				t.Fatal("expected encode failure")
			}
		})
	}
	if _, err := Decode("windows-mgba", model.PlatformGBA, "game.srm", validBattery); err == nil {
		t.Fatal("Decode accepted the wrong extension")
	}
	if _, err := Decode("thor-melonds-ds", model.PlatformNDS, "game.srm", validBattery); err == nil {
		t.Fatal("GBA codec accepted NDS")
	}
}

func TestProfileSuggestionRefusesAmbiguousRawWindowsGBA(t *testing.T) {
	matches := MatchingProfiles("windows", model.PlatformGBA, "game.sav", 64*1024)
	if len(matches) != 2 || matches[0].ID != "windows-vbam" || matches[1].ID != "windows-mgba" {
		t.Fatalf("unexpected raw Windows matches: %#v", matches)
	}
	if _, ok := SuggestProfile("windows", model.PlatformGBA, "game.sav", 64*1024); ok {
		t.Fatal("ambiguous raw Windows GBA save was guessed")
	}
	profile, ok := SuggestProfile("windows", model.PlatformGBA, "game.sav", 64*1024+gbaRTCSize)
	if !ok || profile.ID != "windows-mgba" {
		t.Fatalf("combined mGBA save suggestion = %#v, %v", profile, ok)
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
