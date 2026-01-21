// Package replication provides multi-region replication support for MAIA.
// It implements a leader-follower replication model with Write-Ahead Logging (WAL).
package replication

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"hash/crc32"
	"time"
)

// Common errors for replication operations.
var (
	ErrNotLeader           = errors.New("operation requires leader role")
	ErrNotFollower         = errors.New("operation requires follower role")
	ErrLeaderUnavailable   = errors.New("leader is unavailable")
	ErrReplicationLag      = errors.New("replication lag exceeds threshold")
	ErrConflict            = errors.New("write conflict detected")
	ErrInvalidWALEntry     = errors.New("invalid WAL entry")
	ErrChecksumMismatch    = errors.New("WAL entry checksum mismatch")
	ErrWALClosed           = errors.New("WAL is closed")
	ErrFollowerNotFound    = errors.New("follower not found")
	ErrInvalidPlacement    = errors.New("invalid tenant placement")
	ErrRegionNotAvailable  = errors.New("region not available")
	ErrTenantNotReplicated = errors.New("tenant is not configured for replication")
)

// Role defines the replication role of a MAIA instance.
type Role string

const (
	// RoleLeader indicates this instance is the primary write target.
	RoleLeader Role = "leader"

	// RoleFollower indicates this instance replicates from a leader.
	RoleFollower Role = "follower"

	// RoleStandalone indicates no replication is configured.
	RoleStandalone Role = "standalone"
)

// SyncMode defines how writes are synchronized to followers.
type SyncMode string

const (
	// SyncModeAsync means leader doesn't wait for followers.
	// Provides lowest latency but may lose recent writes on leader failure.
	SyncModeAsync SyncMode = "async"

	// SyncModeSync means leader waits for all followers to acknowledge.
	// Provides strongest durability but highest latency.
	SyncModeSync SyncMode = "sync"

	// SyncModeSemiSync means leader waits for N followers to acknowledge.
	// Balances durability and latency.
	SyncModeSemiSync SyncMode = "semi-sync"
)

// Operation represents a type of write operation in the WAL.
type Operation string

const (
	OperationCreate Operation = "create"
	OperationUpdate Operation = "update"
	OperationDelete Operation = "delete"
)

// ResourceType represents the type of resource being modified.
type ResourceType string

const (
	ResourceTypeMemory    ResourceType = "memory"
	ResourceTypeNamespace ResourceType = "namespace"
	ResourceTypeTenant    ResourceType = "tenant"
	ResourceTypeAPIKey    ResourceType = "apikey"
)

// PlacementMode defines how a tenant's data is distributed.
type PlacementMode string

const (
	// PlacementSingle means data lives in one region only.
	PlacementSingle PlacementMode = "single"

	// PlacementReplicated means leader-follower replication is active.
	PlacementReplicated PlacementMode = "replicated"

	// PlacementGlobal means data is replicated to all regions.
	PlacementGlobal PlacementMode = "global"
)

// ConflictStrategy defines how write conflicts are resolved.
type ConflictStrategy string

const (
	// ConflictLastWriteWins uses timestamp to resolve conflicts.
	ConflictLastWriteWins ConflictStrategy = "last-write-wins"

	// ConflictMerge attempts to merge non-conflicting changes.
	ConflictMerge ConflictStrategy = "merge"

	// ConflictReject rejects conflicting writes.
	ConflictReject ConflictStrategy = "reject"
)

// WALEntry represents a single write operation in the Write-Ahead Log.
type WALEntry struct {
	// ID is a unique identifier for this entry (ULID for ordering).
	ID string `json:"id"`

	// Sequence is a monotonically increasing sequence number.
	Sequence uint64 `json:"sequence"`

	// Timestamp is when the write occurred.
	Timestamp time.Time `json:"timestamp"`

	// TenantID is the tenant this write belongs to.
	TenantID string `json:"tenant_id"`

	// Operation is the type of write (create, update, delete).
	Operation Operation `json:"operation"`

	// ResourceType is the type of resource (memory, namespace, etc.).
	ResourceType ResourceType `json:"resource_type"`

	// ResourceID is the unique ID of the resource.
	ResourceID string `json:"resource_id"`

	// Namespace is the namespace context (for memories).
	Namespace string `json:"namespace,omitempty"`

	// Data is the serialized resource data.
	Data []byte `json:"data,omitempty"`

	// PreviousData is the previous state (for updates/deletes).
	PreviousData []byte `json:"previous_data,omitempty"`

	// Checksum is CRC32 for integrity verification.
	Checksum uint32 `json:"checksum"`

	// Region is the region where this write originated.
	Region string `json:"region"`

	// Replicated indicates if this entry was received via replication.
	Replicated bool `json:"replicated"`
}

// Validate checks the WAL entry for integrity.
func (e *WALEntry) Validate() error {
	if e.ID == "" {
		return errors.New("WAL entry ID is required")
	}
	if e.Timestamp.IsZero() {
		return errors.New("WAL entry timestamp is required")
	}
	if e.Operation == "" {
		return errors.New("WAL entry operation is required")
	}
	if e.ResourceType == "" {
		return errors.New("WAL entry resource type is required")
	}
	if e.ResourceID == "" {
		return errors.New("WAL entry resource ID is required")
	}
	return nil
}

// ComputeChecksum calculates the CRC32 checksum for the entry.
func (e *WALEntry) ComputeChecksum() uint32 {
	data, _ := json.Marshal(struct {
		ID           string       `json:"id"`
		Sequence     uint64       `json:"sequence"`
		Timestamp    time.Time    `json:"timestamp"`
		TenantID     string       `json:"tenant_id"`
		Operation    Operation    `json:"operation"`
		ResourceType ResourceType `json:"resource_type"`
		ResourceID   string       `json:"resource_id"`
		Namespace    string       `json:"namespace,omitempty"`
		Data         []byte       `json:"data,omitempty"`
	}{
		ID:           e.ID,
		Sequence:     e.Sequence,
		Timestamp:    e.Timestamp,
		TenantID:     e.TenantID,
		Operation:    e.Operation,
		ResourceType: e.ResourceType,
		ResourceID:   e.ResourceID,
		Namespace:    e.Namespace,
		Data:         e.Data,
	})
	return crc32.ChecksumIEEE(data)
}

// VerifyChecksum validates the entry checksum.
func (e *WALEntry) VerifyChecksum() bool {
	return e.Checksum == e.ComputeChecksum()
}

// WALPosition represents a position in the WAL.
type WALPosition struct {
	// Sequence is the sequence number.
	Sequence uint64 `json:"sequence"`

	// EntryID is the WAL entry ID at this position.
	EntryID string `json:"entry_id"`

	// Timestamp is when this position was reached.
	Timestamp time.Time `json:"timestamp"`
}

// WAL provides write-ahead logging for replication.
type WAL interface {
	// Append adds an entry to the log.
	Append(ctx context.Context, entry *WALEntry) error

	// Read returns entries after the given sequence number.
	Read(ctx context.Context, afterSequence uint64, limit int) ([]*WALEntry, error)

	// ReadByID returns entries after the given entry ID.
	ReadByID(ctx context.Context, afterID string, limit int) ([]*WALEntry, error)

	// GetEntry returns a specific entry by ID.
	GetEntry(ctx context.Context, id string) (*WALEntry, error)

	// Position returns the current WAL position.
	Position(ctx context.Context) (*WALPosition, error)

	// Truncate removes entries before the given sequence number.
	Truncate(ctx context.Context, beforeSequence uint64) error

	// Sync ensures all entries are persisted to disk.
	Sync(ctx context.Context) error

	// Close closes the WAL.
	Close() error
}

// FollowerConfig configures a follower in the replication cluster.
type FollowerConfig struct {
	// ID is a unique identifier for the follower.
	ID string `json:"id"`

	// Endpoint is the replication endpoint URL.
	Endpoint string `json:"endpoint"`

	// Region is the geographic region of the follower.
	Region string `json:"region"`

	// TLS configures TLS for the connection.
	TLS *tls.Config `json:"-"`

	// Priority is used for leader election (higher = more preferred).
	Priority int `json:"priority"`

	// MaxLag is the maximum acceptable replication lag.
	MaxLag time.Duration `json:"max_lag"`
}

// FollowerStatus represents the current state of a follower.
type FollowerStatus struct {
	// ID is the follower identifier.
	ID string `json:"id"`

	// Region is the follower's region.
	Region string `json:"region"`

	// Connected indicates if the follower is connected.
	Connected bool `json:"connected"`

	// Position is the follower's current WAL position.
	Position *WALPosition `json:"position,omitempty"`

	// Lag is the replication lag.
	Lag time.Duration `json:"lag"`

	// LastSeen is when we last heard from this follower.
	LastSeen time.Time `json:"last_seen"`

	// LastError is the most recent error.
	LastError string `json:"last_error,omitempty"`

	// BytesSent is total bytes sent to this follower.
	BytesSent uint64 `json:"bytes_sent"`

	// EntriesSent is total entries sent to this follower.
	EntriesSent uint64 `json:"entries_sent"`
}

// LeaderInfo provides information about the current leader.
type LeaderInfo struct {
	// ID is the leader's instance ID.
	ID string `json:"id"`

	// Endpoint is the leader's replication endpoint.
	Endpoint string `json:"endpoint"`

	// Region is the leader's region.
	Region string `json:"region"`

	// Position is the leader's current WAL position.
	Position *WALPosition `json:"position"`

	// Since is when this instance became leader.
	Since time.Time `json:"since"`
}

// TenantPlacement defines where a tenant's data lives.
type TenantPlacement struct {
	// TenantID is the tenant identifier.
	TenantID string `json:"tenant_id"`

	// PrimaryRegion is where writes go.
	PrimaryRegion string `json:"primary_region"`

	// Replicas are the read replica regions.
	Replicas []string `json:"replicas,omitempty"`

	// Mode is the placement mode.
	Mode PlacementMode `json:"mode"`

	// CreatedAt is when this placement was created.
	CreatedAt time.Time `json:"created_at"`

	// UpdatedAt is when this placement was last updated.
	UpdatedAt time.Time `json:"updated_at"`
}

// ReplicationStats provides statistics about replication.
type ReplicationStats struct {
	// Role is the current replication role.
	Role Role `json:"role"`

	// Region is the current region.
	Region string `json:"region"`

	// Position is the current WAL position.
	Position *WALPosition `json:"position"`

	// Leader is information about the leader (for followers).
	Leader *LeaderInfo `json:"leader,omitempty"`

	// Followers is the status of each follower (for leader).
	Followers []FollowerStatus `json:"followers,omitempty"`

	// WALSize is the size of the WAL in bytes.
	WALSize int64 `json:"wal_size"`

	// WALEntries is the number of entries in the WAL.
	WALEntries int64 `json:"wal_entries"`

	// ConflictsResolved is the total conflicts resolved.
	ConflictsResolved int64 `json:"conflicts_resolved"`

	// LastReplicationTime is when replication last occurred.
	LastReplicationTime time.Time `json:"last_replication_time,omitempty"`
}

// ConflictResolver handles write conflicts between regions.
type ConflictResolver interface {
	// Resolve determines which version wins in a conflict.
	Resolve(ctx context.Context, local, remote *WALEntry) (*WALEntry, error)
}

// ReplicationManager manages replication for a MAIA instance.
type ReplicationManager interface {
	// Start begins replication operations.
	Start(ctx context.Context) error

	// Stop gracefully stops replication.
	Stop(ctx context.Context) error

	// Role returns the current replication role.
	Role() Role

	// Region returns the current region.
	Region() string

	// Position returns the current WAL position.
	Position(ctx context.Context) (*WALPosition, error)

	// Stats returns replication statistics.
	Stats(ctx context.Context) (*ReplicationStats, error)

	// For leaders:

	// AddFollower registers a new follower.
	AddFollower(ctx context.Context, cfg *FollowerConfig) error

	// RemoveFollower removes a follower.
	RemoveFollower(ctx context.Context, id string) error

	// GetFollowerStatus returns the status of a specific follower.
	GetFollowerStatus(ctx context.Context, id string) (*FollowerStatus, error)

	// ListFollowers returns all registered followers.
	ListFollowers(ctx context.Context) ([]FollowerStatus, error)

	// For followers:

	// GetLeaderInfo returns information about the current leader.
	GetLeaderInfo(ctx context.Context) (*LeaderInfo, error)

	// SetLeader configures the leader to replicate from.
	SetLeader(ctx context.Context, endpoint string) error

	// Tenant placement:

	// GetTenantPlacement returns the placement for a tenant.
	GetTenantPlacement(ctx context.Context, tenantID string) (*TenantPlacement, error)

	// SetTenantPlacement configures placement for a tenant.
	SetTenantPlacement(ctx context.Context, placement *TenantPlacement) error

	// IsLocalTenant checks if a tenant's data should be stored locally.
	IsLocalTenant(ctx context.Context, tenantID string) (bool, error)
}

// StreamHandler handles a replication stream connection.
type StreamHandler interface {
	// HandleStream processes incoming replication stream from leader.
	HandleStream(ctx context.Context, stream ReplicationStream) error
}

// ReplicationStream represents a bidirectional replication stream.
type ReplicationStream interface {
	// Send sends a WAL entry to the remote.
	Send(entry *WALEntry) error

	// Recv receives a WAL entry from the remote.
	Recv() (*WALEntry, error)

	// SendAck sends an acknowledgment for a sequence number.
	SendAck(sequence uint64) error

	// RecvAck receives an acknowledgment.
	RecvAck() (uint64, error)

	// Close closes the stream.
	Close() error
}
