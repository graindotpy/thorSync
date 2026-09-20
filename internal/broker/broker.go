package broker

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	pathpkg "path"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/apgul/thorsync/internal/adapter"
	"github.com/apgul/thorsync/internal/archive"
	"github.com/apgul/thorsync/internal/events"
	"github.com/apgul/thorsync/internal/ids"
	"github.com/apgul/thorsync/internal/model"
	"github.com/apgul/thorsync/internal/store"
)

type CaptureInput struct {
	EndpointID     string
	RelativePath   string
	SourceModified *time.Time
	Provenance     model.Provenance
	// ConfirmDelivery is true only when Syncthing has proven that this
	// occurrence came from the remote endpoint, never from connection timing.
	ConfirmDelivery bool
	Detail          string
}

type Service struct {
	store   *store.Store
	archive *archive.Archive
	hub     *events.Hub

	mu     sync.Mutex
	timers map[string]*time.Timer
	closed bool
}

func New(store *store.Store, archive *archive.Archive, hub *events.Hub) *Service {
	return &Service{store: store, archive: archive, hub: hub, timers: map[string]*time.Timer{}}
}

func (s *Service) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	for _, timer := range s.timers {
		timer.Stop()
	}
	s.timers = map[string]*time.Timer{}
}

// Schedule debounces changes to the same save for five seconds before checking
// the file twice for stability and archiving it.
func (s *Service) Schedule(ctx context.Context, input CaptureInput) {
	key := input.EndpointID + "\x00" + filepath.ToSlash(input.RelativePath)
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	if timer, ok := s.timers[key]; ok {
		timer.Stop()
	}
	s.timers[key] = time.AfterFunc(5*time.Second, func() {
		s.mu.Lock()
		delete(s.timers, key)
		closed := s.closed
		s.mu.Unlock()
		if closed {
			return
		}
		captureCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		if _, err := s.Capture(captureCtx, input); err != nil && !errors.Is(err, context.Canceled) {
			s.hub.Publish(events.Event{Type: "capture-error", Message: err.Error()})
		}
	})
}

func (s *Service) Capture(ctx context.Context, input CaptureInput) (store.IngestResult, error) {
	relative, err := cleanRelative(input.RelativePath)
	if err != nil {
		return store.IngestResult{}, err
	}
	if !adapter.IsSavePath(relative) && !adapter.IsUnexpectedSidecar(relative) {
		return store.IngestResult{}, fmt.Errorf("unsupported save filename %q", relative)
	}
	root, err := s.store.RootForEndpoint(ctx, input.EndpointID)
	if err != nil {
		return store.IngestResult{}, err
	}
	path, err := safeJoin(root, relative)
	if err != nil {
		return store.IngestResult{}, err
	}
	info, err := stableFile(ctx, path)
	if err != nil {
		size := int64(0)
		if stat, statErr := os.Lstat(path); statErr == nil {
			size = stat.Size()
		}
		_ = s.store.RecordQuarantine(ctx, input.EndpointID, relative, size, input.SourceModified, input.Provenance, "Quarantined: "+err.Error())
		return store.IngestResult{}, err
	}
	bindingPath := relative
	forceConflict := false
	if original, ok := adapter.OriginalConflictPath(relative); ok {
		bindingPath = original
		forceConflict = true
	}
	binding, bindErr := s.store.FindBinding(ctx, input.EndpointID, bindingPath)
	if bindErr != nil && !errors.Is(bindErr, store.ErrNotFound) {
		return store.IngestResult{}, bindErr
	}
	blob, err := s.archive.PutFile(path)
	if err != nil {
		_ = s.store.RecordQuarantine(ctx, input.EndpointID, relative, info.Size(), input.SourceModified, input.Provenance, "Capture paused: "+err.Error())
		return store.IngestResult{}, err
	}
	if adapter.IsUnexpectedSidecar(relative) {
		if err := s.store.RecordUnassigned(ctx, input.EndpointID, relative, blob, input.SourceModified, time.Now().UTC(), input.Provenance, "Quarantined: unexpected emulator sidecar"); err != nil {
			return store.IngestResult{}, err
		}
		s.hub.Publish(events.Event{Type: "quarantine", Message: "Unexpected sidecar quarantined on " + input.EndpointID})
		return store.IngestResult{State: "quarantined"}, nil
	}
	if errors.Is(bindErr, store.ErrNotFound) {
		if err := s.store.RecordUnassigned(ctx, input.EndpointID, relative, blob, input.SourceModified, time.Now().UTC(), input.Provenance, "Awaiting game mapping"); err != nil {
			return store.IngestResult{}, err
		}
		s.hub.Publish(events.Event{Type: "unassigned", Message: "Unassigned save discovered on " + input.EndpointID})
		return store.IngestResult{State: "unassigned"}, nil
	}
	platform, err := s.store.GamePlatform(ctx, binding.GameID)
	if err != nil {
		return store.IngestResult{}, err
	}
	if err := adapter.Validate(binding.ProfileID, platform, relative, info.Size()); err != nil {
		_ = s.store.RecordUnassigned(ctx, input.EndpointID, relative, blob, input.SourceModified, time.Now().UTC(), input.Provenance, "Quarantined: "+err.Error())
		return store.IngestResult{}, err
	}
	result, err := s.store.RecordIngest(ctx, store.IngestParams{Binding: binding, Blob: blob, SourceModified: input.SourceModified, ObservedAt: time.Now().UTC(), Provenance: input.Provenance, ForceConflict: forceConflict, ConfirmDelivery: input.ConfirmDelivery, ObservationPath: relative, Detail: input.Detail})
	if err != nil {
		return store.IngestResult{}, err
	}
	if forceConflict && result.State == "branch" {
		// The content is durably archived and linked to a branch. Removing the
		// Syncthing artifact prevents it from being mistaken for a normal save.
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			_ = s.store.RecordQuarantine(ctx, input.EndpointID, relative, info.Size(), input.SourceModified, input.Provenance, "Conflict archived but artifact could not be removed: "+err.Error())
			return result, fmt.Errorf("conflict archived but artifact could not be quarantined: %w", err)
		}
	}
	if result.ShouldPropagate {
		if err := s.PropagateRevision(ctx, result.GameID, result.RevisionID, input.EndpointID); err != nil {
			s.hub.Publish(events.Event{Type: "propagation-error", GameID: result.GameID, Message: err.Error()})
			return result, err
		}
	}
	s.hub.Publish(events.Event{Type: "revision", GameID: result.GameID, Message: "Save revision captured"})
	return result, nil
}

func stableFile(ctx context.Context, path string) (os.FileInfo, error) {
	first, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !first.Mode().IsRegular() {
		return nil, errors.New("save path is not a regular file")
	}
	if first.Size() == 0 {
		return nil, errors.New("zero-byte save is quarantined")
	}
	if first.Size() > 16*1024*1024 {
		return nil, errors.New("save exceeds 16 MiB safety limit")
	}
	timer := time.NewTimer(time.Second)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-timer.C:
	}
	second, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if second.Size() != first.Size() || !second.ModTime().Equal(first.ModTime()) {
		return nil, errors.New("save is still changing; capture will retry on the next event")
	}
	return second, nil
}

func cleanRelative(path string) (string, error) {
	// Normalize both separator styles before validation. Syncthing normally
	// reports slash-separated paths, but accepting a Windows-style backslash
	// here would let a UNC path bypass the Linux runner's filepath checks.
	path = strings.ReplaceAll(strings.TrimSpace(path), "\\", "/")
	clean := pathpkg.Clean(path)
	drivePath := len(clean) >= 2 && ((clean[0] >= 'A' && clean[0] <= 'Z') || (clean[0] >= 'a' && clean[0] <= 'z')) && clean[1] == ':'
	if clean == "" || clean == "." || strings.HasPrefix(clean, "../") || strings.HasPrefix(clean, "/") || drivePath || filepath.IsAbs(filepath.FromSlash(clean)) || strings.ContainsRune(clean, '\x00') {
		return "", errors.New("unsafe relative path")
	}
	return clean, nil
}

func safeJoin(root, relative string) (string, error) {
	clean, err := cleanRelative(relative)
	if err != nil {
		return "", err
	}
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	candidate := filepath.Join(rootAbs, filepath.FromSlash(clean))
	rel, err := filepath.Rel(rootAbs, candidate)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", errors.New("path escapes endpoint root")
	}
	return candidate, nil
}

func (s *Service) PropagateRevision(ctx context.Context, gameID, revisionID, sourceEndpoint string) error {
	revision, err := s.store.Revision(ctx, revisionID)
	if err != nil {
		return err
	}
	bindings, err := s.store.ListBindings(ctx, gameID)
	if err != nil {
		return err
	}
	platform, err := s.store.GamePlatform(ctx, gameID)
	if err != nil {
		return err
	}
	for _, binding := range bindings {
		if !binding.Enabled || binding.EndpointID == sourceEndpoint || binding.LastDeployedRevisionID == revisionID {
			continue
		}
		if err := adapter.Validate(binding.ProfileID, platform, binding.RelativePath, revision.Size); err != nil {
			return fmt.Errorf("target %s: %w", binding.EndpointID, err)
		}
		op, err := s.store.CreateOperation(ctx, gameID, revisionID, binding.EndpointID, binding.RelativePath, revision.BlobHash)
		if err != nil {
			return err
		}
		if op.State == "delivered" || op.State == "written" || op.State == "superseded" {
			continue
		}
		if err := s.writeTarget(ctx, binding, revision); err != nil {
			_ = s.store.UpdateOperation(ctx, op.ID, "failed", err.Error())
			return err
		}
		if err := s.store.UpdateOperation(ctx, op.ID, "written", ""); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) writeTarget(ctx context.Context, binding model.SaveBinding, revision model.Revision) error {
	root, err := s.store.RootForEndpoint(ctx, binding.EndpointID)
	if err != nil {
		return err
	}
	target, err := safeJoin(root, binding.RelativePath)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o750); err != nil {
		return err
	}
	source, err := s.archive.Open(revision.BlobHash)
	if err != nil {
		return err
	}
	defer source.Close()
	temp := filepath.Join(filepath.Dir(target), ".thorsync-"+ids.New()+".tmp")
	output, err := os.OpenFile(temp, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o640)
	if err != nil {
		return err
	}
	remove := true
	defer func() {
		_ = output.Close()
		if remove {
			_ = os.Remove(temp)
		}
	}()
	written, err := io.Copy(output, source)
	if err != nil {
		return err
	}
	if written != revision.Size {
		return fmt.Errorf("short archive copy: wrote %d of %d bytes", written, revision.Size)
	}
	if err = output.Sync(); err != nil {
		return err
	}
	if err = output.Close(); err != nil {
		return err
	}
	if err = os.Rename(temp, target); err != nil {
		return err
	}
	remove = false
	dir, err := os.Open(filepath.Dir(target))
	if err == nil {
		_ = dir.Sync()
		_ = dir.Close()
	}
	return nil
}

func (s *Service) DeleteObserved(ctx context.Context, endpointID, relativePath string, sourceModified *time.Time, provenance model.Provenance) error {
	relative, err := cleanRelative(relativePath)
	if err != nil {
		return err
	}
	if err = s.store.RecordDeletion(ctx, endpointID, relative, sourceModified, provenance); err == nil {
		s.hub.Publish(events.Event{Type: "missing", Message: "Save is missing on " + endpointID})
	}
	return err
}

func (s *Service) Restore(ctx context.Context, gameID, sourceRevisionID, expectedHead, idempotencyKey, actor, kind string) (model.Revision, error) {
	revision, err := s.store.CreateHeadFromExisting(ctx, store.HeadMutation{GameID: gameID, SourceRevisionID: sourceRevisionID, ExpectedHeadID: expectedHead, IdempotencyKey: idempotencyKey, Actor: actor, Kind: kind})
	if err != nil {
		return model.Revision{}, err
	}
	if err = s.PropagateRevision(ctx, gameID, revision.ID, ""); err != nil {
		return revision, err
	}
	s.hub.Publish(events.Event{Type: kind, GameID: gameID, Message: "A new save head was created"})
	return revision, nil
}
