# MAIA Kubernetes Operator

The MAIA Operator provides Kubernetes-native management for MAIA instances and tenants through Custom Resource Definitions (CRDs).

## Overview

The operator manages two custom resources:

- **MaiaInstance**: Deploys and configures MAIA server instances
- **MaiaTenant**: Manages tenants within MAIA instances

## Features

- **Declarative Configuration**: Define MAIA deployments as Kubernetes resources
- **Automatic Resource Management**: Creates Deployments, Services, ConfigMaps, PVCs, Ingresses
- **Prometheus Integration**: Automatically creates ServiceMonitor for Prometheus scraping
- **Automated Backups**: Creates CronJobs for scheduled backups with compression and retention
- **Tenant Lifecycle**: Create, update, suspend, and delete tenants via the MAIA Admin API
- **API Key Provisioning**: Automatically creates API keys and stores them in Kubernetes Secrets
- **Status Tracking**: Rich status updates with conditions and metrics
- **High Availability**: Leader election support for running multiple operator replicas

## Installation

### Prerequisites

- Kubernetes 1.35+
- kubectl configured with cluster access

### Install CRDs

```bash
kubectl apply -f deployments/kubernetes/crds/
```

### Deploy the Operator

```bash
# Using manifests
kubectl apply -f operator/config/manager/namespace.yaml
kubectl apply -f operator/config/rbac/
kubectl apply -f operator/config/manager/

# Or using kustomize
kubectl apply -k operator/config/rbac/
kubectl apply -k operator/config/manager/
```

### Verify Installation

```bash
kubectl get pods -n maia-system
# NAME                             READY   STATUS    RESTARTS   AGE
# maia-operator-7b9d8f6c4-x2j5k   1/1     Running   0          30s
```

## Quick Start

### 1. Create an API Key Secret

```bash
kubectl create secret generic maia-api-key \
  --from-literal=api-key=$(openssl rand -hex 32)
```

### 2. Deploy a MaiaInstance

```yaml
apiVersion: maia.cuemby.com/v1alpha1
kind: MaiaInstance
metadata:
  name: maia
spec:
  replicas: 1
  image:
    repository: ghcr.io/ar4mirez/maia
    tag: v1.0.0
  storage:
    size: 10Gi
  security:
    apiKeySecretRef:
      name: maia-api-key
  tenancy:
    enabled: true
  metrics:
    enabled: true
```

```bash
kubectl apply -f maia-instance.yaml
```

### 3. Check Status

```bash
kubectl get maiainstances
# NAME   STATUS    TENANTS   MEMORIES   AGE
# maia   Running   0         0          2m

kubectl get pods
# NAME                    READY   STATUS    RESTARTS   AGE
# maia-7b9d8f6c4-x2j5k   1/1     Running   0          2m
```

### 4. Create a Tenant

```yaml
apiVersion: maia.cuemby.com/v1alpha1
kind: MaiaTenant
metadata:
  name: demo
spec:
  instanceRef:
    name: maia
  plan: free
  quotas:
    maxMemories: 1000
    maxStorageBytes: 104857600  # 100MB
  apiKeys:
  - name: demo-key
    secretRef:
      name: demo-api-key
    scopes: ["read", "write", "context"]
```

```bash
kubectl apply -f demo-tenant.yaml
```

### 5. Get API Key

```bash
kubectl get secret demo-api-key -o jsonpath='{.data.api-key}' | base64 -d
```

## MaiaInstance Reference

### Spec

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `replicas` | int32 | 1 | Number of MAIA pods (1-10) |
| `image.repository` | string | "maia" | Container image repository |
| `image.tag` | string | "latest" | Container image tag |
| `image.pullPolicy` | string | "IfNotPresent" | Image pull policy |
| `storage.size` | string | "10Gi" | PVC storage size |
| `storage.storageClassName` | string | - | Storage class name |
| `storage.dataDir` | string | "/data" | Data directory path |
| `storage.syncWrites` | bool | false | Enable synchronous writes |
| `storage.gcInterval` | string | "5m" | Garbage collection interval |
| `security.apiKeySecretRef` | SecretRef | - | API key secret reference |
| `security.tlsSecretRef` | SecretRef | - | TLS secret reference |
| `embedding.model` | string | "local" | Embedding model (local, openai, ollama) |
| `embedding.openaiSecretRef` | SecretRef | - | OpenAI API key secret |
| `embedding.ollamaEndpoint` | string | - | Ollama endpoint URL |
| `tenancy.enabled` | bool | false | Enable multi-tenancy |
| `tenancy.requireTenant` | bool | false | Require tenant header |
| `tenancy.defaultTenantId` | string | "default" | Default tenant ID |
| `tenancy.enforceScopesEnabled` | bool | false | Enforce API key scopes |
| `tenancy.dedicatedStorage` | bool | false | Per-tenant storage |
| `rateLimit.enabled` | bool | false | Enable rate limiting |
| `rateLimit.requestsPerSecond` | int | 100 | Rate limit RPS |
| `rateLimit.burst` | int | 200 | Rate limit burst |
| `logging.level` | string | "info" | Log level (debug, info, warn, error) |
| `logging.format` | string | "json" | Log format (json, text) |
| `metrics.enabled` | bool | true | Enable Prometheus metrics |
| `metrics.serviceMonitor.enabled` | bool | false | Create ServiceMonitor |
| `metrics.serviceMonitor.interval` | string | "30s" | Scrape interval |
| `ingress.enabled` | bool | false | Create Ingress |
| `ingress.className` | string | - | Ingress class name |
| `ingress.host` | string | - | Ingress hostname |
| `ingress.tls` | bool | false | Enable TLS |
| `resources.limits.cpu` | string | "1000m" | CPU limit |
| `resources.limits.memory` | string | "1Gi" | Memory limit |
| `resources.requests.cpu` | string | "100m" | CPU request |
| `resources.requests.memory` | string | "256Mi" | Memory request |
| `backup.enabled` | bool | false | Enable backups |
| `backup.schedule` | string | "0 2 * * *" | Cron schedule |
| `backup.retentionDays` | int | 30 | Retention period |

### Status

| Field | Type | Description |
|-------|------|-------------|
| `phase` | string | Lifecycle phase (Pending, Running, Failed, Updating) |
| `conditions` | []Condition | Status conditions |
| `replicas` | int32 | Current replica count |
| `readyReplicas` | int32 | Ready replica count |
| `tenantCount` | int | Number of tenants |
| `totalMemories` | int | Total memories stored |
| `storageUsed` | string | Storage usage |
| `lastBackup` | Time | Last backup timestamp |
| `version` | string | MAIA version |
| `endpoint` | string | Service endpoint URL |

## MaiaTenant Reference

### Spec

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `instanceRef.name` | string | **required** | MaiaInstance name |
| `instanceRef.namespace` | string | same | MaiaInstance namespace |
| `displayName` | string | - | Human-readable name |
| `plan` | string | "free" | Plan tier (free, starter, professional, enterprise) |
| `quotas.maxMemories` | int64 | - | Max memories (0 = unlimited) |
| `quotas.maxStorageBytes` | int64 | - | Max storage bytes |
| `quotas.maxNamespaces` | int | 10 | Max namespaces |
| `quotas.requestsPerMinute` | int | - | Rate limit RPM |
| `quotas.requestsPerDay` | int64 | - | Rate limit RPD |
| `config.defaultTokenBudget` | int | 4000 | Default token budget |
| `config.maxTokenBudget` | int | 16000 | Max token budget |
| `config.features.inference` | bool | false | Enable inference |
| `config.features.vectorSearch` | bool | true | Enable vector search |
| `config.features.fullTextSearch` | bool | true | Enable full-text search |
| `config.features.contextAssembly` | bool | true | Enable context assembly |
| `apiKeys[].name` | string | **required** | API key name |
| `apiKeys[].secretRef.name` | string | **required** | Secret name |
| `apiKeys[].secretRef.key` | string | "api-key" | Secret key |
| `apiKeys[].scopes` | []string | ["read","write"] | API key scopes |
| `apiKeys[].expiresAt` | Time | - | Expiration time |
| `suspended` | bool | false | Suspend tenant |
| `suspendReason` | string | - | Suspension reason |

### Status

| Field | Type | Description |
|-------|------|-------------|
| `phase` | string | Lifecycle phase (Pending, Active, Suspended, Failed) |
| `conditions` | []Condition | Status conditions |
| `tenantId` | string | MAIA tenant ID |
| `memoryCount` | int | Number of memories |
| `namespaceCount` | int | Number of namespaces |
| `storageUsed` | int64 | Storage used (bytes) |
| `quotaUsage.memories` | float64 | Memory quota usage % |
| `quotaUsage.storage` | float64 | Storage quota usage % |
| `apiKeyCount` | int | Number of API keys |
| `lastActivity` | Time | Last activity timestamp |
| `createdAt` | Time | Creation timestamp |

## Examples

### Production Deployment

```yaml
apiVersion: maia.cuemby.com/v1alpha1
kind: MaiaInstance
metadata:
  name: maia-production
spec:
  replicas: 3

  image:
    repository: ghcr.io/ar4mirez/maia
    tag: v1.0.0
    pullPolicy: IfNotPresent

  storage:
    size: 100Gi
    storageClassName: fast-ssd
    syncWrites: true
    gcInterval: 10m

  security:
    apiKeySecretRef:
      name: maia-api-key
    tlsSecretRef:
      name: maia-tls

  embedding:
    model: openai
    openaiSecretRef:
      name: openai-api-key

  tenancy:
    enabled: true
    requireTenant: true
    enforceScopesEnabled: true
    dedicatedStorage: true

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
    annotations:
      cert-manager.io/cluster-issuer: letsencrypt-prod

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
    compress: true
```

### Enterprise Tenant

```yaml
apiVersion: maia.cuemby.com/v1alpha1
kind: MaiaTenant
metadata:
  name: enterprise-customer
spec:
  instanceRef:
    name: maia-production

  displayName: "Enterprise Customer Inc."
  plan: enterprise

  quotas:
    maxMemories: 1000000
    maxStorageBytes: 107374182400  # 100GB
    maxNamespaces: 100
    requestsPerMinute: 1000
    requestsPerDay: 0  # Unlimited

  config:
    defaultTokenBudget: 8000
    maxTokenBudget: 32000
    features:
      inference: true
      vectorSearch: true
      fullTextSearch: true
      contextAssembly: true

  apiKeys:
  - name: admin
    secretRef:
      name: enterprise-admin-key
    scopes: ["*"]

  - name: service
    secretRef:
      name: enterprise-service-key
    scopes: ["read", "write", "context", "inference"]

  - name: readonly
    secretRef:
      name: enterprise-readonly-key
    scopes: ["read", "search"]
```

## Prometheus Integration

When `metrics.serviceMonitor.enabled` is set to `true`, the operator automatically creates a ServiceMonitor resource for Prometheus to scrape MAIA metrics.

### ServiceMonitor Prerequisites

- Prometheus Operator installed (provides ServiceMonitor CRD)
- Prometheus configured to discover ServiceMonitors

### ServiceMonitor Configuration

```yaml
apiVersion: maia.cuemby.com/v1alpha1
kind: MaiaInstance
metadata:
  name: maia
spec:
  metrics:
    enabled: true
    serviceMonitor:
      enabled: true
      interval: 30s
      labels:
        release: prometheus  # Match your Prometheus selector
```

### Verifying ServiceMonitor Creation

```bash
# Check ServiceMonitor was created
kubectl get servicemonitor maia

# Verify Prometheus is scraping
kubectl port-forward -n monitoring svc/prometheus 9090:9090
# Visit http://localhost:9090/targets and look for MAIA
```

The ServiceMonitor is automatically configured to:

- Scrape the `/metrics` endpoint on the `http` port
- Use the configured scrape interval (default: 30s)
- Include labels from `spec.metrics.serviceMonitor.labels`

## Automated Backups

When `backup.enabled` is set to `true`, the operator creates a CronJob that performs scheduled backups of MAIA data.

### Backup Features

- **Scheduled execution**: Configurable cron schedule (default: `0 2 * * *` - 2 AM daily)
- **Compression**: Optional gzip compression (enabled by default)
- **Retention policy**: Automatic cleanup of old backups
- **Dedicated storage**: Separate PVC for backup data

### Backup Configuration

```yaml
apiVersion: maia.cuemby.com/v1alpha1
kind: MaiaInstance
metadata:
  name: maia
spec:
  backup:
    enabled: true
    schedule: "0 2 * * *"     # 2 AM daily
    retentionDays: 30         # Keep backups for 30 days
    storageSize: "20Gi"       # Backup PVC size
    compress: true            # Enable gzip compression
```

### Verifying Backups

```bash
# Check CronJob was created
kubectl get cronjob maia-backup

# View backup history
kubectl get jobs -l app.kubernetes.io/component=backup

# Check backup logs
kubectl logs job/maia-backup-<timestamp>

# List backup files
kubectl exec -it maia-0 -- ls -la /backup
```

### Backup Storage

Backups are stored in a dedicated PVC:

- **Name**: `<instance-name>-backup`
- **Size**: Configured via `backup.storageSize`
- **Path**: `/backup` in the backup job container
- **Format**: `backup-YYYYMMDD-HHMMSS.tar.gz` (compressed) or `backup-YYYYMMDD-HHMMSS.tar`

### Retention

The backup job automatically removes backups older than `retentionDays`:

```bash
# Backups older than retention period are deleted
find /backup -name "backup-*.tar*" -mtime +30 -delete
```

### Manual Backup

To trigger a manual backup:

```bash
kubectl create job --from=cronjob/maia-backup maia-backup-manual
```

### Restoring from Backup

```bash
# Copy backup to local machine
kubectl cp maia-backup-pod:/backup/backup-20250121-020000.tar.gz ./backup.tar.gz

# Extract and restore
tar -xzf backup.tar.gz
# Follow MAIA restore documentation
```

## Troubleshooting

### Operator Not Starting

Check logs:
```bash
kubectl logs -n maia-system deployment/maia-operator
```

### MaiaInstance Stuck in Pending

Check events:
```bash
kubectl describe maiainstance <name>
```

Common causes:
- Missing API key secret
- Storage class not available
- Insufficient resources

### MaiaTenant Not Syncing

Check conditions:
```bash
kubectl get maiatenant <name> -o jsonpath='{.status.conditions}'
```

Common causes:
- MaiaInstance not ready
- Network connectivity to MAIA server
- Invalid API key

### View Operator Metrics

```bash
kubectl port-forward -n maia-system deployment/maia-operator 8080:8080
curl http://localhost:8080/metrics
```
