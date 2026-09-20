package model

import "time"

type Platform string

const (
	PlatformGBA Platform = "gba"
	PlatformNDS Platform = "nds"
)

type Provenance string

const (
	ProvenanceConfirmed Provenance = "confirmed"
	ProvenanceInferred  Provenance = "inferred"
	ProvenanceUnknown   Provenance = "unknown"
)

type Game struct {
	ID                string     `json:"id"`
	Title             string     `json:"title"`
	Platform          Platform   `json:"platform"`
	CRC32             string     `json:"crc32,omitempty"`
	SHA1              string     `json:"sha1,omitempty"`
	ArtworkURL        string     `json:"artworkUrl,omitempty"`
	CurrentRevisionID string     `json:"currentRevisionId,omitempty"`
	CurrentHash       string     `json:"currentHash,omitempty"`
	SourceEndpointID  string     `json:"sourceEndpointId,omitempty"`
	SourceModifiedAt  *time.Time `json:"sourceModifiedAt,omitempty"`
	ObservedAt        *time.Time `json:"observedAt,omitempty"`
	Provenance        Provenance `json:"provenance,omitempty"`
	Status            string     `json:"status"`
	ConflictCount     int        `json:"conflictCount"`
	CreatedAt         time.Time  `json:"createdAt"`
	UpdatedAt         time.Time  `json:"updatedAt"`
}

type Endpoint struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	Kind       string     `json:"kind"`
	DeviceID   string     `json:"deviceId,omitempty"`
	FolderID   string     `json:"folderId"`
	RootPath   string     `json:"rootPath"`
	Online     bool       `json:"online"`
	LastSeenAt *time.Time `json:"lastSeenAt,omitempty"`
	State      string     `json:"state"`
}

type EmulatorProfile struct {
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	EndpointID string   `json:"endpointId"`
	Platform   Platform `json:"platform"`
	Extension  string   `json:"extension"`
	Format     string   `json:"format"`
}

type SaveBinding struct {
	ID                     string `json:"id"`
	GameID                 string `json:"gameId"`
	EndpointID             string `json:"endpointId"`
	ProfileID              string `json:"profileId"`
	RelativePath           string `json:"relativePath"`
	LastDeployedRevisionID string `json:"lastDeployedRevisionId,omitempty"`
	Enabled                bool   `json:"enabled"`
}

type Revision struct {
	ID               string     `json:"id"`
	GameID           string     `json:"gameId"`
	ParentRevisionID string     `json:"parentRevisionId,omitempty"`
	RestoredFromID   string     `json:"restoredFromId,omitempty"`
	PromotedFromID   string     `json:"promotedFromId,omitempty"`
	BlobHash         string     `json:"blobHash"`
	Size             int64      `json:"size"`
	SourceEndpointID string     `json:"sourceEndpointId,omitempty"`
	SourceModifiedAt *time.Time `json:"sourceModifiedAt,omitempty"`
	ObservedAt       time.Time  `json:"observedAt"`
	Provenance       Provenance `json:"provenance"`
	Kind             string     `json:"kind"`
	State            string     `json:"state"`
	Actor            string     `json:"actor,omitempty"`
}

type Observation struct {
	ID               string     `json:"id"`
	GameID           string     `json:"gameId"`
	RevisionID       string     `json:"revisionId,omitempty"`
	EndpointID       string     `json:"endpointId"`
	RelativePath     string     `json:"relativePath"`
	BlobHash         string     `json:"blobHash,omitempty"`
	Action           string     `json:"action"`
	SourceModifiedAt *time.Time `json:"sourceModifiedAt,omitempty"`
	ObservedAt       time.Time  `json:"observedAt"`
	Provenance       Provenance `json:"provenance"`
	Detail           string     `json:"detail,omitempty"`
}

type Conflict struct {
	ID               string     `json:"id"`
	GameID           string     `json:"gameId"`
	HeadRevisionID   string     `json:"headRevisionId,omitempty"`
	BranchRevisionID string     `json:"branchRevisionId"`
	Reason           string     `json:"reason"`
	State            string     `json:"state"`
	CreatedAt        time.Time  `json:"createdAt"`
	ResolvedAt       *time.Time `json:"resolvedAt,omitempty"`
}

type BrokerOperation struct {
	ID               string    `json:"id"`
	GameID           string    `json:"gameId"`
	RevisionID       string    `json:"revisionId"`
	TargetEndpointID string    `json:"targetEndpointId"`
	RelativePath     string    `json:"relativePath"`
	BlobHash         string    `json:"blobHash"`
	State            string    `json:"state"`
	Error            string    `json:"error,omitempty"`
	CreatedAt        time.Time `json:"createdAt"`
	UpdatedAt        time.Time `json:"updatedAt"`
}

type GameDetail struct {
	Game
	Bindings     []SaveBinding     `json:"bindings"`
	Revisions    []Revision        `json:"revisions"`
	Observations []Observation     `json:"observations"`
	Conflicts    []Conflict        `json:"conflicts"`
	Operations   []BrokerOperation `json:"operations"`
}

type Activity struct {
	ID         string    `json:"id"`
	GameID     string    `json:"gameId,omitempty"`
	GameTitle  string    `json:"gameTitle,omitempty"`
	Kind       string    `json:"kind"`
	Summary    string    `json:"summary"`
	EndpointID string    `json:"endpointId,omitempty"`
	CreatedAt  time.Time `json:"createdAt"`
}
