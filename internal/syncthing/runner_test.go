package syncthing

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/apgul/thorsync/internal/archive"
	"github.com/apgul/thorsync/internal/broker"
	"github.com/apgul/thorsync/internal/config"
	"github.com/apgul/thorsync/internal/events"
	"github.com/apgul/thorsync/internal/model"
	"github.com/apgul/thorsync/internal/store"
)

func TestRefreshStatusRequiresLiveConnectionBeforeConfirmingDelivery(t *testing.T) {
	for _, test := range []struct {
		name      string
		connected bool
	}{
		{name: "offline", connected: false},
		{name: "connected", connected: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			root := t.TempDir()
			cfg := config.Config{DataDir: filepath.Join(root, "data"), ArchiveDir: filepath.Join(root, "archive"), ThorDir: filepath.Join(root, "thor"), WindowsDir: filepath.Join(root, "windows"), SyncthingThorFolderID: "thor-folder", SyncthingWindowsFolderID: "windows-folder"}
			for _, dir := range []string{cfg.ThorDir, cfg.WindowsDir} {
				if err := os.MkdirAll(dir, 0o750); err != nil {
					t.Fatal(err)
				}
			}
			db, err := store.Open(ctx, cfg)
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			if err = db.UpdateEndpoint(ctx, "windows", "WINDOWS-DEVICE", "offline", false); err != nil {
				t.Fatal(err)
			}
			game, _ := db.CreateGame(ctx, "Advance Wars", model.PlatformGBA, "", "")
			thor, _ := db.UpsertBinding(ctx, model.SaveBinding{GameID: game.ID, EndpointID: "thor", ProfileID: "thor-mgba", RelativePath: "Advance Wars.srm"})
			_, _ = db.UpsertBinding(ctx, model.SaveBinding{GameID: game.ID, EndpointID: "windows", ProfileID: "windows-mgba", RelativePath: "Advance Wars.sav"})
			archiveStore := archive.New(cfg.ArchiveDir, 5<<30, 0)
			blob, err := archiveStore.PutBytes(bytes.Repeat([]byte{0x55}, 64*1024))
			if err != nil {
				t.Fatal(err)
			}
			head, err := db.RecordIngest(ctx, store.IngestParams{Binding: thor, ObservedBlob: blob, BatteryBlob: blob})
			if err != nil {
				t.Fatal(err)
			}
			op, err := db.CreateOperation(ctx, game.ID, head.RevisionID, "windows", "Advance Wars.sav", "windows-mgba", blob)
			if err != nil {
				t.Fatal(err)
			}
			if err = db.UpdateOperation(ctx, op.ID, "written", ""); err != nil {
				t.Fatal(err)
			}

			completionRequests := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/rest/system/connections":
					_ = json.NewEncoder(w).Encode(map[string]any{"connections": map[string]any{"WINDOWS-DEVICE": map[string]any{"connected": test.connected, "paused": false}}})
				case "/rest/db/status":
					_ = json.NewEncoder(w).Encode(map[string]any{"state": "idle"})
				case "/rest/db/completion":
					completionRequests++
					_ = json.NewEncoder(w).Encode(map[string]any{"completion": 100, "needItems": 0, "needDeletes": 0, "remoteState": "valid"})
				default:
					http.NotFound(w, r)
				}
			}))
			defer server.Close()
			brokerService := broker.New(db, archiveStore, events.New())
			runner := NewRunner(New(server.URL, "secret"), db, brokerService, events.New(), []EndpointConfig{{ID: "windows", FolderID: "windows-folder", Root: cfg.WindowsDir}}, 0)
			runner.refreshStatus(ctx)

			binding, err := db.FindBinding(ctx, "windows", "Advance Wars.sav")
			if err != nil {
				t.Fatal(err)
			}
			detail, err := db.GetGame(ctx, game.ID)
			if err != nil {
				t.Fatal(err)
			}
			if !test.connected {
				if completionRequests != 0 || binding.LastDeployedRevisionID != "" || detail.Operations[0].State != "written" {
					t.Fatalf("offline endpoint was incorrectly confirmed: requests=%d binding=%#v operation=%#v", completionRequests, binding, detail.Operations[0])
				}
			} else if completionRequests != 1 || binding.LastDeployedRevisionID != head.RevisionID || detail.Operations[0].State != "delivered" {
				t.Fatalf("connected endpoint was not confirmed: requests=%d binding=%#v operation=%#v", completionRequests, binding, detail.Operations[0])
			}
		})
	}
}
