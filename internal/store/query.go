package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/apgul/thorsync/internal/adapter"
	"github.com/apgul/thorsync/internal/ids"
	"github.com/apgul/thorsync/internal/model"
)

var ErrNotFound = errors.New("not found")
var ErrConflict = errors.New("conflict")

func (s *Store) CreateGame(ctx context.Context, title string, platform model.Platform, crc32, sha1 string) (model.Game, error) {
	title = strings.TrimSpace(title)
	crc32 = strings.ToUpper(strings.TrimSpace(strings.TrimPrefix(crc32, "0x")))
	sha1 = strings.ToLower(strings.TrimSpace(sha1))
	if title == "" {
		return model.Game{}, errors.New("title is required")
	}
	if platform != model.PlatformGBA && platform != model.PlatformNDS {
		return model.Game{}, errors.New("platform must be gba or nds")
	}
	if existing, err := s.findGameByHash(ctx, platform, crc32, sha1); err == nil {
		return existing, nil
	} else if !errors.Is(err, ErrNotFound) {
		return model.Game{}, err
	}
	now := time.Now().UTC()
	game := model.Game{ID: ids.New(), Title: title, Platform: platform, CRC32: crc32, SHA1: sha1, Status: "unconfigured", CreatedAt: now, UpdatedAt: now}
	_, err := s.db.ExecContext(ctx, `INSERT INTO games (id,title,platform,crc32,sha1,status,created_at,updated_at) VALUES (?,?,?,?,?,?,?,?)`,
		game.ID, game.Title, game.Platform, game.CRC32, game.SHA1, game.Status, now, now)
	return game, err
}

func (s *Store) findGameByHash(ctx context.Context, platform model.Platform, crc32, sha1 string) (model.Game, error) {
	if crc32 == "" && sha1 == "" {
		return model.Game{}, ErrNotFound
	}
	query := `SELECT id,title,platform,crc32,sha1,artwork_path,COALESCE(current_revision_id,''),status,created_at,updated_at
		FROM games WHERE platform=? AND ((?<>'' AND crc32=?) OR (?<>'' AND sha1=?)) LIMIT 1`
	return scanBaseGame(s.db.QueryRowContext(ctx, query, platform, crc32, crc32, sha1, sha1))
}

type rowScanner interface{ Scan(...any) error }

func scanBaseGame(row rowScanner) (model.Game, error) {
	var game model.Game
	var platform, artwork, current string
	if err := row.Scan(&game.ID, &game.Title, &platform, &game.CRC32, &game.SHA1, &artwork, &current, &game.Status, &game.CreatedAt, &game.UpdatedAt); err != nil {
		if err == sql.ErrNoRows {
			return model.Game{}, ErrNotFound
		}
		return model.Game{}, err
	}
	game.Platform = model.Platform(platform)
	game.CurrentRevisionID = current
	if artwork != "" {
		game.ArtworkURL = "/artwork/" + filepath.Base(artwork)
	}
	return game, nil
}

func (s *Store) ListGames(ctx context.Context, search string, platform model.Platform) ([]model.Game, error) {
	args := []any{}
	where := []string{"1=1"}
	if strings.TrimSpace(search) != "" {
		where = append(where, "LOWER(g.title) LIKE ?")
		args = append(args, "%"+strings.ToLower(strings.TrimSpace(search))+"%")
	}
	if platform == model.PlatformGBA || platform == model.PlatformNDS {
		where = append(where, "g.platform=?")
		args = append(args, platform)
	}
	query := `SELECT g.id,g.title,g.platform,g.crc32,g.sha1,g.artwork_path,COALESCE(g.current_revision_id,''),g.status,g.created_at,g.updated_at,
		COALESCE(r.blob_hash,''),COALESCE(r.source_endpoint_id,''),r.source_modified_at,r.observed_at,COALESCE(r.provenance,''),
		(SELECT COUNT(*) FROM conflicts c WHERE c.game_id=g.id AND c.state='open')
		FROM games g LEFT JOIN revisions r ON r.id=g.current_revision_id WHERE ` + strings.Join(where, " AND ") + ` ORDER BY COALESCE(r.observed_at,g.updated_at) DESC,g.title`
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	games := []model.Game{}
	for rows.Next() {
		var game model.Game
		var platformRaw, artwork, current, provenance string
		var modified, observed sql.NullTime
		if err := rows.Scan(&game.ID, &game.Title, &platformRaw, &game.CRC32, &game.SHA1, &artwork, &current, &game.Status, &game.CreatedAt, &game.UpdatedAt,
			&game.CurrentHash, &game.SourceEndpointID, &modified, &observed, &provenance, &game.ConflictCount); err != nil {
			return nil, err
		}
		game.Platform = model.Platform(platformRaw)
		game.CurrentRevisionID = current
		game.Provenance = model.Provenance(provenance)
		if modified.Valid {
			value := modified.Time
			game.SourceModifiedAt = &value
		}
		if observed.Valid {
			value := observed.Time
			game.ObservedAt = &value
		}
		if artwork != "" {
			game.ArtworkURL = "/artwork/" + filepath.Base(artwork)
		}
		games = append(games, game)
	}
	return games, rows.Err()
}

func (s *Store) GetGame(ctx context.Context, id string) (model.GameDetail, error) {
	base, err := scanBaseGame(s.db.QueryRowContext(ctx, `SELECT id,title,platform,crc32,sha1,artwork_path,COALESCE(current_revision_id,''),status,created_at,updated_at FROM games WHERE id=?`, id))
	if err != nil {
		return model.GameDetail{}, err
	}
	detail := model.GameDetail{Game: base}
	if detail.Bindings, err = s.listBindings(ctx, id); err != nil {
		return model.GameDetail{}, err
	}
	if detail.Revisions, err = s.listRevisions(ctx, id); err != nil {
		return model.GameDetail{}, err
	}
	if detail.Observations, err = s.listObservations(ctx, id); err != nil {
		return model.GameDetail{}, err
	}
	if detail.Conflicts, err = s.listConflicts(ctx, id, ""); err != nil {
		return model.GameDetail{}, err
	}
	if detail.Operations, err = s.listOperations(ctx, id); err != nil {
		return model.GameDetail{}, err
	}
	if base.CurrentRevisionID != "" {
		for _, revision := range detail.Revisions {
			if revision.ID == base.CurrentRevisionID {
				detail.CurrentHash = revision.BlobHash
				detail.SourceEndpointID = revision.SourceEndpointID
				detail.SourceModifiedAt = revision.SourceModifiedAt
				value := revision.ObservedAt
				detail.ObservedAt = &value
				detail.Provenance = revision.Provenance
				break
			}
		}
	}
	detail.ConflictCount = 0
	for _, conflict := range detail.Conflicts {
		if conflict.State == "open" {
			detail.ConflictCount++
		}
	}
	return detail, nil
}

func (s *Store) listBindings(ctx context.Context, gameID string) ([]model.SaveBinding, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id,game_id,endpoint_id,profile_id,relative_path,COALESCE(last_deployed_revision_id,''),enabled FROM save_bindings WHERE game_id=? ORDER BY endpoint_id`, gameID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []model.SaveBinding{}
	for rows.Next() {
		var binding model.SaveBinding
		if err := rows.Scan(&binding.ID, &binding.GameID, &binding.EndpointID, &binding.ProfileID, &binding.RelativePath, &binding.LastDeployedRevisionID, &binding.Enabled); err != nil {
			return nil, err
		}
		result = append(result, binding)
	}
	return result, rows.Err()
}

func (s *Store) ListBindings(ctx context.Context, gameID string) ([]model.SaveBinding, error) {
	return s.listBindings(ctx, gameID)
}

func (s *Store) ListEndpointBindings(ctx context.Context, endpointID string) ([]model.SaveBinding, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id,game_id,endpoint_id,profile_id,relative_path,COALESCE(last_deployed_revision_id,''),enabled FROM save_bindings WHERE endpoint_id=? AND enabled=1 ORDER BY relative_path`, endpointID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []model.SaveBinding{}
	for rows.Next() {
		var binding model.SaveBinding
		if err := rows.Scan(&binding.ID, &binding.GameID, &binding.EndpointID, &binding.ProfileID, &binding.RelativePath, &binding.LastDeployedRevisionID, &binding.Enabled); err != nil {
			return nil, err
		}
		result = append(result, binding)
	}
	return result, rows.Err()
}

func (s *Store) FindBinding(ctx context.Context, endpointID, relativePath string) (model.SaveBinding, error) {
	var binding model.SaveBinding
	err := s.db.QueryRowContext(ctx, `SELECT id,game_id,endpoint_id,profile_id,relative_path,COALESCE(last_deployed_revision_id,''),enabled FROM save_bindings WHERE endpoint_id=? AND relative_path=?`, endpointID, filepath.ToSlash(relativePath)).Scan(
		&binding.ID, &binding.GameID, &binding.EndpointID, &binding.ProfileID, &binding.RelativePath, &binding.LastDeployedRevisionID, &binding.Enabled)
	if err == sql.ErrNoRows {
		return model.SaveBinding{}, ErrNotFound
	}
	return binding, err
}

func (s *Store) UpsertBinding(ctx context.Context, binding model.SaveBinding) (model.SaveBinding, error) {
	profile, ok := adapter.Profile(binding.ProfileID)
	if !ok || profile.EndpointID != binding.EndpointID {
		return model.SaveBinding{}, errors.New("profile does not belong to endpoint")
	}
	clean, err := cleanRelative(binding.RelativePath)
	if err != nil {
		return model.SaveBinding{}, err
	}
	var platform string
	if err := s.db.QueryRowContext(ctx, "SELECT platform FROM games WHERE id=?", binding.GameID).Scan(&platform); err != nil {
		if err == sql.ErrNoRows {
			return model.SaveBinding{}, ErrNotFound
		}
		return model.SaveBinding{}, err
	}
	if profile.Platform != model.Platform(platform) {
		return model.SaveBinding{}, errors.New("profile platform does not match game")
	}
	now := time.Now().UTC()
	if binding.ID == "" {
		binding.ID = ids.New()
	}
	binding.RelativePath = clean
	binding.Enabled = true
	_, err = s.db.ExecContext(ctx, `INSERT INTO save_bindings (id,game_id,endpoint_id,profile_id,relative_path,enabled,created_at,updated_at)
		VALUES (?,?,?,?,?,1,?,?) ON CONFLICT(game_id,endpoint_id) DO UPDATE SET profile_id=excluded.profile_id,relative_path=excluded.relative_path,enabled=1,updated_at=excluded.updated_at`,
		binding.ID, binding.GameID, binding.EndpointID, binding.ProfileID, binding.RelativePath, now, now)
	if err != nil {
		return model.SaveBinding{}, err
	}
	_, _ = s.db.ExecContext(ctx, "UPDATE unassigned_files SET state='mapped' WHERE endpoint_id=? AND relative_path=?", binding.EndpointID, binding.RelativePath)
	return s.FindBinding(ctx, binding.EndpointID, binding.RelativePath)
}

func cleanRelative(path string) (string, error) {
	path = filepath.ToSlash(strings.TrimSpace(path))
	clean := filepath.ToSlash(filepath.Clean(path))
	drivePath := len(clean) >= 2 && ((clean[0] >= 'A' && clean[0] <= 'Z') || (clean[0] >= 'a' && clean[0] <= 'z')) && clean[1] == ':'
	if clean == "." || clean == "" || strings.HasPrefix(clean, "../") || strings.HasPrefix(clean, "/") || drivePath || filepath.IsAbs(filepath.FromSlash(clean)) || strings.Contains(clean, "\x00") {
		return "", errors.New("path must be a safe relative path")
	}
	return clean, nil
}

func (s *Store) listRevisions(ctx context.Context, gameID string) ([]model.Revision, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id,game_id,COALESCE(parent_revision_id,''),COALESCE(restored_from_id,''),COALESCE(promoted_from_id,''),blob_hash,size,COALESCE(source_endpoint_id,''),source_modified_at,observed_at,provenance,kind,state,actor FROM revisions WHERE game_id=? ORDER BY observed_at DESC`, gameID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []model.Revision{}
	for rows.Next() {
		var item model.Revision
		var modified sql.NullTime
		var provenance string
		if err := rows.Scan(&item.ID, &item.GameID, &item.ParentRevisionID, &item.RestoredFromID, &item.PromotedFromID, &item.BlobHash, &item.Size, &item.SourceEndpointID, &modified, &item.ObservedAt, &provenance, &item.Kind, &item.State, &item.Actor); err != nil {
			return nil, err
		}
		item.Provenance = model.Provenance(provenance)
		if modified.Valid {
			v := modified.Time
			item.SourceModifiedAt = &v
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (s *Store) listObservations(ctx context.Context, gameID string) ([]model.Observation, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id,COALESCE(game_id,''),COALESCE(revision_id,''),endpoint_id,relative_path,COALESCE(blob_hash,''),action,source_modified_at,observed_at,provenance,detail FROM observations WHERE game_id=? ORDER BY observed_at DESC LIMIT 250`, gameID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []model.Observation{}
	for rows.Next() {
		var item model.Observation
		var modified sql.NullTime
		var provenance string
		if err := rows.Scan(&item.ID, &item.GameID, &item.RevisionID, &item.EndpointID, &item.RelativePath, &item.BlobHash, &item.Action, &modified, &item.ObservedAt, &provenance, &item.Detail); err != nil {
			return nil, err
		}
		item.Provenance = model.Provenance(provenance)
		if modified.Valid {
			v := modified.Time
			item.SourceModifiedAt = &v
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (s *Store) listConflicts(ctx context.Context, gameID, state string) ([]model.Conflict, error) {
	where := "1=1"
	args := []any{}
	if gameID != "" {
		where += " AND game_id=?"
		args = append(args, gameID)
	}
	if state != "" {
		where += " AND state=?"
		args = append(args, state)
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id,game_id,COALESCE(head_revision_id,''),branch_revision_id,reason,state,created_at,resolved_at FROM conflicts WHERE `+where+` ORDER BY created_at DESC`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []model.Conflict{}
	for rows.Next() {
		var item model.Conflict
		var resolved sql.NullTime
		if err := rows.Scan(&item.ID, &item.GameID, &item.HeadRevisionID, &item.BranchRevisionID, &item.Reason, &item.State, &item.CreatedAt, &resolved); err != nil {
			return nil, err
		}
		if resolved.Valid {
			v := resolved.Time
			item.ResolvedAt = &v
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (s *Store) ListConflicts(ctx context.Context, state string) ([]model.Conflict, error) {
	return s.listConflicts(ctx, "", state)
}

func (s *Store) Conflict(ctx context.Context, id string) (model.Conflict, error) {
	var item model.Conflict
	var resolved sql.NullTime
	err := s.db.QueryRowContext(ctx, `SELECT id,game_id,COALESCE(head_revision_id,''),branch_revision_id,reason,state,created_at,resolved_at FROM conflicts WHERE id=?`, id).Scan(&item.ID, &item.GameID, &item.HeadRevisionID, &item.BranchRevisionID, &item.Reason, &item.State, &item.CreatedAt, &resolved)
	if err == sql.ErrNoRows {
		return model.Conflict{}, ErrNotFound
	}
	if err != nil {
		return model.Conflict{}, err
	}
	if resolved.Valid {
		v := resolved.Time
		item.ResolvedAt = &v
	}
	return item, nil
}

func (s *Store) listOperations(ctx context.Context, gameID string) ([]model.BrokerOperation, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id,game_id,revision_id,target_endpoint_id,relative_path,blob_hash,state,error,created_at,updated_at FROM broker_operations WHERE game_id=? ORDER BY created_at DESC LIMIT 250`, gameID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []model.BrokerOperation{}
	for rows.Next() {
		var item model.BrokerOperation
		if err := rows.Scan(&item.ID, &item.GameID, &item.RevisionID, &item.TargetEndpointID, &item.RelativePath, &item.BlobHash, &item.State, &item.Error, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (s *Store) ListEndpoints(ctx context.Context) ([]model.Endpoint, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id,name,kind,device_id,folder_id,root_path,online,last_seen_at,state FROM endpoints ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []model.Endpoint{}
	for rows.Next() {
		var item model.Endpoint
		var seen sql.NullTime
		if err := rows.Scan(&item.ID, &item.Name, &item.Kind, &item.DeviceID, &item.FolderID, &item.RootPath, &item.Online, &seen, &item.State); err != nil {
			return nil, err
		}
		if seen.Valid {
			v := seen.Time
			item.LastSeenAt = &v
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (s *Store) UpdateEndpoint(ctx context.Context, id, deviceID, state string, online bool) error {
	now := time.Now().UTC()
	var seen any = nil
	if online {
		seen = now
	}
	result, err := s.db.ExecContext(ctx, `UPDATE endpoints SET device_id=CASE WHEN ?='' THEN device_id ELSE ? END,state=?,online=?,last_seen_at=COALESCE(?,last_seen_at),updated_at=? WHERE id=?`, deviceID, deviceID, state, online, seen, now, id)
	if err != nil {
		return err
	}
	count, _ := result.RowsAffected()
	if count == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) Profiles(ctx context.Context) ([]model.EmulatorProfile, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id,name,endpoint_id,platform,extension,format FROM emulator_profiles ORDER BY endpoint_id,platform`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []model.EmulatorProfile{}
	for rows.Next() {
		var item model.EmulatorProfile
		var platform string
		if err := rows.Scan(&item.ID, &item.Name, &item.EndpointID, &platform, &item.Extension, &item.Format); err != nil {
			return nil, err
		}
		item.Platform = model.Platform(platform)
		result = append(result, item)
	}
	return result, rows.Err()
}

func (s *Store) ListActivity(ctx context.Context, limit int) ([]model.Activity, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx, `SELECT a.id,COALESCE(a.game_id,''),COALESCE(g.title,''),a.kind,a.summary,COALESCE(a.endpoint_id,''),a.created_at FROM activity a LEFT JOIN games g ON g.id=a.game_id ORDER BY a.created_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []model.Activity{}
	for rows.Next() {
		var item model.Activity
		if err := rows.Scan(&item.ID, &item.GameID, &item.GameTitle, &item.Kind, &item.Summary, &item.EndpointID, &item.CreatedAt); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

type UnassignedFile struct {
	ID               string           `json:"id"`
	EndpointID       string           `json:"endpointId"`
	RelativePath     string           `json:"relativePath"`
	BlobHash         string           `json:"blobHash,omitempty"`
	Size             int64            `json:"size"`
	SourceModifiedAt *time.Time       `json:"sourceModifiedAt,omitempty"`
	ObservedAt       time.Time        `json:"observedAt"`
	Provenance       model.Provenance `json:"provenance"`
	State            string           `json:"state"`
	Detail           string           `json:"detail,omitempty"`
}

func (s *Store) ListUnassigned(ctx context.Context) ([]UnassignedFile, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id,endpoint_id,relative_path,COALESCE(blob_hash,''),size,source_modified_at,observed_at,provenance,state,detail FROM unassigned_files WHERE state='unassigned' ORDER BY observed_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []UnassignedFile{}
	for rows.Next() {
		var item UnassignedFile
		var modified sql.NullTime
		var provenance string
		if err := rows.Scan(&item.ID, &item.EndpointID, &item.RelativePath, &item.BlobHash, &item.Size, &modified, &item.ObservedAt, &provenance, &item.State, &item.Detail); err != nil {
			return nil, err
		}
		item.Provenance = model.Provenance(provenance)
		if modified.Valid {
			v := modified.Time
			item.SourceModifiedAt = &v
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (s *Store) SetArtwork(ctx context.Context, gameID, path string) error {
	result, err := s.db.ExecContext(ctx, "UPDATE games SET artwork_path=?,updated_at=? WHERE id=?", path, time.Now().UTC(), gameID)
	if err != nil {
		return err
	}
	count, _ := result.RowsAffected()
	if count == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) GamePlatform(ctx context.Context, gameID string) (model.Platform, error) {
	var value string
	err := s.db.QueryRowContext(ctx, "SELECT platform FROM games WHERE id=?", gameID).Scan(&value)
	if err == sql.ErrNoRows {
		return "", ErrNotFound
	}
	return model.Platform(value), err
}

func (s *Store) Revision(ctx context.Context, id string) (model.Revision, error) {
	var item model.Revision
	var modified sql.NullTime
	var provenance string
	err := s.db.QueryRowContext(ctx, `SELECT id,game_id,COALESCE(parent_revision_id,''),COALESCE(restored_from_id,''),COALESCE(promoted_from_id,''),blob_hash,size,COALESCE(source_endpoint_id,''),source_modified_at,observed_at,provenance,kind,state,actor FROM revisions WHERE id=?`, id).Scan(&item.ID, &item.GameID, &item.ParentRevisionID, &item.RestoredFromID, &item.PromotedFromID, &item.BlobHash, &item.Size, &item.SourceEndpointID, &modified, &item.ObservedAt, &provenance, &item.Kind, &item.State, &item.Actor)
	if err == sql.ErrNoRows {
		return model.Revision{}, ErrNotFound
	}
	if err != nil {
		return model.Revision{}, err
	}
	item.Provenance = model.Provenance(provenance)
	if modified.Valid {
		v := modified.Time
		item.SourceModifiedAt = &v
	}
	return item, nil
}

func (s *Store) RootForEndpoint(ctx context.Context, id string) (string, error) {
	var root string
	err := s.db.QueryRowContext(ctx, "SELECT root_path FROM endpoints WHERE id=?", id).Scan(&root)
	if err == sql.ErrNoRows {
		return "", ErrNotFound
	}
	return root, err
}

func (s *Store) AssertNoOpenConflict(ctx context.Context, gameID string) error {
	var count int
	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM conflicts WHERE game_id=? AND state='open'", gameID).Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return fmt.Errorf("%w: game has unresolved conflicts", ErrConflict)
	}
	return nil
}

type Counts struct {
	BlobCount         int `json:"blobCount"`
	RevisionCount     int `json:"revisionCount"`
	GameCount         int `json:"gameCount"`
	OpenConflictCount int `json:"openConflictCount"`
	UnassignedCount   int `json:"unassignedCount"`
}

func (s *Store) Counts(ctx context.Context) (Counts, error) {
	var result Counts
	queries := []struct {
		target *int
		query  string
	}{{&result.BlobCount, "SELECT COUNT(*) FROM blobs"}, {&result.RevisionCount, "SELECT COUNT(*) FROM revisions"}, {&result.GameCount, "SELECT COUNT(*) FROM games"}, {&result.OpenConflictCount, "SELECT COUNT(*) FROM conflicts WHERE state='open'"}, {&result.UnassignedCount, "SELECT COUNT(*) FROM unassigned_files WHERE state='unassigned'"}}
	for _, item := range queries {
		if err := s.db.QueryRowContext(ctx, item.query).Scan(item.target); err != nil {
			return Counts{}, err
		}
	}
	return result, nil
}
