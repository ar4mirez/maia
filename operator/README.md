# MAIA Operator

Kubernetes Operator for MAIA (Memory AI Architecture) - managing MAIA instances and tenants declaratively.

## Overview

The MAIA Operator provides Kubernetes-native management for:

- **MaiaInstance**: Deploy and configure MAIA server instances
- **MaiaTenant**: Manage tenants within MAIA instances

## Features

- Declarative configuration via Custom Resources
- Automatic resource management (Deployments, Services, ConfigMaps, PVCs)
- Tenant lifecycle management via MAIA Admin API
- API key provisioning and secret management
- Status tracking and condition reporting
- Leader election for high availability
- Prometheus metrics

## Requirements

- Kubernetes 1.35+
- kubectl configured with cluster access
- MAIA CRDs installed

## Installation

### 1. Install CRDs

```bash
kubectl apply -f ../deployments/kubernetes/crds/
```

### 2. Deploy the Operator

```bash
# Create namespace
kubectl apply -f config/manager/namespace.yaml

# Deploy RBAC
kubectl apply -f config/rbac/

# Deploy operator
kubectl apply -f config/manager/
```

Or using kustomize:

```bash
kubectl apply -k config/rbac/
kubectl apply -k config/manager/
```

### 3. Verify Installation

```bash
kubectl get pods -n maia-system
kubectl logs -n maia-system deployment/maia-operator
```

## Usage

### Create a MaiaInstance

```yaml
apiVersion: maia.cuemby.com/v1alpha1
kind: MaiaInstance
metadata:
  name: maia-production
  namespace: default
spec:
  replicas: 3

  image:
    repository: ghcr.io/ar4mirez/maia
    tag: v1.0.0

  storage:
    size: 50Gi
    storageClassName: fast-ssd
    dataDir: /data
    syncWrites: true

  security:
    apiKeySecretRef:
      name: maia-api-key
      key: api-key

  embedding:
    model: local

  tenancy:
    enabled: true
    requireTenant: true
    enforceScopesEnabled: true

  rateLimit:
    enabled: true
    requestsPerSecond: 100
    burst: 200

  logging:
    level: info
    format: json

  metrics:
    enabled: true
    serviceMonitor:
      enabled: true
      interval: 30s
      labels:
        release: prometheus

  ingress:
    enabled: true
    className: nginx
    host: maia.example.com
    tls: true

  resources:
    limits:
      cpu: "4"
      memory: 8Gi
    requests:
      cpu: "1"
      memory: 2Gi

  backup:
    enabled: true
    schedule: "0 2 * * *"
    retentionDays: 30
```

Apply it:

```bash
kubectl apply -f maia-instance.yaml
```

Check status:

```bash
kubectl get maiainstances
kubectl describe maiainstance maia-production
```

### Create a MaiaTenant

```yaml
apiVersion: maia.cuemby.com/v1alpha1
kind: MaiaTenant
metadata:
  name: acme-corp
  namespace: default
spec:
  instanceRef:
    name: maia-production

  displayName: "Acme Corporation"
  plan: professional

  quotas:
    maxMemories: 100000
    maxStorageBytes: 10737418240  # 10GB
    maxNamespaces: 50
    requestsPerMinute: 600
    requestsPerDay: 100000

  config:
    defaultTokenBudget: 8000
    maxTokenBudget: 16000
    features:
      inference: true
      vectorSearch: true
      fullTextSearch: true
      contextAssembly: true

  apiKeys:
  - name: production-key
    secretRef:
      name: acme-corp-api-key
      key: api-key
    scopes:
    - "*"

  - name: readonly-key
    secretRef:
      name: acme-corp-readonly-key
      key: api-key
    scopes:
    - read
    - search
    - context
```

Apply it:

```bash
kubectl apply -f maia-tenant.yaml
```

Check status:

```bash
kubectl get maiatenants
kubectl describe maiatenant acme-corp
```

### Suspend a Tenant

```yaml
apiVersion: maia.cuemby.com/v1alpha1
kind: MaiaTenant
metadata:
  name: acme-corp
spec:
  # ... existing spec ...
  suspended: true
  suspendReason: "Payment overdue"
```

## API Reference

### MaiaInstance

| Field | Type | Description |
|-------|------|-------------|
| `spec.replicas` | int | Number of replicas (1-10) |
| `spec.image` | ImageSpec | Container image configuration |
| `spec.storage` | StorageSpec | Storage configuration |
| `spec.security` | SecuritySpec | Security settings (API keys, TLS) |
| `spec.embedding` | EmbeddingSpec | Embedding model configuration |
| `spec.tenancy` | TenancySpec | Multi-tenancy settings |
| `spec.rateLimit` | RateLimitSpec | Rate limiting configuration |
| `spec.logging` | LoggingSpec | Logging configuration |
| `spec.metrics` | MetricsSpec | Metrics and ServiceMonitor settings |
| `spec.ingress` | IngressSpec | Ingress configuration |
| `spec.resources` | ResourcesSpec | Resource requirements |
| `spec.backup` | BackupSpec | Backup configuration |

### MaiaTenant

| Field | Type | Description |
|-------|------|-------------|
| `spec.instanceRef` | InstanceReference | Reference to MaiaInstance |
| `spec.displayName` | string | Human-readable name |
| `spec.plan` | string | Plan tier (free, starter, professional, enterprise) |
| `spec.quotas` | TenantQuotas | Resource quotas |
| `spec.config` | TenantConfig | Tenant configuration |
| `spec.apiKeys` | []TenantAPIKey | API key definitions |
| `spec.suspended` | bool | Suspend the tenant |
| `spec.suspendReason` | string | Reason for suspension |

## Development

### Prerequisites

- Go 1.23+
- Docker
- kubectl
- Access to a Kubernetes cluster

### Build

```bash
# Generate code
make generate

# Build binary
make build

# Run locally
make run

# Build Docker image
make docker-build IMG=my-registry/maia-operator:dev

# Push Docker image
make docker-push IMG=my-registry/maia-operator:dev
```

### Test

```bash
# Run unit tests
make test

# Run e2e tests (requires envtest)
make test-e2e
```

### Lint

```bash
make lint
```

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                    Kubernetes Cluster                        │
├─────────────────────────────────────────────────────────────┤
│                                                              │
│  ┌─────────────┐    watches    ┌─────────────────┐          │
│  │ MaiaInstance │◄─────────────│  MAIA Operator  │          │
│  └──────┬──────┘               └────────┬────────┘          │
│         │                               │                    │
│         │ creates/manages               │ creates/manages    │
│         ▼                               ▼                    │
│  ┌─────────────┐               ┌─────────────────┐          │
│  │  Deployment │               │   MaiaTenant    │          │
│  │   Service   │               └────────┬────────┘          │
│  │  ConfigMap  │                        │                    │
│  │     PVC     │                        │ API calls          │
│  │   Ingress   │                        ▼                    │
│  └─────────────┘               ┌─────────────────┐          │
│                                │  MAIA Server    │          │
│                                │  Admin API      │          │
│                                └─────────────────┘          │
│                                                              │
└─────────────────────────────────────────────────────────────┘
```

## License

Apache License 2.0
