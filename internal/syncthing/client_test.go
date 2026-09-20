package syncthing

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClientUsesAPIKeyAndEncodesFileMetadataRequest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-API-Key") != "secret" {
			t.Errorf("missing API key")
		}
		if r.URL.Path != "/rest/db/file" || r.URL.Query().Get("folder") != "thor-folder" || r.URL.Query().Get("file") != "nested/My Save.srm" {
			t.Errorf("unexpected request: %s?%s", r.URL.Path, r.URL.RawQuery)
		}
		_ = json.NewEncoder(w).Encode(FileInfo{Global: FileVersion{Name: "nested/My Save.srm", Size: 65536, ModifiedBy: "ABC1234"}})
	}))
	defer server.Close()

	info, err := New(server.URL, "secret").File(context.Background(), "thor-folder", `nested\My Save.srm`)
	if err != nil {
		t.Fatal(err)
	}
	if info.Global.Size != 65536 || info.Global.ModifiedBy != "ABC1234" {
		t.Fatalf("unexpected metadata: %#v", info)
	}
}

func TestCompletionAndEventMask(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/rest/db/completion":
			if r.URL.Query().Get("device") != "DEVICE-ID" || r.URL.Query().Get("folder") != "windows-folder" {
				t.Errorf("unexpected completion query: %s", r.URL.RawQuery)
			}
			_, _ = w.Write([]byte(`{"completion":100,"needBytes":0,"needItems":0,"needDeletes":0,"remoteState":"valid"}`))
		case "/rest/events/disk":
			if r.URL.Query().Get("events") != "RemoteChangeDetected,LocalChangeDetected" || r.URL.Query().Get("since") != "41" {
				t.Errorf("unexpected events query: %s", r.URL.RawQuery)
			}
			_, _ = w.Write([]byte(`[{"id":42,"globalID":42,"type":"RemoteChangeDetected","data":{"folder":"windows-folder","path":"game.sav","action":"modified"}}]`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client := New(server.URL, "secret")
	completion, err := client.Completion(context.Background(), "windows-folder", "DEVICE-ID")
	if err != nil || completion.Completion != 100 || completion.RemoteState != "valid" {
		t.Fatalf("completion=%#v err=%v", completion, err)
	}
	events, err := client.Events(context.Background(), 41)
	if err != nil || len(events) != 1 || events[0].ID != 42 {
		t.Fatalf("events=%#v err=%v", events, err)
	}
}

func TestEnsureFolderCreatesIsolatedVersionedFolder(t *testing.T) {
	var configured FolderConfig
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rest/config/folders/thorsync-thor" {
			http.NotFound(w, r)
			return
		}
		if r.Method == http.MethodGet {
			http.NotFound(w, r)
			return
		}
		if r.Method != http.MethodPut {
			t.Fatalf("unexpected method %s", r.Method)
		}
		if err := json.NewDecoder(r.Body).Decode(&configured); err != nil {
			t.Fatal(err)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	client := New(server.URL, "secret")
	if err := client.EnsureFolder(context.Background(), "thorsync-thor", "ThorSync - Thor", "/var/syncthing/sync/thor", "THOR-ID"); err != nil {
		t.Fatal(err)
	}
	if configured.Path != "/var/syncthing/sync/thor" || configured.Type != "sendreceive" || len(configured.Devices) != 1 || configured.Devices[0].DeviceID != "THOR-ID" {
		t.Fatalf("unexpected folder config: %#v", configured)
	}
	if configured.Versioning["type"] != "staggered" {
		t.Fatalf("staggered versioning not configured: %#v", configured.Versioning)
	}
}
