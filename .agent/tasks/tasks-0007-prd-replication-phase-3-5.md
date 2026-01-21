# Tasks: Replication Phase 3-5

**PRD Reference**: [0007-prd-replication-phase-3-5.md](0007-prd-replication-phase-3-5.md)
**Created**: 2026-01-21

---

## Phase 3: Tenant Placement Routing & Migration

### Task 3.1: Placement Cache
**File**: `internal/replication/cache.go`
**Estimated**: 2 hours

- [ ] Create `PlacementCache` struct with TTL-based caching
- [ ] Implement `Get(tenantID)` with cache-aside pattern
- [ ] Implement `Invalidate(tenantID)` for cache invalidation
- [ ] Implement `InvalidateAll()` for full cache clear
- [ ] Add cache hit/miss metrics
- [ ] Write comprehensive tests

**Test Coverage Target**: 90%

---

### Task 3.2: Request Routing Middleware
**File**: `internal/replication/routing.go`
**Estimated**: 3 hours

- [ ] Create `RoutingMiddleware` struct
- [ ] Implement `RouteRequest()` gin middleware
- [ ] Handle write request routing (redirect to primary)
- [ ] Handle read request routing (local or preferred region)
- [ ] Parse `X-MAIA-Preferred-Region` header
- [ ] Generate redirect URLs with original path/query
- [ ] Skip routing for admin/health endpoints
- [ ] Write comprehensive tests

**Test Coverage Target**: 85%

---

### Task 3.3: Migration Types
**File**: `internal/replication/migration.go` (types section)
**Estimated**: 1 hour

- [ ] Define `MigrationState` enum
- [ ] Define `Migration` struct
- [ ] Define `MigrationManager` interface
- [ ] Define `MigrationEvent` for lifecycle events
- [ ] Add validation methods

---

### Task 3.4: Migration Storage
**File**: `internal/replication/migration.go` (storage section)
**Estimated**: 2 hours

- [ ] Store migrations in BadgerDB with `migration:{id}` key
- [ ] Index by tenant: `migration_tenant:{tenant_id}:{id}`
- [ ] Implement `CreateMigration()`
- [ ] Implement `UpdateMigration()`
- [ ] Implement `GetMigration()`
- [ ] Implement `ListMigrations()`
- [ ] Write tests for storage operations

---

### Task 3.5: Migration Executor
**File**: `internal/replication/migration.go` (executor section)
**Estimated**: 4 hours

- [ ] Implement `MigrationExecutor` struct
- [ ] Implement pre-migration validation (target region health)
- [ ] Implement write pause mechanism for tenant
- [ ] Implement WAL drain wait (all entries replicated)
- [ ] Implement placement update
- [ ] Implement write resume
- [ ] Handle migration timeout
- [ ] Handle migration cancellation
- [ ] Emit migration events
- [ ] Write comprehensive tests

**Test Coverage Target**: 85%

---

### Task 3.6: Migration HTTP Handlers
**File**: `internal/replication/handlers.go` (add to existing)
**Estimated**: 2 hours

- [ ] Add `POST /admin/tenants/:id/migrate` handler
- [ ] Add `GET /admin/migrations/:id` handler
- [ ] Add `POST /admin/migrations/:id/cancel` handler
- [ ] Add `GET /admin/tenants/:id/migrations` handler
- [ ] Add dry-run support
- [ ] Write handler tests

---

### Task 3.7: Server Integration for Phase 3
**File**: `internal/server/server.go`
**Estimated**: 1 hour

- [ ] Add routing middleware to server
- [ ] Add migration handlers to admin routes
- [ ] Add placement cache initialization
- [ ] Configure cache TTL from config

---

## Phase 4: Observability

### Task 4.1: Replication Health Endpoint
**File**: `internal/replication/handlers.go` (add to existing)
**Estimated**: 2 hours

- [ ] Add `GET /v1/replication/health` handler
- [ ] Calculate overall health status (healthy/degraded/unhealthy)
- [ ] Include WAL statistics
- [ ] Include per-follower health
- [ ] Include replication lag
- [ ] Write handler tests

---

### Task 4.2: Additional Replication Metrics
**File**: `internal/metrics/metrics.go` (add to existing)
**Estimated**: 1 hour

- [ ] Add `maia_replication_lag_seconds` histogram
- [ ] Add `maia_replication_entries_total` counter
- [ ] Add `maia_replication_bytes_total` counter
- [ ] Add `maia_replication_conflicts_total` counter (by strategy)
- [ ] Add `maia_replication_follower_health` gauge
- [ ] Add `maia_migration_duration_seconds` histogram
- [ ] Add `maia_migration_total` counter (by state)

---

### Task 4.3: Grafana Replication Dashboard
**File**: `deployments/grafana/dashboards/maia-replication.json`
**Estimated**: 3 hours

- [ ] Create dashboard JSON
- [ ] Add replication lag panel (per follower)
- [ ] Add WAL throughput panel (entries/sec)
- [ ] Add WAL size panel
- [ ] Add conflict resolution rate panel
- [ ] Add follower health status panel
- [ ] Add migration status panel
- [ ] Add cross-region latency panel
- [ ] Test dashboard in Grafana

---

### Task 4.4: Prometheus Alerting Rules
**File**: `deployments/prometheus/alerts-replication.yaml`
**Estimated**: 2 hours

- [ ] Add `MAIAReplicationLagWarning` (lag > 30s)
- [ ] Add `MAIAReplicationLagCritical` (lag > 60s)
- [ ] Add `MAIAFollowerDisconnected` (disconnected > 60s)
- [ ] Add `MAIAHighConflictRate` (conflicts > 10/min)
- [ ] Add `MAIAWALGrowthHigh` (WAL > retention threshold)
- [ ] Add `MAIAMigrationStuck` (migration in_progress > 5min)
- [ ] Update `deployments/prometheus.yaml` to include new rules

---

### Task 4.5: Update docker-compose for Replication Observability
**File**: `docker-compose.yaml`
**Estimated**: 30 minutes

- [ ] Mount replication dashboard
- [ ] Mount replication alert rules
- [ ] Add multi-instance example configuration

---

## Phase 5: Production Hardening

### Task 5.1: Leader Health Monitoring
**File**: `internal/replication/health.go`
**Estimated**: 2 hours

- [ ] Create `LeaderHealthMonitor` struct
- [ ] Implement periodic health check to leader
- [ ] Track consecutive failures/successes
- [ ] Calculate health status (healthy/suspect/unhealthy)
- [ ] Support configurable check interval
- [ ] Support exponential backoff on failures
- [ ] Emit health change events
- [ ] Write comprehensive tests

**Test Coverage Target**: 90%

---

### Task 5.2: Election Types
**File**: `internal/replication/election.go` (types section)
**Estimated**: 1 hour

- [ ] Define `ElectionState` enum (follower, candidate, leader)
- [ ] Define `ElectionConfig` struct
- [ ] Define `VoteRequest` and `VoteResponse` structs
- [ ] Define `AppendEntriesRequest` and `AppendEntriesResponse` structs
- [ ] Define election-related errors

---

### Task 5.3: Election State Machine
**File**: `internal/replication/election.go` (state machine)
**Estimated**: 4 hours

- [ ] Implement `Election` struct with mutex protection
- [ ] Implement state transitions (follower → candidate → leader)
- [ ] Implement `StartElection()` - begin candidate state
- [ ] Implement `RequestVote()` - handle vote requests
- [ ] Implement `GrantVote()` - vote for candidate
- [ ] Implement `BecomeLeader()` - transition to leader
- [ ] Implement `StepDown()` - transition to follower
- [ ] Implement randomized election timeout
- [ ] Handle term number correctly
- [ ] Write comprehensive state machine tests

**Test Coverage Target**: 90%

---

### Task 5.4: Election HTTP Handlers
**File**: `internal/replication/handlers.go` (add to existing)
**Estimated**: 2 hours

- [ ] Add `POST /replication/vote` handler
- [ ] Add `POST /replication/heartbeat` handler
- [ ] Add `GET /admin/replication/election` handler
- [ ] Write handler tests

---

### Task 5.5: Failover Types
**File**: `internal/replication/failover.go` (types section)
**Estimated**: 1 hour

- [ ] Define `FailoverConfig` struct
- [ ] Define `FailoverEvent` struct
- [ ] Define `FailoverManager` interface
- [ ] Define failover-related errors

---

### Task 5.6: Automatic Failover
**File**: `internal/replication/failover.go` (implementation)
**Estimated**: 4 hours

- [ ] Implement `FailoverManager` struct
- [ ] Implement leader failure detection
- [ ] Implement failover trigger logic
- [ ] Implement inhibit window (prevent flapping)
- [ ] Implement `TriggerFailover()` for manual trigger
- [ ] Implement `IsFailoverInhibited()` check
- [ ] Notify all followers of new leader
- [ ] Log failover events with context
- [ ] Write comprehensive tests

**Test Coverage Target**: 85%

---

### Task 5.7: Failover HTTP Handler
**File**: `internal/replication/handlers.go` (add to existing)
**Estimated**: 1 hour

- [ ] Add `POST /admin/replication/failover` handler
- [ ] Support optional target follower
- [ ] Return old/new leader info
- [ ] Write handler tests

---

### Task 5.8: Manager Integration
**File**: `internal/replication/manager.go` (update)
**Estimated**: 3 hours

- [ ] Integrate `LeaderHealthMonitor` for followers
- [ ] Integrate `Election` state machine
- [ ] Integrate `FailoverManager`
- [ ] Update `Start()` to initialize components
- [ ] Update `Stop()` to cleanup components
- [ ] Handle leader change notifications
- [ ] Update follower list on leadership change
- [ ] Write integration tests

---

### Task 5.9: Server Integration for Phase 5
**File**: `internal/server/server.go`
**Estimated**: 1 hour

- [ ] Add election handlers to replication routes
- [ ] Add failover handler to admin routes
- [ ] Configure election/failover from config
- [ ] Handle graceful leadership transfer on shutdown

---

### Task 5.10: Configuration Updates
**File**: `internal/config/config.go`
**Estimated**: 1 hour

- [ ] Add `ElectionConfig` to `ReplicationConfig`
- [ ] Add `FailoverConfig` to `ReplicationConfig`
- [ ] Add validation for new config fields
- [ ] Add sensible defaults
- [ ] Update config documentation

---

## Documentation & Testing

### Task 6.1: Disaster Recovery Runbook
**File**: `docs/disaster-recovery.md`
**Estimated**: 2 hours

- [ ] Document manual failover procedure
- [ ] Document recovery from split-brain
- [ ] Document data reconciliation after partition
- [ ] Document backup/restore during failover
- [ ] Add troubleshooting section

---

### Task 6.2: Replication Operations Guide
**File**: `docs/replication-operations.md`
**Estimated**: 2 hours

- [ ] Document replication architecture
- [ ] Document tenant migration procedure
- [ ] Document monitoring and alerting
- [ ] Document scaling replicas
- [ ] Add common operational tasks

---

### Task 6.3: CLI Commands for Replication
**File**: `cmd/maiactl/cmd/replication.go`
**Estimated**: 3 hours

- [ ] Add `maiactl replication status` command
- [ ] Add `maiactl replication health` command
- [ ] Add `maiactl replication failover` command
- [ ] Add `maiactl migrate start` command
- [ ] Add `maiactl migrate status` command
- [ ] Add `maiactl migrate cancel` command
- [ ] Write command tests

---

### Task 6.4: Integration Tests
**File**: `internal/replication/integration_test.go`
**Estimated**: 4 hours

- [ ] Test end-to-end tenant migration
- [ ] Test automatic failover scenario
- [ ] Test leader election with multiple candidates
- [ ] Test split-brain prevention
- [ ] Test routing middleware with placement

---

### Task 6.5: Update State and Documentation
**Files**: `.agent/state.md`, `README.md`
**Estimated**: 1 hour

- [ ] Update state.md with Phase 3-5 progress
- [ ] Update README roadmap
- [ ] Update API documentation
- [ ] Update configuration documentation

---

## Task Summary

| Phase | Tasks | Estimated Time |
|-------|-------|----------------|
| Phase 3 | 7 tasks | ~15 hours |
| Phase 4 | 5 tasks | ~8.5 hours |
| Phase 5 | 10 tasks | ~19 hours |
| Docs/Testing | 5 tasks | ~12 hours |
| **Total** | **27 tasks** | **~54.5 hours** |

## Execution Order

### Week 1: Phase 3
1. Task 3.1 (Placement Cache)
2. Task 3.2 (Routing Middleware)
3. Task 3.3 (Migration Types)
4. Task 3.4 (Migration Storage)
5. Task 3.5 (Migration Executor)
6. Task 3.6 (Migration Handlers)
7. Task 3.7 (Server Integration)

### Week 2: Phase 4
1. Task 4.1 (Health Endpoint)
2. Task 4.2 (Metrics)
3. Task 4.3 (Grafana Dashboard)
4. Task 4.4 (Alert Rules)
5. Task 4.5 (docker-compose update)

### Week 3-4: Phase 5
1. Task 5.1 (Leader Health Monitor)
2. Task 5.2 (Election Types)
3. Task 5.3 (Election State Machine)
4. Task 5.4 (Election Handlers)
5. Task 5.5 (Failover Types)
6. Task 5.6 (Automatic Failover)
7. Task 5.7 (Failover Handler)
8. Task 5.8 (Manager Integration)
9. Task 5.9 (Server Integration)
10. Task 5.10 (Configuration)

### Week 5: Documentation & Testing
1. Task 6.1 (DR Runbook)
2. Task 6.2 (Operations Guide)
3. Task 6.3 (CLI Commands)
4. Task 6.4 (Integration Tests)
5. Task 6.5 (State/README Update)
