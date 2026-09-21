package syncthing

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"log/slog"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/apgul/thorsync/internal/adapter"
	"github.com/apgul/thorsync/internal/broker"
	"github.com/apgul/thorsync/internal/events"
	"github.com/apgul/thorsync/internal/model"
	"github.com/apgul/thorsync/internal/store"
)

type EndpointConfig struct{ ID, FolderID, Root string }

type Runner struct {
	client           *Client
	store            *store.Store
	broker           *broker.Service
	hub              *events.Hub
	endpoints        map[string]EndpointConfig
	folderToEndpoint map[string]string
	reconcileEvery   time.Duration
}

func NewRunner(client *Client, db *store.Store, brokerService *broker.Service, hub *events.Hub, endpoints []EndpointConfig, reconcileEvery time.Duration) *Runner {
	r := &Runner{client: client, store: db, broker: brokerService, hub: hub, endpoints: map[string]EndpointConfig{}, folderToEndpoint: map[string]string{}, reconcileEvery: reconcileEvery}
	for _, endpoint := range endpoints {
		r.endpoints[endpoint.ID] = endpoint
		r.folderToEndpoint[endpoint.FolderID] = endpoint.ID
	}
	return r
}

func (r *Runner) Run(ctx context.Context) {
	go r.eventLoop(ctx)
	go r.reconcileLoop(ctx)
	go r.statusLoop(ctx)
}

func (r *Runner) eventLoop(ctx context.Context) {
	raw, _ := r.store.Setting(ctx, "syncthing_event_cursor")
	cursor, _ := strconv.ParseInt(raw, 10, 64)
	backoff := time.Second
	for ctx.Err() == nil {
		eventsBatch, err := r.client.Events(ctx, cursor)
		if err != nil {
			r.hub.Publish(events.Event{Type: "dependency-degraded", Message: "Syncthing events: " + err.Error()})
			if !wait(ctx, backoff) {
				return
			}
			if backoff < 30*time.Second {
				backoff *= 2
			}
			continue
		}
		backoff = time.Second
		for _, event := range eventsBatch {
			if event.ID > cursor {
				cursor = event.ID
			}
			r.handleEvent(ctx, event)
		}
		_ = r.store.SetSetting(ctx, "syncthing_event_cursor", strconv.FormatInt(cursor, 10))
	}
}

func (r *Runner) handleEvent(ctx context.Context, event Event) {
	var data ChangeData
	if err := json.Unmarshal(event.Data, &data); err != nil {
		return
	}
	endpointID, ok := r.folderToEndpoint[data.Folder]
	if !ok {
		return
	}
	if !adapter.IsSavePath(data.Path) && !adapter.IsUnexpectedSidecar(data.Path) {
		return
	}
	remoteChange := event.Type == "RemoteChangeDetected"
	provenance := model.ProvenanceUnknown
	if remoteChange {
		// The endpoint folders are shared with exactly one remote device, so a
		// RemoteChangeDetected event is direct provenance for that endpoint.
		provenance = model.ProvenanceConfirmed
	}
	if strings.EqualFold(data.Action, "deleted") {
		_ = r.broker.DeleteObserved(ctx, endpointID, data.Path, nil, provenance)
		return
	}
	var modified *time.Time
	if info, err := r.client.File(ctx, data.Folder, data.Path); err == nil {
		if !info.Global.Modified.IsZero() {
			value := info.Global.Modified
			modified = &value
		}
		if data.ModifiedBy == "" && info.Global.ModifiedBy != "" {
			data.ModifiedBy = info.Global.ModifiedBy
		}
	}
	r.broker.Schedule(ctx, broker.CaptureInput{EndpointID: endpointID, RelativePath: data.Path, SourceModified: modified, Provenance: provenance, ConfirmDelivery: remoteChange, Detail: "Syncthing " + event.Type + " modifiedBy=" + data.ModifiedBy})
}

func (r *Runner) reconcileLoop(ctx context.Context) {
	_ = r.Reconcile(ctx)
	ticker := time.NewTicker(r.reconcileEvery)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := r.Reconcile(ctx); err != nil {
				slog.Warn("Syncthing reconciliation failed", "error", err)
				r.hub.Publish(events.Event{Type: "dependency-degraded", Message: "Reconciliation: " + err.Error()})
			}
		}
	}
}

func (r *Runner) Reconcile(ctx context.Context) error {
	for _, endpoint := range r.endpoints {
		seen := map[string]struct{}{}
		err := filepath.WalkDir(endpoint.Root, func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() {
				return nil
			}
			relative, err := filepath.Rel(endpoint.Root, path)
			if err != nil {
				return err
			}
			relative = filepath.ToSlash(relative)
			if !adapter.IsSavePath(relative) && !adapter.IsUnexpectedSidecar(relative) {
				return nil
			}
			seen[relative] = struct{}{}
			var modified *time.Time
			provenance := model.ProvenanceUnknown
			detail := "Startup/periodic reconciliation"
			if info, err := r.client.File(ctx, endpoint.FolderID, relative); err == nil {
				if !info.Global.Modified.IsZero() {
					value := info.Global.Modified
					modified = &value
				}
				if info.Global.ModifiedBy != "" {
					// Reconciliation proves metadata but not the originating event.
					// Folder isolation makes the endpoint the likely source without
					// pretending that this is confirmed provenance.
					provenance = model.ProvenanceInferred
					detail += " modifiedBy=" + info.Global.ModifiedBy
				}
			}
			r.broker.Schedule(ctx, broker.CaptureInput{EndpointID: endpoint.ID, RelativePath: relative, SourceModified: modified, Provenance: provenance, Detail: detail})
			return nil
		})
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			_ = r.store.SetSetting(ctx, "last_reconcile_error", err.Error())
			return err
		}
		bindings, bindErr := r.store.ListEndpointBindings(ctx, endpoint.ID)
		if bindErr != nil {
			return bindErr
		}
		for _, binding := range bindings {
			if _, ok := seen[filepath.ToSlash(binding.RelativePath)]; !ok {
				_ = r.broker.DeleteObserved(ctx, endpoint.ID, binding.RelativePath, nil, model.ProvenanceUnknown)
			}
		}
	}
	_ = r.store.SetSetting(ctx, "last_reconcile_at", time.Now().UTC().Format(time.RFC3339Nano))
	_ = r.store.SetSetting(ctx, "last_reconcile_error", "")
	return nil
}

func (r *Runner) statusLoop(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		r.refreshStatus(ctx)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (r *Runner) refreshStatus(ctx context.Context) {
	connections, err := r.client.Connections(ctx)
	if err != nil {
		return
	}
	endpoints, _ := r.store.ListEndpoints(ctx)
	for _, endpoint := range endpoints {
		connected := false
		if endpoint.DeviceID != "" {
			if connection, ok := connections.Connections[endpoint.DeviceID]; ok {
				connected = connection.Connected && !connection.Paused
			}
		}
		state := "offline"
		if connected {
			state = "connected"
		}
		if cfg, ok := r.endpoints[endpoint.ID]; ok {
			if folderStatus, err := r.client.FolderStatus(ctx, cfg.FolderID); err == nil && folderStatus.State != "" {
				state = folderStatus.State
			}
			if connected && endpoint.DeviceID != "" {
				if completion, err := r.client.Completion(ctx, cfg.FolderID, endpoint.DeviceID); err == nil && completion.RemoteState == "valid" && completion.Completion >= 100 && completion.NeedItems == 0 && completion.NeedDeletes == 0 {
					if delivered, err := r.store.CompleteEndpointDeliveries(ctx, endpoint.ID); err == nil && delivered > 0 {
						r.hub.Publish(events.Event{Type: "delivery", Message: "Syncthing confirmed delivery to " + endpoint.ID})
					}
				}
			}
		}
		_ = r.store.UpdateEndpoint(ctx, endpoint.ID, "", state, connected)
	}
}

func wait(ctx context.Context, duration time.Duration) bool {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
