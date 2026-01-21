# RFD 0006: Kubernetes Operator for MAIA

## Status

**State**: Approved
**Created**: 2026-01-21
**Author**: MAIA Development Team

---

## Summary

Implement a Kubernetes Operator for MAIA using controller-runtime v0.23.0+ to provide native Kubernetes management of MAIA instances and tenants via the existing `MaiaInstance` and `MaiaTenant` Custom Resource Definitions.

---

## Motivation

Currently, MAIA can be deployed to Kubernetes using:
1. Raw manifests in `deployments/kubernetes/`
2. Helm chart in `deployments/helm/maia/`

However, these approaches require manual management of:
- MAIA server configuration
- Tenant lifecycle management
- API key provisioning
- Quota enforcement
- Status monitoring

A Kubernetes Operator provides:
- **Declarative Management**: Define desired state in CRs, operator ensures reality matches
- **Automated Tenant Lifecycle**: Create/update/delete tenants via Kubernetes resources
- **Native Integration**: Works with GitOps, kubectl, Kubernetes RBAC
- **Self-Healing**: Automatic reconciliation on drift or failures
- **Status Reporting**: Rich status subresources for observability

---

## Requirements

### Functional Requirements

1. **MaiaInstance Controller**
   - Create/update Kubernetes Deployment from MaiaInstance spec
   - Manage PersistentVolumeClaim for data storage
   - Generate ConfigMap from spec configuration
   - Create Services (ClusterIP and headless)
   - Optionally create Ingress resources
   - Optionally create ServiceMonitor for Prometheus
   - Optionally create CronJob for backups
   - Track status: phase, readyReplicas, endpoint, storage usage

2. **MaiaTenant Controller**
   - Create tenants in MAIA via Admin API
   - Sync quotas and configuration
   - Provision API keys from Kubernetes Secrets
   - Handle tenant suspension/activation
   - Track status: phase, memoryCount, storageUsed, quotaUsage
   - Clean up tenant on CR deletion

3. **API Integration**
   - HTTP client for MAIA Admin API
   - Automatic discovery of MAIA endpoint from MaiaInstance
   - Authentication via API key from Secret

### Non-Functional Requirements

1. **Kubernetes Compatibility**: 1.35+
2. **Controller-runtime**: v0.23.0+ (k8s.io/* v1.35 dependencies)
3. **Reconciliation**: Leader election for HA
4. **Observability**: Prometheus metrics, structured logging
5. **Testing**: >80% coverage on controller logic

---

## Design

### Project Structure

```
operator/
├── cmd/
│   └── operator/
│       └── main.go              # Operator entrypoint
├── api/
│   └── v1alpha1/
│       ├── groupversion_info.go # API group registration
│       ├── maiainstance_types.go
│       ├── maiainstance_webhook.go
│       ├── maiatenent_types.go
│       ├── maiatenent_webhook.go
│       └── zz_generated.deepcopy.go
├── internal/
│   └── controller/
│       ├── maiainstance_controller.go
│       ├── maiainstance_controller_test.go
│       ├── maiatenent_controller.go
│       ├── maiatenent_controller_test.go
│       └── suite_test.go        # envtest setup
├── pkg/
│   └── maia/
│       ├── client.go            # MAIA Admin API client
│       └── client_test.go
├── config/
│   ├── crd/                     # Generated CRD manifests
│   ├── rbac/                    # RBAC configuration
│   ├── manager/                 # Manager deployment
│   └── samples/                 # Example CRs
├── Dockerfile
├── Makefile
└── PROJECT                      # Kubebuilder project file
```

### Controller-Runtime Integration

Using controller-runtime v0.23.0 features:

```go
// Manager setup with leader election
mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
    Scheme:                 scheme,
    Metrics:                metricsserver.Options{BindAddress: ":8080"},
    HealthProbeBindAddress: ":8081",
    LeaderElection:         true,
    LeaderElectionID:       "maia-operator.cuemby.com",
})

// Controller registration
if err := (&MaiaInstanceReconciler{
    Client:   mgr.GetClient(),
    Scheme:   mgr.GetScheme(),
    Recorder: mgr.GetEventRecorderFor("maiainstance-controller"),
}).SetupWithManager(mgr); err != nil {
    // handle error
}
```

### MaiaInstance Reconciliation Flow

```
┌─────────────────────────────────────────────────────────────────┐
│                    MaiaInstance Reconciler                       │
├─────────────────────────────────────────────────────────────────┤
│                                                                  │
│  1. Fetch MaiaInstance CR                                       │
│     └── Not found? Return (deleted)                             │
│                                                                  │
│  2. Handle Deletion (if DeletionTimestamp set)                  │
│     ├── Remove finalizer                                        │
│     └── Return                                                  │
│                                                                  │
│  3. Add Finalizer (if not present)                              │
│                                                                  │
│  4. Reconcile ConfigMap                                         │
│     ├── Generate config from spec                               │
│     └── Create/Update ConfigMap                                 │
│                                                                  │
│  5. Reconcile PVC (if not exists)                               │
│     └── Create PVC with spec.storage settings                   │
│                                                                  │
│  6. Reconcile Deployment                                        │
│     ├── Build Deployment from spec                              │
│     ├── Set image, replicas, resources                          │
│     ├── Mount ConfigMap and PVC                                 │
│     ├── Set env vars from secrets                               │
│     └── Create/Update Deployment                                │
│                                                                  │
│  7. Reconcile Services                                          │
│     ├── Create ClusterIP service                                │
│     └── Create Headless service                                 │
│                                                                  │
│  8. Reconcile Ingress (if enabled)                              │
│     └── Create/Update Ingress resource                          │
│                                                                  │
│  9. Reconcile ServiceMonitor (if enabled)                       │
│     └── Create/Update ServiceMonitor                            │
│                                                                  │
│ 10. Reconcile Backup CronJob (if enabled)                       │
│     └── Create/Update CronJob                                   │
│                                                                  │
│ 11. Update Status                                               │
│     ├── Set phase (Pending/Running/Failed)                      │
│     ├── Set readyReplicas                                       │
│     ├── Set endpoint URL                                        │
│     └── Set conditions                                          │
│                                                                  │
│ 12. Requeue after 30s for status sync                           │
│                                                                  │
└─────────────────────────────────────────────────────────────────┘
```

### MaiaTenant Reconciliation Flow

```
┌─────────────────────────────────────────────────────────────────┐
│                    MaiaTenant Reconciler                         │
├─────────────────────────────────────────────────────────────────┤
│                                                                  │
│  1. Fetch MaiaTenant CR                                         │
│     └── Not found? Return (deleted)                             │
│                                                                  │
│  2. Fetch Referenced MaiaInstance                               │
│     └── Not found/not ready? Requeue with backoff               │
│                                                                  │
│  3. Get MAIA API Client                                         │
│     ├── Get endpoint from MaiaInstance status                   │
│     ├── Get API key from Secret                                 │
│     └── Create HTTP client                                      │
│                                                                  │
│  4. Handle Deletion (if DeletionTimestamp set)                  │
│     ├── Delete tenant via API                                   │
│     ├── Remove finalizer                                        │
│     └── Return                                                  │
│                                                                  │
│  5. Add Finalizer (if not present)                              │
│                                                                  │
│  6. Check if Tenant Exists in MAIA                              │
│     ├── Get tenant by ID                                        │
│     └── If not found, create                                    │
│                                                                  │
│  7. Create Tenant (if not exists)                               │
│     ├── Build CreateTenantRequest from spec                     │
│     ├── Call POST /admin/tenants                                │
│     └── Store tenant ID in status                               │
│                                                                  │
│  8. Update Tenant (if exists)                                   │
│     ├── Compare spec with current state                         │
│     ├── Update quotas, config via API                           │
│     └── Handle suspension/activation                            │
│                                                                  │
│  9. Reconcile API Keys                                          │
│     ├── List current API keys from MAIA                         │
│     ├── Create missing keys from spec                           │
│     ├── Revoke keys not in spec                                 │
│     └── Store keys in referenced Secrets                        │
│                                                                  │
│ 10. Update Status                                               │
│     ├── Set phase (Pending/Active/Suspended/Failed)             │
│     ├── Fetch usage from MAIA API                               │
│     ├── Set memoryCount, storageUsed                            │
│     ├── Calculate quotaUsage percentages                        │
│     └── Set conditions                                          │
│                                                                  │
│ 11. Requeue after 60s for status sync                           │
│                                                                  │
└─────────────────────────────────────────────────────────────────┘
```

### MAIA Admin API Client

```go
// pkg/maia/client.go
type Client struct {
    baseURL    string
    apiKey     string
    httpClient *http.Client
}

type ClientOption func(*Client)

func NewClient(baseURL string, opts ...ClientOption) *Client

// Tenant operations
func (c *Client) CreateTenant(ctx context.Context, req *CreateTenantRequest) (*Tenant, error)
func (c *Client) GetTenant(ctx context.Context, id string) (*Tenant, error)
func (c *Client) UpdateTenant(ctx context.Context, id string, req *UpdateTenantRequest) (*Tenant, error)
func (c *Client) DeleteTenant(ctx context.Context, id string) error
func (c *Client) GetTenantUsage(ctx context.Context, id string) (*Usage, error)
func (c *Client) SuspendTenant(ctx context.Context, id string, reason string) error
func (c *Client) ActivateTenant(ctx context.Context, id string) error

// API Key operations
func (c *Client) CreateAPIKey(ctx context.Context, tenantID string, req *CreateAPIKeyRequest) (*APIKeyResponse, error)
func (c *Client) ListAPIKeys(ctx context.Context, tenantID string) ([]APIKey, error)
func (c *Client) RevokeAPIKey(ctx context.Context, key string) error

// Health
func (c *Client) Health(ctx context.Context) error
```

### Status Conditions

Following Kubernetes conventions:

```go
// MaiaInstance conditions
const (
    ConditionTypeReady       = "Ready"
    ConditionTypeProgressing = "Progressing"
    ConditionTypeDegraded    = "Degraded"
)

// MaiaTenant conditions
const (
    ConditionTypeSynced    = "Synced"
    ConditionTypeReady     = "Ready"
    ConditionTypeSuspended = "Suspended"
)
```

### RBAC Requirements

```yaml
# Controller permissions
rules:
- apiGroups: ["maia.cuemby.com"]
  resources: ["maiainstances", "maiatenants"]
  verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]
- apiGroups: ["maia.cuemby.com"]
  resources: ["maiainstances/status", "maiatenants/status"]
  verbs: ["get", "update", "patch"]
- apiGroups: ["maia.cuemby.com"]
  resources: ["maiainstances/finalizers", "maiatenants/finalizers"]
  verbs: ["update"]
- apiGroups: [""]
  resources: ["configmaps", "secrets", "services", "persistentvolumeclaims"]
  verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]
- apiGroups: ["apps"]
  resources: ["deployments"]
  verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]
- apiGroups: ["batch"]
  resources: ["cronjobs"]
  verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]
- apiGroups: ["networking.k8s.io"]
  resources: ["ingresses"]
  verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]
- apiGroups: ["monitoring.coreos.com"]
  resources: ["servicemonitors"]
  verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]
- apiGroups: [""]
  resources: ["events"]
  verbs: ["create", "patch"]
- apiGroups: ["coordination.k8s.io"]
  resources: ["leases"]
  verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]
```

---

## Implementation Plan

### Phase 1: Project Setup
- Initialize operator module with controller-runtime v0.23.0
- Define API types matching existing CRDs
- Generate DeepCopy functions
- Set up envtest for testing

### Phase 2: MaiaInstance Controller
- Implement ConfigMap reconciliation
- Implement PVC reconciliation
- Implement Deployment reconciliation
- Implement Service reconciliation
- Implement status updates
- Add tests with envtest

### Phase 3: MaiaTenant Controller
- Implement MAIA Admin API client
- Implement tenant creation/update
- Implement API key management
- Implement status sync
- Add tests with mock MAIA server

### Phase 4: Optional Resources
- Implement Ingress reconciliation
- Implement ServiceMonitor reconciliation
- Implement backup CronJob reconciliation

### Phase 5: Deployment & Documentation
- Create operator Dockerfile
- Create Helm chart for operator
- Write deployment documentation
- Create usage examples

---

## Dependencies

```go
require (
    k8s.io/api v0.35.0
    k8s.io/apimachinery v0.35.0
    k8s.io/client-go v0.35.0
    sigs.k8s.io/controller-runtime v0.23.0
)
```

---

## Alternatives Considered

### 1. Operator SDK with OLM

**Pros**: OLM integration, CSV generation
**Cons**: Additional complexity, OLM dependency
**Decision**: Use controller-runtime directly for simplicity

### 2. Kubebuilder Scaffolding

**Pros**: Quick setup, conventions
**Cons**: Generated code needs customization for existing CRDs
**Decision**: Manual setup to match existing CRD structure

### 3. Kopf (Python Operator Framework)

**Pros**: Simpler development, Python ecosystem
**Cons**: Different language from MAIA, performance overhead
**Decision**: Stay with Go for consistency

---

## Security Considerations

1. **RBAC**: Minimum required permissions
2. **Secrets**: API keys stored in Kubernetes Secrets
3. **Network**: Operator communicates with MAIA via cluster network
4. **Service Account**: Dedicated SA with scoped permissions

---

## Testing Strategy

1. **Unit Tests**: Controller logic with mock clients
2. **Integration Tests**: envtest with real API server
3. **E2E Tests**: Kind cluster with full operator deployment

---

## References

- [controller-runtime v0.23.0 Release](https://github.com/kubernetes-sigs/controller-runtime/releases/tag/v0.23.0)
- [Kubernetes 1.35 Release](https://kubernetes.io/blog/2025/11/26/kubernetes-v1-35-sneak-peek/)
- Existing CRDs: `deployments/kubernetes/crds/`
- MAIA Admin API: `internal/server/admin_handlers.go`
