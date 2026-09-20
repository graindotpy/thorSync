package api

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"mime/multipart"

	"github.com/apgul/thorsync/internal/model"
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
