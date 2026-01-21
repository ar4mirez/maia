# RFD 0005: Multi-Region Replication

## Metadata

| Field | Value |
|-------|-------|
| RFD ID | 0005 |
| Title | Multi-Region Replication |
| Author | MAIA Team |
| Created | 2026-01-20 |
| Status | Draft |
| Decision | Pending |

## Summary

This RFD proposes a multi-region replication design for MAIA to enable:
- Geographic distribution of memory data for lower latency
- High availability across regions
- Disaster recovery capabilities
- Data locality compliance

## Background

As MAIA deployments grow, organizations need:
1. **Low latency access** - Users in different regions should access data with minimal latency
2. **High availability** - Continued operation even if a region fails
3. **Data compliance** - Some data must stay within specific geographic boundaries
4. **Disaster recovery** - Ability to recover from regional outages

## Design Options

### Option A: Leader-Follower Replication (Recommended)

**Architecture:**
```
                    ┌─────────────────┐
                    │   Global Load   │
                    │    Balancer     │
                    └────────┬────────┘
                             │
        ┌────────────────────┼────────────────────┐
        │                    │                    │
        ▼                    ▼                    ▼
┌───────────────┐   ┌───────────────┐   ┌───────────────┐
│   US-West     │   │   EU-Central  │   │   AP-Tokyo    │
│   (Leader)    │◄──│   (Follower)  │──►│   (Follower)  │
│               │   │               │   │               │
│ ┌───────────┐ │   │ ┌───────────┐ │   │ ┌───────────┐ │
│ │   MAIA    │ │   │ │   MAIA    │ │   │ │   MAIA    │ │
│ │ Instance  │ │   │ │ Instance  │ │   │ │ Instance  │ │
│ └───────────┘ │   │ └───────────┘ │   │ └───────────┘ │
│ ┌───────────┐ │   │ ┌───────────┐ │   │ ┌───────────┐ │
│ │  BadgerDB │ │   │ │  BadgerDB │ │   │ │  BadgerDB │ │
│ └───────────┘ │   │ └───────────┘ │   │ └───────────┘ │
└───────────────┘   └───────────────┘   └───────────────┘
        │                    ▲                    ▲
        │                    │                    │
        └──────────Async Replication──────────────┘
```

**Pros:**
- Simple to implement and reason about
- Strong consistency for writes (via leader)
- Eventually consistent reads (configurable)
- Clear failover semantics

**Cons:**
- Write latency for non-leader regions
- Leader is a single point of failure (mitigated by failover)
- Cross-region bandwidth costs

### Option B: Multi-Leader (Active-Active)

**Architecture:**
```
┌───────────────┐   ┌───────────────┐   ┌───────────────┐
│   US-West     │◄─►│   EU-Central  │◄─►│   AP-Tokyo    │
│   (Leader)    │   │   (Leader)    │   │   (Leader)    │
└───────────────┘   └───────────────┘   └───────────────┘
        │                    │                    │
        └────────────────────┴────────────────────┘
               Bidirectional Sync (CRDTs)
```

**Pros:**
- Low write latency in all regions
- No single point of failure
- Better for write-heavy workloads

**Cons:**
- Complex conflict resolution (requires CRDTs)
- Higher implementation complexity
- More expensive (more storage, more sync)

### Option C: Tenant-Based Sharding

**Architecture:**
```
Tenant A (US-only) ──► US-West Region
Tenant B (EU-only) ──► EU-Central Region
Tenant C (Global)  ──► US-West (Leader) + EU-Central + AP-Tokyo
```

**Pros:**
- Simple data locality compliance
- Reduced replication costs
- Natural isolation

**Cons:**
- Tenant migration is complex
- Global tenants still need replication

## Recommended Approach

**Option A (Leader-Follower) with Tenant-Based Sharding (Option C)**

This hybrid approach provides:
1. Single-region tenants get local storage only (compliance + cost savings)
2. Global tenants get leader-follower replication
3. Clear consistency model
4. Straightforward failover

## Detailed Design

### 1. Replication Protocol

```go
// ReplicationConfig defines replication settings
type ReplicationConfig struct {
    // Role of this instance
    Role ReplicationRole // leader, follower

    // Leader endpoint (for followers)
    LeaderEndpoint string

    // Follower endpoints (for leader)
    Followers []FollowerConfig

    // Replication settings
    SyncMode       SyncMode // async, sync, semi-sync
    ReplicationLag time.Duration // max acceptable lag for async

    // Conflict resolution
    ConflictResolution ConflictStrategy // last-write-wins, merge
}

type SyncMode string

const (
    SyncModeAsync    SyncMode = "async"     // Leader doesn't wait
    SyncModeSync     SyncMode = "sync"      // Wait for all followers
    SyncModeSemiSync SyncMode = "semi-sync" // Wait for N followers
)
```

### 2. Write-Ahead Log (WAL)

All writes are logged to enable replication:

```go
// WALEntry represents a single write operation
type WALEntry struct {
    ID          string    // Unique entry ID
    Timestamp   time.Time // When the write occurred
    TenantID    string    // Tenant this write belongs to
    Operation   Operation // CREATE, UPDATE, DELETE
    ResourceType string   // memory, namespace, etc.
    ResourceID  string    // ID of the resource
    Data        []byte    // Serialized resource data
    Checksum    uint32    // CRC32 for integrity
}

// WAL provides write-ahead logging
type WAL interface {
    // Append adds an entry to the log
    Append(ctx context.Context, entry *WALEntry) error

    // Read returns entries after the given position
    Read(ctx context.Context, afterID string, limit int) ([]*WALEntry, error)

    // Truncate removes entries before the given ID
    Truncate(ctx context.Context, beforeID string) error

    // Position returns the current WAL position
    Position(ctx context.Context) (string, error)
}
```

### 3. Replication Stream

```go
// ReplicationStream handles data sync between regions
type ReplicationStream struct {
    config   *ReplicationConfig
    wal      WAL
    store    storage.Store
    logger   *zap.Logger
}

// For leaders: push changes to followers
func (s *ReplicationStream) PushToFollowers(ctx context.Context) error {
    for _, follower := range s.config.Followers {
        go s.syncToFollower(ctx, follower)
    }
    return nil
}

// For followers: pull changes from leader
func (s *ReplicationStream) PullFromLeader(ctx context.Context) error {
    conn, err := s.connectToLeader()
    if err != nil {
        return err
    }
    defer conn.Close()

    for {
        entries, err := conn.FetchEntries(s.lastPosition)
        if err != nil {
            return err
        }

        for _, entry := range entries {
            if err := s.applyEntry(entry); err != nil {
                return err
            }
        }
    }
}
```

### 4. Conflict Resolution

For semi-sync or async modes, conflicts can occur:

```go
// ConflictResolver handles write conflicts
type ConflictResolver interface {
    // Resolve determines which version wins
    Resolve(local, remote *WALEntry) (*WALEntry, error)
}

// LastWriteWins uses timestamp to resolve conflicts
type LastWriteWins struct{}

func (l *LastWriteWins) Resolve(local, remote *WALEntry) (*WALEntry, error) {
    if remote.Timestamp.After(local.Timestamp) {
        return remote, nil
    }
    return local, nil
}

// MergeResolver attempts to merge non-conflicting changes
type MergeResolver struct {
    // For memories: merge metadata, keep content from latest
}
```

### 5. Tenant Placement

```go
// TenantPlacement defines where tenant data lives
type TenantPlacement struct {
    TenantID     string
    PrimaryRegion string          // Where writes go
    Replicas     []string         // Read replicas
    Mode         PlacementMode    // single, replicated, global
}

type PlacementMode string

const (
    PlacementSingle     PlacementMode = "single"     // One region only
    PlacementReplicated PlacementMode = "replicated" // Leader + followers
    PlacementGlobal     PlacementMode = "global"     // All regions
)
```

### 6. API Changes

```yaml
# New configuration options
replication:
  enabled: true
  role: leader  # or follower
  region: us-west-1

  # Leader config
  followers:
    - endpoint: https://eu-maia.example.com
      region: eu-central-1
    - endpoint: https://ap-maia.example.com
      region: ap-northeast-1

  # Follower config
  leader:
    endpoint: https://us-maia.example.com
    region: us-west-1

  # Sync settings
  sync_mode: semi-sync
  min_sync_replicas: 1
  max_replication_lag: 30s

# Tenant placement API
POST /admin/tenants/{tenant_id}/placement
{
  "primary_region": "eu-central-1",
  "replicas": ["us-west-1"],
  "mode": "replicated"
}
```

### 7. Monitoring & Observability

```go
// Metrics for replication monitoring
const (
    MetricReplicationLag      = "maia_replication_lag_seconds"
    MetricReplicationPosition = "maia_replication_position"
    MetricReplicationErrors   = "maia_replication_errors_total"
    MetricConflictsResolved   = "maia_conflicts_resolved_total"
)
```

## Implementation Phases

### Phase 1: Foundation (4-6 weeks)
- [ ] Implement WAL for all write operations
- [ ] Add WAL compaction and cleanup
- [ ] Create replication position tracking
- [ ] Add WAL integrity checks

### Phase 2: Leader-Follower (4-6 weeks)
- [ ] Implement push replication for leaders
- [ ] Implement pull replication for followers
- [ ] Add sync mode configuration
- [ ] Implement basic conflict detection

### Phase 3: Tenant Placement (2-3 weeks)
- [ ] Add tenant placement API
- [ ] Implement request routing based on placement
- [ ] Add placement migration tools

### Phase 4: Observability (2-3 weeks)
- [ ] Add replication metrics
- [ ] Create Grafana dashboard
- [ ] Add alerting rules for lag/errors

### Phase 5: Production Hardening (3-4 weeks)
- [ ] Implement automatic failover
- [ ] Add leader election
- [ ] Create disaster recovery runbooks
- [ ] Load testing across regions

## Security Considerations

1. **Encryption in Transit**: All replication traffic uses mTLS
2. **Authentication**: Followers authenticate with leader using certificates
3. **Authorization**: Only authorized followers can receive replication stream
4. **Data Encryption**: WAL entries are encrypted at rest

## Cost Analysis

| Scenario | Monthly Cost Estimate |
|----------|----------------------|
| Single Region | $X (baseline) |
| 2 Regions (leader-follower) | ~2.5x (storage + transfer) |
| 3 Regions | ~4x |
| Global (all regions) | ~6x |

Cross-region data transfer is the primary cost driver.

## Alternatives Considered

1. **External Replication (e.g., Litestream)**: Simpler but less control
2. **Database-level Replication**: Would require switching from BadgerDB
3. **Object Storage Sync (S3)**: High latency, eventual consistency only

## Open Questions

1. Should we support read-your-writes consistency for async mode?
2. How do we handle long network partitions between regions?
3. Should tenant placement be automatic based on access patterns?

## References

- [Designing Data-Intensive Applications - Chapter 5](https://dataintensive.net/)
- [AWS Multi-Region Architectures](https://aws.amazon.com/solutions/implementations/multi-region/)
- [CockroachDB Replication](https://www.cockroachlabs.com/docs/stable/architecture/replication-layer.html)
