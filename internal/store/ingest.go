package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/apgul/thorsync/internal/archive"
	"github.com/apgul/thorsync/internal/ids"
	"github.com/apgul/thorsync/internal/model"
)

type IngestParams struct {
	Binding model.SaveBinding
	// Blob is the legacy single-component input. New callers should provide
	// ObservedBlob and BatteryBlob; Blob remains as a compatibility fallback.
	Blob            archive.Blob
	ObservedBlob    archive.Blob
	BatteryBlob     archive.Blob
	RTCBlob         *archive.Blob
	SourceModified  *time.Time
	ObservedAt      time.Time
	Provenance      model.Provenance
	ForceConflict   bool
	ConfirmDelivery bool
	ObservationPath string
	Detail          string
}

type IngestResult struct {
	GameID          string
	RevisionID      string
	ContentHash     string
	State           string
	ShouldPropagate bool
	Echo            bool
}

type RejectedIngestParams struct {
	Binding         model.SaveBinding
	ObservedBlob    archive.Blob
	BatteryBlob     archive.Blob
	RTCBlob         *archive.Blob
	SourceModified  *time.Time
	ObservedAt      time.Time
	Provenance      model.Provenance
	ObservationPath string
	Detail          string
}

// PayloadContentHash returns the logical identity of a decoded save payload.
// Battery-only revisions retain the battery blob hash for backwards identity.
// Multi-component revisions use a versioned, unambiguous manifest hash.
func PayloadContentHash(batteryBlobHash, rtcBlobHash string) string {
	if rtcBlobHash == "" {
		return batteryBlobHash
	}
	digest := sha256.Sum256([]byte("thorsync-payload-v1\x00" + batteryBlobHash + "\x00" + rtcBlobHash))
	return hex.EncodeToString(digest[:])
}

func normalizeIngestBlobs(p IngestParams) (archive.Blob, archive.Blob, error) {
	observed := p.ObservedBlob
	if observed.Hash == "" {
		observed = p.Blob
	}
	battery := p.BatteryBlob
	if battery.Hash == "" {
		battery = p.Blob
	}
	if battery.Hash == "" {
		battery = observed
	}
	if observed.Hash == "" {
		observed = battery
	}
	if observed.Hash == "" || battery.Hash == "" {
		return archive.Blob{}, archive.Blob{}, errors.New("observed and battery blobs are required")
	}
	return observed, battery, nil
}

func profileInheritsRTC(profileID string) bool {
	return profileID == "thor-mgba" || profileID == "windows-vbam"
}

func (s *Store) RecordIngest(ctx context.Context, p IngestParams) (IngestResult, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return IngestResult{}, err
	}
	defer tx.Rollback()
	if p.ObservedAt.IsZero() {
		p.ObservedAt = time.Now().UTC()
	}
	if p.Provenance == "" {
		p.Provenance = model.ProvenanceUnknown
	}
	observedBlob, batteryBlob, err := normalizeIngestBlobs(p)
	if err != nil {
		return IngestResult{}, err
	}
	observationPath := p.ObservationPath
	if observationPath == "" {
		observationPath = p.Binding.RelativePath
	}
	if _, err := tx.ExecContext(ctx, "INSERT OR IGNORE INTO blobs (hash,size,created_at) VALUES (?,?,?)", observedBlob.Hash, observedBlob.Size, p.ObservedAt); err != nil {
		return IngestResult{}, err
	}
	if _, err := tx.ExecContext(ctx, "INSERT OR IGNORE INTO blobs (hash,size,created_at) VALUES (?,?,?)", batteryBlob.Hash, batteryBlob.Size, p.ObservedAt); err != nil {
		return IngestResult{}, err
	}
	if p.RTCBlob != nil && p.RTCBlob.Hash != "" {
		if _, err := tx.ExecContext(ctx, "INSERT OR IGNORE INTO blobs (hash,size,created_at) VALUES (?,?,?)", p.RTCBlob.Hash, p.RTCBlob.Size, p.ObservedAt); err != nil {
			return IngestResult{}, err
		}
	}

	observationID := ids.New()
	if _, err := tx.ExecContext(ctx, `INSERT INTO observations (id,game_id,endpoint_id,relative_path,blob_hash,action,source_modified_at,observed_at,provenance,detail) VALUES (?,?,?,?,?,'changed',?,?,?,?)`,
		observationID, p.Binding.GameID, p.Binding.EndpointID, observationPath, observedBlob.Hash, p.SourceModified, p.ObservedAt, p.Provenance, p.Detail); err != nil {
		return IngestResult{}, err
	}

	var echoOperationID, echoRevisionID string
	if !p.ForceConflict {
		err = tx.QueryRowContext(ctx, `SELECT id,revision_id FROM broker_operations
			WHERE target_endpoint_id=? AND relative_path=? AND blob_hash=?
			AND (profile_id='' OR profile_id=?) AND state IN ('pending','written')
			ORDER BY created_at DESC LIMIT 1`, p.Binding.EndpointID, p.Binding.RelativePath, observedBlob.Hash, p.Binding.ProfileID).Scan(&echoOperationID, &echoRevisionID)
	} else {
		err = sql.ErrNoRows
	}
	if err == nil {
		now := time.Now().UTC()
		action := "broker-write-observed"
		state := "written"
		if p.ConfirmDelivery {
			action = "delivery-confirmed"
			state = "delivered"
			if _, err = tx.ExecContext(ctx, "UPDATE save_bindings SET last_deployed_revision_id=?,updated_at=? WHERE id=?", echoRevisionID, now, p.Binding.ID); err != nil {
				return IngestResult{}, err
			}
		}
		if _, err = tx.ExecContext(ctx, "UPDATE broker_operations SET state=?,updated_at=? WHERE id=?", state, now, echoOperationID); err != nil {
			return IngestResult{}, err
		}
		if _, err = tx.ExecContext(ctx, "UPDATE observations SET revision_id=?,action=? WHERE id=?", echoRevisionID, action, observationID); err != nil {
			return IngestResult{}, err
		}
		if p.ConfirmDelivery {
			if err = insertActivityTx(ctx, tx, p.Binding.GameID, "delivery", "Save delivered to "+p.Binding.EndpointID, p.Binding.EndpointID, now); err != nil {
				return IngestResult{}, err
			}
		}
		var contentHash string
		_ = tx.QueryRowContext(ctx, `SELECT COALESCE(rp.content_hash,r.blob_hash) FROM revisions r LEFT JOIN revision_payloads rp ON rp.revision_id=r.id WHERE r.id=?`, echoRevisionID).Scan(&contentHash)
		if err = tx.Commit(); err != nil {
			return IngestResult{}, err
		}
		return IngestResult{GameID: p.Binding.GameID, RevisionID: echoRevisionID, ContentHash: contentHash, State: state, Echo: true}, nil
	}
	if err != sql.ErrNoRows {
		return IngestResult{}, err
	}

	var rtcBlobHash string
	var rtcSize int64
	if p.RTCBlob != nil && p.RTCBlob.Hash != "" {
		rtcBlobHash = p.RTCBlob.Hash
		rtcSize = p.RTCBlob.Size
	} else if profileInheritsRTC(p.Binding.ProfileID) && p.Binding.LastDeployedRevisionID != "" {
		// A battery-only emulator cannot report RTC changes. Preserve the RTC from
		// the exact revision last delivered to that physical endpoint; inheriting
		// from the current head could silently make a stale edit appear linear.
		err = tx.QueryRowContext(ctx, `SELECT COALESCE(rtc_blob_hash,''),rtc_size FROM revision_payloads WHERE revision_id=?`, p.Binding.LastDeployedRevisionID).Scan(&rtcBlobHash, &rtcSize)
		if err != nil && err != sql.ErrNoRows {
			return IngestResult{}, err
		}
	}
	contentHash := PayloadContentHash(batteryBlob.Hash, rtcBlobHash)

	var currentID, currentHash string
	err = tx.QueryRowContext(ctx, `SELECT COALESCE(g.current_revision_id,''),COALESCE(rp.content_hash,r.blob_hash,'')
		FROM games g LEFT JOIN revisions r ON r.id=g.current_revision_id
		LEFT JOIN revision_payloads rp ON rp.revision_id=r.id WHERE g.id=?`, p.Binding.GameID).Scan(&currentID, &currentHash)
	if err == sql.ErrNoRows {
		return IngestResult{}, ErrNotFound
	}
	if err != nil {
		return IngestResult{}, err
	}
	if currentHash == contentHash && currentID != "" && !p.ForceConflict {
		if _, err = tx.ExecContext(ctx, "UPDATE observations SET revision_id=?,action='duplicate' WHERE id=?", currentID, observationID); err != nil {
			return IngestResult{}, err
		}
		if _, err = tx.ExecContext(ctx, "UPDATE save_bindings SET last_deployed_revision_id=?,updated_at=? WHERE id=?", currentID, time.Now().UTC(), p.Binding.ID); err != nil {
			return IngestResult{}, err
		}
		var enabled string
		if err = tx.QueryRowContext(ctx, "SELECT value FROM settings WHERE key='propagation_enabled'").Scan(&enabled); err != nil {
			return IngestResult{}, err
		}
		if err = tx.Commit(); err != nil {
			return IngestResult{}, err
		}
		return IngestResult{GameID: p.Binding.GameID, RevisionID: currentID, ContentHash: contentHash, State: "duplicate", ShouldPropagate: enabled == "true", Echo: true}, nil
	}

	// Repeated scans of the same conflict artifact must preserve provenance
	// without manufacturing duplicate immutable revisions. If the old branch
	// is no longer under review, reopen a conflict against the current head.
	var existingBranchID string
	err = tx.QueryRowContext(ctx, `SELECT r.id FROM revisions r LEFT JOIN revision_payloads rp ON rp.revision_id=r.id
		WHERE r.game_id=? AND COALESCE(rp.content_hash,r.blob_hash)=? AND r.state='branch'
		ORDER BY r.observed_at DESC LIMIT 1`, p.Binding.GameID, contentHash).Scan(&existingBranchID)
	if err == nil {
		if _, err = tx.ExecContext(ctx, "UPDATE observations SET revision_id=?,action='duplicate-branch' WHERE id=?", existingBranchID, observationID); err != nil {
			return IngestResult{}, err
		}
		var open int
		if err = tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM conflicts WHERE game_id=? AND branch_revision_id=? AND state='open'", p.Binding.GameID, existingBranchID).Scan(&open); err != nil {
			return IngestResult{}, err
		}
		if open == 0 {
			reason := "previously archived branch returned from a stale device"
			if p.ForceConflict {
				reason = "Syncthing conflict file detected"
			}
			if _, err = tx.ExecContext(ctx, `INSERT INTO conflicts (id,game_id,head_revision_id,branch_revision_id,reason,state,created_at) VALUES (?,?,?,?,?,'open',?)`, ids.New(), p.Binding.GameID, nullString(currentID), existingBranchID, reason, p.ObservedAt); err != nil {
				return IngestResult{}, err
			}
			if _, err = tx.ExecContext(ctx, "UPDATE games SET status='conflict',updated_at=? WHERE id=?", p.ObservedAt, p.Binding.GameID); err != nil {
				return IngestResult{}, err
			}
			if err = insertActivityTx(ctx, tx, p.Binding.GameID, "conflict", "Archived branch returned from "+p.Binding.EndpointID, p.Binding.EndpointID, p.ObservedAt); err != nil {
				return IngestResult{}, err
			}
		}
		if err = tx.Commit(); err != nil {
			return IngestResult{}, err
		}
		return IngestResult{GameID: p.Binding.GameID, RevisionID: existingBranchID, ContentHash: contentHash, State: "branch", Echo: true}, nil
	}
	if err != sql.ErrNoRows {
		return IngestResult{}, err
	}

	var openConflicts int
	if err = tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM conflicts WHERE game_id=? AND state='open'", p.Binding.GameID).Scan(&openConflicts); err != nil {
		return IngestResult{}, err
	}
	isInitial := currentID == ""
	linear := isInitial || (p.Binding.LastDeployedRevisionID == currentID && openConflicts == 0)
	if p.ForceConflict {
		linear = false
	}
	state := "head"
	kind := "captured"
	shouldPropagate := false
	if !linear {
		state = "branch"
		kind = "conflict"
	}
	parentRevisionID := currentID
	if !linear {
		// A branch descends from the exact revision last deployed to its source
		// binding, not from a newer head that the offline/stale device never saw.
		// An empty baseline remains unknown rather than inventing ancestry.
		parentRevisionID = p.Binding.LastDeployedRevisionID
	}
	revisionID := ids.New()
	if _, err = tx.ExecContext(ctx, `INSERT INTO revisions (id,game_id,parent_revision_id,blob_hash,size,source_endpoint_id,source_modified_at,observed_at,provenance,kind,state) VALUES (?,?,?,?,?,?,?,?,?,?,?)`,
		revisionID, p.Binding.GameID, nullString(parentRevisionID), batteryBlob.Hash, batteryBlob.Size, p.Binding.EndpointID, p.SourceModified, p.ObservedAt, p.Provenance, kind, state); err != nil {
		return IngestResult{}, err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO revision_payloads (revision_id,battery_blob_hash,battery_size,rtc_blob_hash,rtc_size,content_hash) VALUES (?,?,?,?,?,?)`,
		revisionID, batteryBlob.Hash, batteryBlob.Size, nullString(rtcBlobHash), rtcSize, contentHash); err != nil {
		return IngestResult{}, err
	}
	if _, err = tx.ExecContext(ctx, "UPDATE observations SET revision_id=? WHERE id=?", revisionID, observationID); err != nil {
		return IngestResult{}, err
	}
	if linear {
		if currentID != "" {
			if _, err = tx.ExecContext(ctx, "UPDATE revisions SET state='history' WHERE id=? AND state='head'", currentID); err != nil {
				return IngestResult{}, err
			}
		}
		if _, err = tx.ExecContext(ctx, "UPDATE games SET current_revision_id=?,status='healthy',updated_at=? WHERE id=?", revisionID, p.ObservedAt, p.Binding.GameID); err != nil {
			return IngestResult{}, err
		}
		if _, err = tx.ExecContext(ctx, "UPDATE save_bindings SET last_deployed_revision_id=?,updated_at=? WHERE id=?", revisionID, p.ObservedAt, p.Binding.ID); err != nil {
			return IngestResult{}, err
		}
		var enabled string
		if err = tx.QueryRowContext(ctx, "SELECT value FROM settings WHERE key='propagation_enabled'").Scan(&enabled); err != nil {
			return IngestResult{}, err
		}
		shouldPropagate = enabled == "true"
		if err = insertActivityTx(ctx, tx, p.Binding.GameID, "revision", "New save captured from "+p.Binding.EndpointID, p.Binding.EndpointID, p.ObservedAt); err != nil {
			return IngestResult{}, err
		}
	} else {
		reason := "save does not descend from the last version delivered to this device"
		if p.ForceConflict {
			reason = "Syncthing conflict file detected"
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO conflicts (id,game_id,head_revision_id,branch_revision_id,reason,state,created_at) VALUES (?,?,?,?,?,'open',?)`, ids.New(), p.Binding.GameID, nullString(currentID), revisionID, reason, p.ObservedAt); err != nil {
			return IngestResult{}, err
		}
		if _, err = tx.ExecContext(ctx, "UPDATE games SET status='conflict',updated_at=? WHERE id=?", p.ObservedAt, p.Binding.GameID); err != nil {
			return IngestResult{}, err
		}
		if err = insertActivityTx(ctx, tx, p.Binding.GameID, "conflict", "Conflicting save captured from "+p.Binding.EndpointID, p.Binding.EndpointID, p.ObservedAt); err != nil {
			return IngestResult{}, err
		}
	}
	if err = tx.Commit(); err != nil {
		return IngestResult{}, err
	}
	return IngestResult{GameID: p.Binding.GameID, RevisionID: revisionID, ContentHash: contentHash, State: state, ShouldPropagate: shouldPropagate}, nil
}

func (s *Store) RecordUnassigned(ctx context.Context, endpointID, relativePath string, blob archive.Blob, sourceModified *time.Time, observed time.Time, provenance model.Provenance, detail string) error {
	if observed.IsZero() {
		observed = time.Now().UTC()
	}
	if _, err := s.db.ExecContext(ctx, "INSERT OR IGNORE INTO blobs (hash,size,created_at) VALUES (?,?,?)", blob.Hash, blob.Size, observed); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO unassigned_files (id,endpoint_id,relative_path,blob_hash,size,source_modified_at,observed_at,provenance,state,detail) VALUES (?,?,?,?,?,?,?,?,'unassigned',?)
		ON CONFLICT(endpoint_id,relative_path) DO UPDATE SET blob_hash=excluded.blob_hash,size=excluded.size,source_modified_at=excluded.source_modified_at,observed_at=excluded.observed_at,provenance=excluded.provenance,state='unassigned',detail=excluded.detail`,
		ids.New(), endpointID, relativePath, blob.Hash, blob.Size, sourceModified, observed, provenance, detail)
	if err == nil {
		_ = s.RecordActivity(ctx, "", "unassigned", "Unassigned save discovered: "+relativePath, endpointID)
	}
	return err
}

// RecordRejectedIngest preserves a bound physical occurrence and all decoded
// components without creating a revision or changing the current game head.
// It is used for structurally recognizable but incomplete emulator writes.
func (s *Store) RecordRejectedIngest(ctx context.Context, p RejectedIngestParams) error {
	if p.ObservedAt.IsZero() {
		p.ObservedAt = time.Now().UTC()
	}
	if p.Provenance == "" {
		p.Provenance = model.ProvenanceUnknown
	}
	if p.ObservationPath == "" {
		p.ObservationPath = p.Binding.RelativePath
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, blob := range []archive.Blob{p.ObservedBlob, p.BatteryBlob} {
		if blob.Hash == "" {
			continue
		}
		if _, err = tx.ExecContext(ctx, "INSERT OR IGNORE INTO blobs (hash,size,created_at) VALUES (?,?,?)", blob.Hash, blob.Size, p.ObservedAt); err != nil {
			return err
		}
	}
	if p.RTCBlob != nil && p.RTCBlob.Hash != "" {
		if _, err = tx.ExecContext(ctx, "INSERT OR IGNORE INTO blobs (hash,size,created_at) VALUES (?,?,?)", p.RTCBlob.Hash, p.RTCBlob.Size, p.ObservedAt); err != nil {
			return err
		}
	}

	if _, err = tx.ExecContext(ctx, `INSERT INTO observations (id,game_id,endpoint_id,relative_path,blob_hash,action,source_modified_at,observed_at,provenance,detail)
		VALUES (?,?,?,?,?,'quarantined',?,?,?,?)`, ids.New(), p.Binding.GameID, p.Binding.EndpointID, p.ObservationPath, p.ObservedBlob.Hash, p.SourceModified, p.ObservedAt, p.Provenance, p.Detail); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO unassigned_files (id,endpoint_id,relative_path,blob_hash,size,source_modified_at,observed_at,provenance,state,detail) VALUES (?,?,?,?,?,?,?,?,'unassigned',?)
		ON CONFLICT(endpoint_id,relative_path) DO UPDATE SET blob_hash=excluded.blob_hash,size=excluded.size,source_modified_at=excluded.source_modified_at,observed_at=excluded.observed_at,provenance=excluded.provenance,state='unassigned',detail=excluded.detail`,
		ids.New(), p.Binding.EndpointID, p.ObservationPath, p.ObservedBlob.Hash, p.ObservedBlob.Size, p.SourceModified, p.ObservedAt, p.Provenance, p.Detail); err != nil {
		return err
	}
	if err = insertActivityTx(ctx, tx, p.Binding.GameID, "quarantine", "Incomplete save write archived; delivery paused", p.Binding.EndpointID, p.ObservedAt); err != nil {
		return err
	}
	return tx.Commit()
}

// RecordQuarantine persists a file problem even when no safe non-empty blob
// could be archived (for example a zero-byte or locked save). This makes the
// condition visible after the transient event stream has gone away.
func (s *Store) RecordQuarantine(ctx context.Context, endpointID, relativePath string, size int64, sourceModified *time.Time, provenance model.Provenance, detail string) error {
	now := time.Now().UTC()
	if provenance == "" {
		provenance = model.ProvenanceUnknown
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO unassigned_files (id,endpoint_id,relative_path,size,source_modified_at,observed_at,provenance,state,detail) VALUES (?,?,?,?,?,?,?,'unassigned',?)
		ON CONFLICT(endpoint_id,relative_path) DO UPDATE SET blob_hash=NULL,size=excluded.size,source_modified_at=excluded.source_modified_at,observed_at=excluded.observed_at,provenance=excluded.provenance,state='unassigned',detail=excluded.detail`,
		ids.New(), endpointID, relativePath, size, sourceModified, now, provenance, detail)
	if err == nil {
		_ = s.RecordActivity(ctx, "", "quarantine", "Quarantined file: "+relativePath+" ("+detail+")", endpointID)
	}
	return err
}

func (s *Store) RecordDeletion(ctx context.Context, endpointID, relativePath string, sourceModified *time.Time, provenance model.Provenance) error {
	binding, err := s.FindBinding(ctx, endpointID, relativePath)
	if err != nil && !errors.Is(err, ErrNotFound) {
		return err
	}
	gameID := ""
	if err == nil {
		gameID = binding.GameID
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var latest string
	err = tx.QueryRowContext(ctx, `SELECT action FROM observations WHERE endpoint_id=? AND relative_path=? ORDER BY observed_at DESC LIMIT 1`, endpointID, relativePath).Scan(&latest)
	if err != nil && err != sql.ErrNoRows {
		return err
	}
	if latest == "missing" {
		return tx.Commit()
	}
	now := time.Now().UTC()
	if _, err = tx.ExecContext(ctx, `INSERT INTO observations (id,game_id,endpoint_id,relative_path,action,source_modified_at,observed_at,provenance,detail) VALUES (?,?,?,?, 'missing',?,?,?,'Deletion is not propagated automatically')`, ids.New(), nullString(gameID), endpointID, relativePath, sourceModified, now, provenance); err != nil {
		return err
	}
	if err = insertActivityTx(ctx, tx, gameID, "missing", "Save is missing on "+endpointID, endpointID, now); err != nil {
		return err
	}
	return tx.Commit()
}

func insertActivityTx(ctx context.Context, tx *sql.Tx, gameID, kind, summary, endpointID string, at time.Time) error {
	_, err := tx.ExecContext(ctx, "INSERT INTO activity (id,game_id,kind,summary,endpoint_id,created_at) VALUES (?,?,?,?,?,?)", ids.New(), nullString(gameID), kind, summary, nullString(endpointID), at)
	return err
}

// CreateOperation journals an exact materialized target. Legacy callers may
// pass only a blob hash as profileOrHash. Component-aware callers pass the
// target profile followed by the materialized archive blob.
func (s *Store) CreateOperation(ctx context.Context, gameID, revisionID, targetEndpoint, path, profileOrHash string, materialized ...archive.Blob) (model.BrokerOperation, error) {
	var profileID string
	var target archive.Blob
	switch len(materialized) {
	case 0:
		target.Hash = profileOrHash
	case 1:
		profileID = profileOrHash
		target = materialized[0]
	default:
		return model.BrokerOperation{}, errors.New("exactly one materialized blob is allowed")
	}
	if target.Hash == "" {
		return model.BrokerOperation{}, errors.New("materialized blob hash is required")
	}
	now := time.Now().UTC()
	keyInput := revisionID + "\x00" + targetEndpoint + "\x00" + path
	if len(materialized) == 1 {
		keyInput = "thorsync-operation-v2\x00" + keyInput + "\x00" + profileID + "\x00" + target.Hash
	}
	digest := sha256.Sum256([]byte(keyInput))
	key := hex.EncodeToString(digest[:])
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return model.BrokerOperation{}, err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO blobs (hash,size,created_at) VALUES (?,?,?)`, target.Hash, target.Size, now); err != nil {
		return model.BrokerOperation{}, err
	}
	if existing, err := operationByKey(ctx, tx, key); err == nil {
		if err = tx.Commit(); err != nil {
			return model.BrokerOperation{}, err
		}
		return existing, nil
	} else if err != sql.ErrNoRows {
		return model.BrokerOperation{}, err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE broker_operations SET state='superseded',updated_at=? WHERE target_endpoint_id=? AND relative_path=? AND state IN ('pending','written')`, now, targetEndpoint, path); err != nil {
		return model.BrokerOperation{}, err
	}
	op := model.BrokerOperation{ID: ids.New(), GameID: gameID, RevisionID: revisionID, TargetEndpointID: targetEndpoint, RelativePath: path, ProfileID: profileID, BlobHash: target.Hash, State: "pending", CreatedAt: now, UpdatedAt: now}
	if _, err = tx.ExecContext(ctx, `INSERT INTO broker_operations (id,idempotency_key,game_id,revision_id,target_endpoint_id,relative_path,profile_id,blob_hash,state,created_at,updated_at) VALUES (?,?,?,?,?,?,?,?,?,?,?)`, op.ID, key, op.GameID, op.RevisionID, op.TargetEndpointID, op.RelativePath, op.ProfileID, op.BlobHash, op.State, now, now); err != nil {
		return model.BrokerOperation{}, err
	}
	if err = tx.Commit(); err != nil {
		return model.BrokerOperation{}, err
	}
	return op, nil
}

type operationScanner interface{ Scan(...any) error }

func operationByKey(ctx context.Context, queryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, key string) (model.BrokerOperation, error) {
	var op model.BrokerOperation
	err := queryer.QueryRowContext(ctx, `SELECT id,game_id,revision_id,target_endpoint_id,relative_path,profile_id,blob_hash,state,error,created_at,updated_at FROM broker_operations WHERE idempotency_key=?`, key).Scan(&op.ID, &op.GameID, &op.RevisionID, &op.TargetEndpointID, &op.RelativePath, &op.ProfileID, &op.BlobHash, &op.State, &op.Error, &op.CreatedAt, &op.UpdatedAt)
	return op, err
}

// CompleteEndpointDeliveries advances device baselines only after Syncthing
// reports that the actual remote endpoint has reached 100% completion.
func (s *Store) CompleteEndpointDeliveries(ctx context.Context, endpointID string) (int, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	rows, err := tx.QueryContext(ctx, `SELECT id,game_id,revision_id,relative_path,profile_id FROM broker_operations WHERE target_endpoint_id=? AND state='written' ORDER BY created_at`, endpointID)
	if err != nil {
		return 0, err
	}
	type delivery struct{ id, gameID, revisionID, path, profileID string }
	var deliveries []delivery
	for rows.Next() {
		var item delivery
		if err = rows.Scan(&item.id, &item.gameID, &item.revisionID, &item.path, &item.profileID); err != nil {
			rows.Close()
			return 0, err
		}
		deliveries = append(deliveries, item)
	}
	if err = rows.Close(); err != nil {
		return 0, err
	}
	now := time.Now().UTC()
	for _, item := range deliveries {
		if _, err = tx.ExecContext(ctx, `UPDATE broker_operations SET state='delivered',updated_at=? WHERE id=? AND state='written'`, now, item.id); err != nil {
			return 0, err
		}
		if _, err = tx.ExecContext(ctx, `UPDATE save_bindings SET last_deployed_revision_id=?,updated_at=? WHERE endpoint_id=? AND relative_path=? AND (?='' OR profile_id=?)`, item.revisionID, now, endpointID, item.path, item.profileID, item.profileID); err != nil {
			return 0, err
		}
		if err = insertActivityTx(ctx, tx, item.gameID, "delivery", "Save delivered to "+endpointID, endpointID, now); err != nil {
			return 0, err
		}
	}
	if err = tx.Commit(); err != nil {
		return 0, err
	}
	return len(deliveries), nil
}

func (s *Store) UpdateOperation(ctx context.Context, id, state, errorText string) error {
	_, err := s.db.ExecContext(ctx, "UPDATE broker_operations SET state=?,error=?,updated_at=? WHERE id=?", state, errorText, time.Now().UTC(), id)
	return err
}

func (s *Store) MarkBindingDeployed(ctx context.Context, bindingID, revisionID string) error {
	_, err := s.db.ExecContext(ctx, "UPDATE save_bindings SET last_deployed_revision_id=?,updated_at=? WHERE id=?", revisionID, time.Now().UTC(), bindingID)
	return err
}

type HeadMutation struct{ GameID, SourceRevisionID, ExpectedHeadID, IdempotencyKey, Actor, Kind string }

func copyRevisionPayloadTx(ctx context.Context, tx *sql.Tx, sourceRevisionID, targetRevisionID string) (model.RevisionPayload, error) {
	var payload model.RevisionPayload
	var rtcHash sql.NullString
	err := tx.QueryRowContext(ctx, `SELECT r.blob_hash,r.size,rp.battery_blob_hash,rp.battery_size,rp.rtc_blob_hash,rp.rtc_size,rp.content_hash
		FROM revisions r LEFT JOIN revision_payloads rp ON rp.revision_id=r.id WHERE r.id=?`, sourceRevisionID).Scan(
		new(string), new(int64), &payload.BatteryBlobHash, &payload.BatterySize, &rtcHash, &payload.RTCSize, &payload.ContentHash)
	if err != nil {
		// The normal migration guarantees a payload row. Keep this fallback for a
		// partially restored database so a legacy revision is still recoverable.
		var blobHash string
		var size int64
		if fallbackErr := tx.QueryRowContext(ctx, `SELECT blob_hash,size FROM revisions WHERE id=?`, sourceRevisionID).Scan(&blobHash, &size); fallbackErr != nil {
			return model.RevisionPayload{}, err
		}
		payload.BatteryBlobHash = blobHash
		payload.BatterySize = size
		payload.ContentHash = blobHash
	}
	if rtcHash.Valid {
		payload.RTCBlobHash = rtcHash.String
	}
	payload.RevisionID = targetRevisionID
	if _, err = tx.ExecContext(ctx, `INSERT INTO revision_payloads (revision_id,battery_blob_hash,battery_size,rtc_blob_hash,rtc_size,content_hash) VALUES (?,?,?,?,?,?)`,
		payload.RevisionID, payload.BatteryBlobHash, payload.BatterySize, nullString(payload.RTCBlobHash), payload.RTCSize, payload.ContentHash); err != nil {
		return model.RevisionPayload{}, err
	}
	return payload, nil
}

func (s *Store) CreateHeadFromExisting(ctx context.Context, p HeadMutation) (model.Revision, error) {
	if p.IdempotencyKey == "" {
		return model.Revision{}, errors.New("idempotency key is required")
	}
	if p.Kind != "restore" && p.Kind != "promote" {
		return model.Revision{}, errors.New("invalid mutation kind")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return model.Revision{}, err
	}
	defer tx.Rollback()
	var existing, existingGame string
	err = tx.QueryRowContext(ctx, "SELECT game_id,revision_id FROM mutation_requests WHERE idempotency_key=?", p.IdempotencyKey).Scan(&existingGame, &existing)
	if err == nil {
		if existingGame != p.GameID {
			return model.Revision{}, fmt.Errorf("%w: idempotency key was used for a different game", ErrConflict)
		}
		if err = tx.Commit(); err != nil {
			return model.Revision{}, err
		}
		revision, lookupErr := s.Revision(ctx, existing)
		if lookupErr != nil {
			return model.Revision{}, lookupErr
		}
		if revision.Kind != p.Kind || (p.Kind == "restore" && revision.RestoredFromID != p.SourceRevisionID) || (p.Kind == "promote" && revision.PromotedFromID != p.SourceRevisionID) {
			return model.Revision{}, fmt.Errorf("%w: idempotency key was reused for a different mutation", ErrConflict)
		}
		return revision, nil
	}
	if err != sql.ErrNoRows {
		return model.Revision{}, err
	}
	var current string
	if err = tx.QueryRowContext(ctx, "SELECT COALESCE(current_revision_id,'') FROM games WHERE id=?", p.GameID).Scan(&current); err == sql.ErrNoRows {
		return model.Revision{}, ErrNotFound
	}
	if err != nil {
		return model.Revision{}, err
	}
	if current != p.ExpectedHeadID {
		return model.Revision{}, fmt.Errorf("%w: current head changed", ErrConflict)
	}
	var source model.Revision
	var modified sql.NullTime
	var provenance string
	err = tx.QueryRowContext(ctx, `SELECT id,game_id,COALESCE(parent_revision_id,''),COALESCE(restored_from_id,''),COALESCE(promoted_from_id,''),blob_hash,size,COALESCE(source_endpoint_id,''),source_modified_at,observed_at,provenance,kind,state,actor FROM revisions WHERE id=? AND game_id=?`, p.SourceRevisionID, p.GameID).Scan(&source.ID, &source.GameID, &source.ParentRevisionID, &source.RestoredFromID, &source.PromotedFromID, &source.BlobHash, &source.Size, &source.SourceEndpointID, &modified, &source.ObservedAt, &provenance, &source.Kind, &source.State, &source.Actor)
	if err == sql.ErrNoRows {
		return model.Revision{}, ErrNotFound
	}
	if err != nil {
		return model.Revision{}, err
	}
	now := time.Now().UTC()
	revision := model.Revision{ID: ids.New(), GameID: p.GameID, ParentRevisionID: current, BlobHash: source.BlobHash, Size: source.Size, ObservedAt: now, Provenance: model.ProvenanceConfirmed, Kind: p.Kind, State: "head", Actor: p.Actor}
	if p.Kind == "restore" {
		revision.RestoredFromID = source.ID
	} else {
		revision.PromotedFromID = source.ID
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO revisions (id,game_id,parent_revision_id,restored_from_id,promoted_from_id,blob_hash,size,observed_at,provenance,kind,state,actor) VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`, revision.ID, revision.GameID, nullString(revision.ParentRevisionID), nullString(revision.RestoredFromID), nullString(revision.PromotedFromID), revision.BlobHash, revision.Size, now, revision.Provenance, revision.Kind, revision.State, revision.Actor)
	if err != nil {
		return model.Revision{}, err
	}
	payload, err := copyRevisionPayloadTx(ctx, tx, source.ID, revision.ID)
	if err != nil {
		return model.Revision{}, err
	}
	revision.ContentHash = payload.ContentHash
	revision.HasRTC = payload.RTCBlobHash != ""
	if current != "" {
		if _, err = tx.ExecContext(ctx, "UPDATE revisions SET state='history' WHERE id=? AND state='head'", current); err != nil {
			return model.Revision{}, err
		}
	}
	if _, err = tx.ExecContext(ctx, "UPDATE games SET current_revision_id=?,status='healthy',updated_at=? WHERE id=?", revision.ID, now, p.GameID); err != nil {
		return model.Revision{}, err
	}
	if _, err = tx.ExecContext(ctx, "UPDATE conflicts SET state='resolved',resolved_at=? WHERE game_id=? AND state='open'", now, p.GameID); err != nil {
		return model.Revision{}, err
	}
	if _, err = tx.ExecContext(ctx, "INSERT INTO mutation_requests (idempotency_key,game_id,revision_id,created_at) VALUES (?,?,?,?)", p.IdempotencyKey, p.GameID, revision.ID, now); err != nil {
		return model.Revision{}, err
	}
	if err = insertActivityTx(ctx, tx, p.GameID, p.Kind, strings.Title(p.Kind)+" created a new save head", "", now); err != nil {
		return model.Revision{}, err
	}
	if err = tx.Commit(); err != nil {
		return model.Revision{}, err
	}
	return revision, nil
}

// CreateSnapshot records a user-named point in history without changing the
// bytes deployed to any device. Bindings which already had the previous head
// advance to the snapshot revision because their physical content is exactly
// the same; offline or stale bindings retain their older baseline.
func (s *Store) CreateSnapshot(ctx context.Context, gameID, expectedHeadID, idempotencyKey, actor string) (model.Revision, error) {
	if idempotencyKey == "" {
		return model.Revision{}, errors.New("idempotency key is required")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return model.Revision{}, err
	}
	defer tx.Rollback()
	var existing, existingGame string
	err = tx.QueryRowContext(ctx, "SELECT game_id,revision_id FROM mutation_requests WHERE idempotency_key=?", idempotencyKey).Scan(&existingGame, &existing)
	if err == nil {
		if existingGame != gameID {
			return model.Revision{}, fmt.Errorf("%w: idempotency key was used for a different game", ErrConflict)
		}
		if err = tx.Commit(); err != nil {
			return model.Revision{}, err
		}
		revision, lookupErr := s.Revision(ctx, existing)
		if lookupErr != nil {
			return model.Revision{}, lookupErr
		}
		if revision.Kind != "snapshot" {
			return model.Revision{}, fmt.Errorf("%w: idempotency key was reused for a different mutation", ErrConflict)
		}
		return revision, nil
	}
	if err != sql.ErrNoRows {
		return model.Revision{}, err
	}
	var currentID, blobHash string
	var size int64
	err = tx.QueryRowContext(ctx, `SELECT COALESCE(g.current_revision_id,''),COALESCE(r.blob_hash,''),COALESCE(r.size,0) FROM games g LEFT JOIN revisions r ON r.id=g.current_revision_id WHERE g.id=?`, gameID).Scan(&currentID, &blobHash, &size)
	if err == sql.ErrNoRows {
		return model.Revision{}, ErrNotFound
	}
	if err != nil {
		return model.Revision{}, err
	}
	if currentID == "" {
		return model.Revision{}, fmt.Errorf("%w: game has no revision to snapshot", ErrConflict)
	}
	if currentID != expectedHeadID {
		return model.Revision{}, fmt.Errorf("%w: current head changed", ErrConflict)
	}
	var openConflicts int
	if err = tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM conflicts WHERE game_id=? AND state='open'", gameID).Scan(&openConflicts); err != nil {
		return model.Revision{}, err
	}
	if openConflicts > 0 {
		return model.Revision{}, fmt.Errorf("%w: resolve conflicts before taking a snapshot", ErrConflict)
	}
	now := time.Now().UTC()
	revision := model.Revision{ID: ids.New(), GameID: gameID, ParentRevisionID: currentID, BlobHash: blobHash, Size: size, ObservedAt: now, Provenance: model.ProvenanceConfirmed, Kind: "snapshot", State: "head", Actor: actor}
	if _, err = tx.ExecContext(ctx, `INSERT INTO revisions (id,game_id,parent_revision_id,blob_hash,size,observed_at,provenance,kind,state,actor) VALUES (?,?,?,?,?,?,?,?,?,?)`, revision.ID, gameID, currentID, blobHash, size, now, revision.Provenance, revision.Kind, revision.State, actor); err != nil {
		return model.Revision{}, err
	}
	payload, err := copyRevisionPayloadTx(ctx, tx, currentID, revision.ID)
	if err != nil {
		return model.Revision{}, err
	}
	revision.ContentHash = payload.ContentHash
	revision.HasRTC = payload.RTCBlobHash != ""
	if _, err = tx.ExecContext(ctx, "UPDATE revisions SET state='history' WHERE id=? AND state='head'", currentID); err != nil {
		return model.Revision{}, err
	}
	if _, err = tx.ExecContext(ctx, "UPDATE games SET current_revision_id=?,updated_at=? WHERE id=?", revision.ID, now, gameID); err != nil {
		return model.Revision{}, err
	}
	if _, err = tx.ExecContext(ctx, "UPDATE save_bindings SET last_deployed_revision_id=?,updated_at=? WHERE game_id=? AND last_deployed_revision_id=?", revision.ID, now, gameID, currentID); err != nil {
		return model.Revision{}, err
	}
	if _, err = tx.ExecContext(ctx, "INSERT INTO mutation_requests (idempotency_key,game_id,revision_id,created_at) VALUES (?,?,?,?)", idempotencyKey, gameID, revision.ID, now); err != nil {
		return model.Revision{}, err
	}
	if err = insertActivityTx(ctx, tx, gameID, "snapshot", "Manual snapshot created", "", now); err != nil {
		return model.Revision{}, err
	}
	if err = tx.Commit(); err != nil {
		return model.Revision{}, err
	}
	return revision, nil
}
