# PRD 0007: Replication Phase 3-5 Implementation

## Metadata

| Field | Value |
|-------|-------|
| PRD ID | 0007 |
| Title | Replication Phase 3-5: Tenant Placement, Observability, and Production Hardening |
| Author | MAIA Team |
| Created | 2026-01-21 |
| Status | Draft |
| Related RFD | 0005 (Multi-Region Replication) |

## Summary

This PRD defines the implementation of Phases 3-5 from RFD 0005, completing the multi-region replication feature set with:
- **Phase 3**: Tenant placement routing and migration tools
- **Phase 4**: Observability enhancements (dashboards, alerts)
- **Phase 5**: Automatic failover and leader election

## Background

Phase 1-2 of multi-region replication is complete, providing:
- Write-Ahead Log (WAL) with BadgerDB storage
- Leader-follower replication manager
- Conflict resolution (last-write-wins, merge, reject)
- Tenant placement API
- HTTP handlers for replication operations

What's missing:
- Request routing based on tenant placement (writes to primary, reads to replicas)
- Tenant migration between regions
- Replication-specific Grafana dashboards
- Alerting rules for replication health
- Automatic failover when leader becomes unhealthy
- Leader election for HA

## Requirements

### Phase 3: Tenant Placement Routing & Migration

#### P3.1 Request Routing Middleware
- **Must**: Route write requests to primary region for tenant
- **Must**: Allow read requests from any replica
- **Must**: Return HTTP 307 redirect to correct region for misrouted writes
- **Must**: Support `X-MAIA-Preferred-Region` header for read affinity
- **Should**: Cache placement lookups for performance
- **Should**: Handle placement cache invalidation on updates

#### P3.2 Tenant Migration API
- **Must**: Provide `POST /admin/tenants/:id/migrate` endpoint
- **Must**: Support dry-run mode to preview migration plan
- **Must**: Validate target region is available and healthy
- **Must**: Track migration status (pending, in_progress, completed, failed)
- **Should**: Support cancellation of in-progress migrations
- **Should**: Emit events for migration lifecycle

#### P3.3 Migration Execution
- **Must**: Pause writes during migration window
- **Must**: Ensure all WAL entries replicated before switchover
- **Must**: Update placement atomically
- **Must**: Resume writes to new primary
- **Should**: Minimize downtime (target: <30 seconds)

### Phase 4: Observability

#### P4.1 Grafana Dashboard
- **Must**: Show replication lag per follower
- **Must**: Show WAL throughput (entries/sec, bytes/sec)
- **Must**: Show conflict resolution rate by strategy
- **Must**: Show follower health status
- **Should**: Show tenant placement distribution map
- **Should**: Show cross-region latency

#### P4.2 Prometheus Alerts
- **Must**: Alert on replication lag > threshold (warning: 30s, critical: 60s)
- **Must**: Alert on follower disconnect > duration
- **Must**: Alert on high conflict rate
- **Must**: Alert on WAL growth exceeding retention
- **Should**: Alert on cross-region latency spike

#### P4.3 Replication Health Endpoint
- **Must**: Provide `GET /v1/replication/health` endpoint
- **Must**: Return overall health status (healthy, degraded, unhealthy)
- **Must**: Include per-follower health details
- **Must**: Include WAL statistics

### Phase 5: Production Hardening

#### P5.1 Leader Health Monitoring
- **Must**: Track leader health from follower perspective
- **Must**: Detect leader unavailability within configurable timeout
- **Must**: Support configurable health check interval
- **Should**: Use exponential backoff for retries

#### P5.2 Automatic Failover
- **Must**: Promote highest-priority healthy follower on leader failure
- **Must**: Require configurable quorum for failover decision
- **Must**: Update all followers with new leader
- **Must**: Log failover events with full context
- **Should**: Support manual failover trigger via API
- **Should**: Support failover inhibit window (prevent flapping)

#### P5.3 Leader Election
- **Must**: Implement distributed leader election
- **Must**: Support multiple election backends:
  - Embedded (using WAL position + priority)
  - External (etcd, Consul) - optional
- **Must**: Handle split-brain scenarios
- **Must**: Ensure exactly-one-leader guarantee
- **Should**: Support graceful leadership transfer

#### P5.4 Disaster Recovery
- **Must**: Document manual failover procedure
- **Must**: Document recovery from split-brain
- **Must**: Document data reconciliation after network partition
- **Should**: Provide CLI commands for DR operations

## Technical Design

### Phase 3: Request Routing

```go
// internal/replication/routing.go

// RoutingMiddleware routes requests based on tenant placement
type RoutingMiddleware struct {
    manager   ReplicationManager
    cache     *PlacementCache
    logger    *zap.Logger
}

// RouteRequest checks if request should be handled locally
func (r *RoutingMiddleware) RouteRequest(c *gin.Context) {
    tenantID := extractTenantID(c)
    placement, _ := r.cache.Get(tenantID)

    if isWriteRequest(c) && placement.PrimaryRegion != r.manager.Region() {
        // Redirect to primary
        c.Redirect(http.StatusTemporaryRedirect, buildRedirectURL(placement.PrimaryRegion, c.Request))
        c.Abort()
        return
    }

    c.Next()
}
```

### Phase 3: Tenant Migration

```go
// internal/replication/migration.go

type MigrationState string
const (
    MigrationStatePending    MigrationState = "pending"
    MigrationStateInProgress MigrationState = "in_progress"
    MigrationStateCompleted  MigrationState = "completed"
    MigrationStateFailed     MigrationState = "failed"
    MigrationStateCancelled  MigrationState = "cancelled"
)

type Migration struct {
    ID            string
    TenantID      string
    FromRegion    string
    ToRegion      string
    State         MigrationState
    StartedAt     time.Time
    CompletedAt   *time.Time
    Error         string
    WALPosition   *WALPosition
}

type MigrationManager interface {
    // Start initiates a tenant migration
    StartMigration(ctx context.Context, tenantID, toRegion string) (*Migration, error)

    // GetMigration returns migration status
    GetMigration(ctx context.Context, migrationID string) (*Migration, error)

    // CancelMigration cancels an in-progress migration
    CancelMigration(ctx context.Context, migrationID string) error

    // ListMigrations returns migrations for a tenant
    ListMigrations(ctx context.Context, tenantID string) ([]*Migration, error)
}
```

### Phase 5: Leader Election

```go
// internal/replication/election.go

type ElectionState string
const (
    ElectionStateFollower  ElectionState = "follower"
    ElectionStateCandidate ElectionState = "candidate"
    ElectionStateLeader    ElectionState = "leader"
)

type Election struct {
    config    *ElectionConfig
    state     ElectionState
    term      uint64
    votedFor  string
    leader    string
    votes     map[string]bool
    mu        sync.RWMutex
}

type ElectionConfig struct {
    // Election timeout range (randomized)
    MinElectionTimeout time.Duration // Default: 150ms
    MaxElectionTimeout time.Duration // Default: 300ms

    // Heartbeat interval for leader
    HeartbeatInterval time.Duration // Default: 50ms

    // Quorum size (majority of nodes)
    QuorumSize int

    // Priority for election (higher = more likely to be elected)
    Priority int
}

// StartElection begins leader election process
func (e *Election) StartElection(ctx context.Context) error

// RequestVote handles vote request from candidate
func (e *Election) RequestVote(ctx context.Context, req *VoteRequest) (*VoteResponse, error)

// AppendEntries handles heartbeat/replication from leader
func (e *Election) AppendEntries(ctx context.Context, req *AppendEntriesRequest) (*AppendEntriesResponse, error)
```

### Phase 5: Automatic Failover

```go
// internal/replication/failover.go

type FailoverManager struct {
    manager   ReplicationManager
    election  *Election
    config    *FailoverConfig
    logger    *zap.Logger
}

type FailoverConfig struct {
    // How long leader can be unhealthy before failover
    LeaderTimeout time.Duration // Default: 30s

    // Minimum time between failovers (prevent flapping)
    InhibitWindow time.Duration // Default: 60s

    // Whether to auto-failover or require manual trigger
    AutoFailover bool // Default: true
}

// TriggerFailover manually initiates failover
func (f *FailoverManager) TriggerFailover(ctx context.Context) error

// IsFailoverInhibited checks if failover is currently inhibited
func (f *FailoverManager) IsFailoverInhibited() bool
```

## API Changes

### New Endpoints

```yaml
# Phase 3: Migration
POST   /admin/tenants/:id/migrate
  body: { "to_region": "eu-central-1", "dry_run": false }
  response: { "migration_id": "...", "state": "pending", ... }

GET    /admin/migrations/:id
  response: { "migration": { ... } }

POST   /admin/migrations/:id/cancel
  response: { "migration": { "state": "cancelled" } }

GET    /admin/tenants/:id/migrations
  response: { "migrations": [...] }

# Phase 4: Health
GET    /v1/replication/health
  response: {
    "status": "healthy|degraded|unhealthy",
    "role": "leader",
    "region": "us-west-1",
    "wal": { "position": 12345, "size": "1.2GB" },
    "followers": [
      { "id": "...", "region": "...", "lag": "2s", "healthy": true }
    ]
  }

# Phase 5: Failover
POST   /admin/replication/failover
  body: { "target_follower": "follower-1" }  # optional
  response: { "new_leader": "follower-1", "old_leader": "leader-1" }

GET    /admin/replication/election
  response: {
    "state": "leader|follower|candidate",
    "term": 5,
    "leader": "node-1",
    "voted_for": "node-1"
  }
```

## Testing Strategy

### Unit Tests
- Routing middleware with mock placement cache
- Migration state machine transitions
- Election logic (state transitions, voting)
- Failover trigger conditions

### Integration Tests
- End-to-end tenant migration
- Automatic failover on leader shutdown
- Leader election with multiple candidates
- Split-brain recovery

### Load Tests
- Request routing under load
- Migration during high write throughput
- Failover time measurement

## Acceptance Criteria

### Phase 3
- [ ] Write requests redirected to primary region
- [ ] Read requests served from any replica
- [ ] Tenant migration completes in <30 seconds
- [ ] Migration can be cancelled mid-flight
- [ ] Placement cache invalidation works correctly

### Phase 4
- [ ] Grafana dashboard shows all required metrics
- [ ] Alerts fire within specified thresholds
- [ ] Health endpoint returns accurate status

### Phase 5
- [ ] Failover triggers within 30 seconds of leader failure
- [ ] New leader elected deterministically
- [ ] All followers recognize new leader
- [ ] No data loss during failover
- [ ] Manual failover works via API

## Rollout Plan

1. **Phase 3** (Week 1-2)
   - Implement routing middleware
   - Implement migration API
   - Test with synthetic traffic

2. **Phase 4** (Week 3)
   - Create Grafana dashboards
   - Configure alerting rules
   - Deploy to staging

3. **Phase 5** (Week 4-5)
   - Implement leader election
   - Implement automatic failover
   - Chaos testing

4. **Documentation** (Week 6)
   - Update operator docs
   - Create runbooks
   - Training materials

## Risks & Mitigations

| Risk | Impact | Mitigation |
|------|--------|------------|
| Split-brain during failover | High | Quorum-based election, fencing |
| Data loss during migration | High | Wait for full WAL sync before switchover |
| Failover flapping | Medium | Inhibit window, manual override |
| Network partition handling | Medium | Clear partition detection, manual reconciliation |

## Dependencies

- Phase 1-2 replication (complete)
- Tenant management (complete)
- Prometheus metrics (complete)
- Grafana deployment (complete)
