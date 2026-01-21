# MAIA Deployment Guide

This guide covers deploying MAIA in various environments, from local development to production Kubernetes clusters.

---

## Deployment Options

| Option | Best For | Complexity |
|--------|----------|------------|
| Binary | Development, testing | Low |
| Docker | Single-node production | Low |
| Docker Compose | Local development with monitoring | Medium |
| Helm Chart | Production Kubernetes | Medium |
| Kubernetes (Kustomize) | Custom Kubernetes deployments | Medium |
| Kubernetes CRDs | Cloud-native, operator-managed | High |

---

## Binary Deployment

### Build

```bash
# Build all binaries for current platform
make build

# Or build individually
go build -o maia ./cmd/maia
go build -o maiactl ./cmd/maiactl
go build -o maia-mcp ./cmd/mcp-server
go build -o maia-migrate ./cmd/migrate

# Build for specific platform
GOOS=linux GOARCH=amd64 go build -o maia-linux-amd64 ./cmd/maia
GOOS=darwin GOARCH=arm64 go build -o maia-darwin-arm64 ./cmd/maia
```

### Run

```bash
# Development
./maia

# Production with config
./maia --config /etc/maia/config.yaml

# With environment variables
MAIA_HTTP_PORT=8080 \
MAIA_DATA_DIR=/var/lib/maia \
MAIA_API_KEY=your-key \
./maia
```

### Systemd Service

Create `/etc/systemd/system/maia.service`:

```ini
[Unit]
Description=MAIA Memory Server
After=network.target

[Service]
Type=simple
User=maia
Group=maia
ExecStart=/usr/local/bin/maia --config /etc/maia/config.yaml
Restart=always
RestartSec=5

# Security hardening
NoNewPrivileges=yes
ProtectSystem=strict
ProtectHome=yes
ReadWritePaths=/var/lib/maia

[Install]
WantedBy=multi-user.target
```

```bash
# Enable and start
sudo systemctl daemon-reload
sudo systemctl enable maia
sudo systemctl start maia

# Check status
sudo systemctl status maia
journalctl -u maia -f
```

---

## Docker Deployment

### Using Pre-built Image

```bash
# Pull and run
docker run -d \
  --name maia \
  -p 8080:8080 \
  -v maia-data:/data \
  -e MAIA_LOG_LEVEL=info \
  ghcr.io/ar4mirez/maia:latest

# With authentication
docker run -d \
  --name maia \
  -p 8080:8080 \
  -v maia-data:/data \
  -e MAIA_API_KEY=your-secure-key \
  -e MAIA_LOG_LEVEL=info \
  ghcr.io/ar4mirez/maia:latest
```

### Building Custom Image

```dockerfile
# Dockerfile
FROM golang:1.22-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -o maia ./cmd/maia

FROM alpine:3.19

RUN apk add --no-cache ca-certificates
COPY --from=builder /app/maia /usr/local/bin/

EXPOSE 8080
VOLUME /data

ENTRYPOINT ["maia"]
```

```bash
# Build
docker build -t maia:custom .

# Run
docker run -d -p 8080:8080 -v maia-data:/data maia:custom
```

### Docker Health Checks

```yaml
healthcheck:
  test: ["CMD", "wget", "-q", "--spider", "http://localhost:8080/health"]
  interval: 30s
  timeout: 10s
  retries: 3
  start_period: 10s
```

---

## Docker Compose

### Basic Setup

```yaml
# docker-compose.yaml
services:
  maia:
    image: ghcr.io/ar4mirez/maia:latest
    ports:
      - "8080:8080"
    volumes:
      - maia-data:/data
    environment:
      MAIA_LOG_LEVEL: info
      MAIA_DATA_DIR: /data
    healthcheck:
      test: ["CMD", "wget", "-q", "--spider", "http://localhost:8080/health"]
      interval: 30s
      timeout: 10s
      retries: 3
    restart: unless-stopped

volumes:
  maia-data:
```

### With Monitoring

```yaml
# docker-compose.yaml
services:
  maia:
    image: ghcr.io/ar4mirez/maia:latest
    ports:
      - "8080:8080"
    volumes:
      - maia-data:/data
    environment:
      MAIA_LOG_LEVEL: info
      MAIA_TRACING_ENABLED: "true"
      MAIA_TRACING_ENDPOINT: jaeger:4318
    depends_on:
      - jaeger
    restart: unless-stopped

  prometheus:
    image: prom/prometheus:latest
    ports:
      - "9090:9090"
    volumes:
      - ./prometheus.yaml:/etc/prometheus/prometheus.yml
    command:
      - '--config.file=/etc/prometheus/prometheus.yml'
    restart: unless-stopped

  grafana:
    image: grafana/grafana:latest
    ports:
      - "3000:3000"
    volumes:
      - grafana-data:/var/lib/grafana
    environment:
      GF_SECURITY_ADMIN_PASSWORD: admin
    restart: unless-stopped

  jaeger:
    image: jaegertracing/all-in-one:latest
    ports:
      - "16686:16686"  # UI
      - "4318:4318"    # OTLP HTTP
    restart: unless-stopped

volumes:
  maia-data:
  grafana-data:
```

### Prometheus Configuration

```yaml
# prometheus.yaml
global:
  scrape_interval: 15s

scrape_configs:
  - job_name: 'maia'
    static_configs:
      - targets: ['maia:8080']
    metrics_path: /metrics
```

### Start Services

```bash
# Basic deployment
docker-compose up -d

# With monitoring (Prometheus, Grafana, Jaeger)
docker-compose --profile monitoring up -d

# View logs
docker-compose logs -f maia

# Stop all services
docker-compose down
```

---

## Helm Chart Deployment

The recommended way to deploy MAIA on Kubernetes.

### Prerequisites

- Kubernetes 1.25+
- Helm 3.10+
- kubectl configured

### Quick Start

```bash
# Install from GitHub release
helm install maia https://github.com/ar4mirez/maia/releases/latest/download/maia-chart.tgz

# Or install from source
helm install maia ./deployments/helm/maia -n maia-system --create-namespace
```

### Custom Values

Create a custom `values.yaml`:

```yaml
# values.yaml
replicaCount: 2

image:
  repository: ghcr.io/ar4mirez/maia
  tag: "latest"

config:
  logLevel: info
  dataDir: /data

persistence:
  enabled: true
  size: 10Gi
  storageClass: standard

resources:
  limits:
    cpu: 1000m
    memory: 1Gi
  requests:
    cpu: 100m
    memory: 256Mi

ingress:
  enabled: true
  className: nginx
  hosts:
    - host: maia.example.com
      paths:
        - path: /
          pathType: Prefix
  tls:
    - secretName: maia-tls
      hosts:
        - maia.example.com

autoscaling:
  enabled: true
  minReplicas: 2
  maxReplicas: 10
  targetCPUUtilizationPercentage: 70

backup:
  enabled: true
  schedule: "0 2 * * *"
  retention: 7

serviceMonitor:
  enabled: true
```

Install with custom values:

```bash
helm install maia ./deployments/helm/maia -f values.yaml -n maia-system --create-namespace
```

### Upgrade

```bash
helm upgrade maia ./deployments/helm/maia -f values.yaml -n maia-system
```

### Uninstall

```bash
helm uninstall maia -n maia-system
```

---

## Kubernetes CRDs

MAIA provides Custom Resource Definitions for cloud-native deployments.

### Install CRDs

```bash
# Install all CRDs
kubectl apply -k deployments/kubernetes/crds/

# Or install individually
kubectl apply -f deployments/kubernetes/crds/maia.cuemby.com_maiainstances.yaml
kubectl apply -f deployments/kubernetes/crds/maia.cuemby.com_maiatenants.yaml
```

### MaiaInstance Resource

Create a MAIA instance:

```yaml
# basic-instance.yaml
apiVersion: maia.cuemby.com/v1alpha1
kind: MaiaInstance
metadata:
  name: maia-production
  namespace: maia-system
spec:
  replicas: 2
  image:
    repository: ghcr.io/ar4mirez/maia
    tag: latest
  config:
    logLevel: info
    httpPort: 8080
    dataDir: /data
  storage:
    size: 50Gi
    storageClass: fast-ssd
  resources:
    limits:
      cpu: "2"
      memory: 4Gi
    requests:
      cpu: 500m
      memory: 1Gi
```

```bash
kubectl apply -f basic-instance.yaml
```

### MaiaTenant Resource

Create tenants:

```yaml
# tenant.yaml
apiVersion: maia.cuemby.com/v1alpha1
kind: MaiaTenant
metadata:
  name: acme-corp
  namespace: maia-system
spec:
  instanceRef:
    name: maia-production
  displayName: "Acme Corporation"
  plan: enterprise
  quotas:
    maxMemories: 1000000
    maxStorageBytes: 107374182400  # 100GB
    maxNamespaces: 100
    requestsPerMinute: 1000
    requestsPerDay: 100000
  config:
    defaultTokenBudget: 8000
    maxTokenBudget: 32000
  apiKeys:
    - name: admin-key
      secretRef:
        name: acme-corp-api-key
        key: api-key
      scopes: ["*"]
```

```bash
kubectl apply -f tenant.yaml
```

### Examples

More examples available in `deployments/kubernetes/examples/`:

```bash
# Basic instance
kubectl apply -f deployments/kubernetes/examples/basic-instance.yaml

# Production instance with HA
kubectl apply -f deployments/kubernetes/examples/production-instance.yaml

# Multi-tenant setup
kubectl apply -f deployments/kubernetes/examples/tenant-example.yaml
```

---

## Kubernetes Deployment (Kustomize)

### Kustomize Prerequisites

- Kubernetes 1.25+
- kubectl configured
- Storage class for persistent volumes

### Basic Kustomize Deployment

```yaml
# deployment.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: maia
  labels:
    app: maia
spec:
  replicas: 1
  selector:
    matchLabels:
      app: maia
  template:
    metadata:
      labels:
        app: maia
    spec:
      containers:
        - name: maia
          image: ghcr.io/ar4mirez/maia:latest
          ports:
            - containerPort: 8080
              name: http
          env:
            - name: MAIA_DATA_DIR
              value: /data
            - name: MAIA_LOG_LEVEL
              value: info
          volumeMounts:
            - name: data
              mountPath: /data
          resources:
            requests:
              cpu: 100m
              memory: 256Mi
            limits:
              cpu: 1000m
              memory: 1Gi
          livenessProbe:
            httpGet:
              path: /health
              port: http
            initialDelaySeconds: 10
            periodSeconds: 30
          readinessProbe:
            httpGet:
              path: /ready
              port: http
            initialDelaySeconds: 5
            periodSeconds: 10
      volumes:
        - name: data
          persistentVolumeClaim:
            claimName: maia-data

---
apiVersion: v1
kind: Service
metadata:
  name: maia
spec:
  selector:
    app: maia
  ports:
    - port: 80
      targetPort: http
  type: ClusterIP

---
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: maia-data
spec:
  accessModes:
    - ReadWriteOnce
  resources:
    requests:
      storage: 10Gi
```

### With ConfigMap and Secret

```yaml
# configmap.yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: maia-config
data:
  config.yaml: |
    server:
      http_port: 8080
      max_concurrent_requests: 200
    storage:
      data_dir: /data
    memory:
      default_token_budget: 4000
    logging:
      level: info
      format: json

---
# secret.yaml
apiVersion: v1
kind: Secret
metadata:
  name: maia-secrets
type: Opaque
stringData:
  api-key: "your-secure-api-key"
  encryption-key: "32-byte-encryption-key-here!!"
```

Update deployment to use ConfigMap and Secret:

```yaml
spec:
  containers:
    - name: maia
      env:
        - name: MAIA_API_KEY
          valueFrom:
            secretKeyRef:
              name: maia-secrets
              key: api-key
        - name: MAIA_ENCRYPTION_KEY
          valueFrom:
            secretKeyRef:
              name: maia-secrets
              key: encryption-key
      volumeMounts:
        - name: config
          mountPath: /etc/maia
        - name: data
          mountPath: /data
      args:
        - --config
        - /etc/maia/config.yaml
  volumes:
    - name: config
      configMap:
        name: maia-config
    - name: data
      persistentVolumeClaim:
        claimName: maia-data
```

### Ingress with TLS

```yaml
# ingress.yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: maia
  annotations:
    cert-manager.io/cluster-issuer: letsencrypt-prod
    nginx.ingress.kubernetes.io/proxy-body-size: 10m
spec:
  ingressClassName: nginx
  tls:
    - hosts:
        - maia.example.com
      secretName: maia-tls
  rules:
    - host: maia.example.com
      http:
        paths:
          - path: /
            pathType: Prefix
            backend:
              service:
                name: maia
                port:
                  number: 80
```

### Kustomization

```yaml
# kustomization.yaml
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization

namespace: maia

resources:
  - deployment.yaml
  - service.yaml
  - pvc.yaml
  - configmap.yaml
  - ingress.yaml

secretGenerator:
  - name: maia-secrets
    literals:
      - api-key=your-secure-api-key
      - encryption-key=32-byte-encryption-key-here!!

configMapGenerator:
  - name: maia-config
    files:
      - config.yaml

images:
  - name: ghcr.io/ar4mirez/maia
    newTag: v1.0.0
```

Deploy:

```bash
kubectl apply -k ./deployments/kubernetes/
```

### Horizontal Pod Autoscaler

```yaml
# hpa.yaml
apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
metadata:
  name: maia
spec:
  scaleTargetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: maia
  minReplicas: 2
  maxReplicas: 10
  metrics:
    - type: Resource
      resource:
        name: cpu
        target:
          type: Utilization
          averageUtilization: 70
    - type: Resource
      resource:
        name: memory
        target:
          type: Utilization
          averageUtilization: 80
```

> **Note**: MAIA uses BadgerDB which requires single-writer access. For HPA, consider using a shared storage solution or running in read-only replica mode.

### Service Monitor (Prometheus Operator)

```yaml
# servicemonitor.yaml
apiVersion: monitoring.coreos.com/v1
kind: ServiceMonitor
metadata:
  name: maia
  labels:
    release: prometheus
spec:
  selector:
    matchLabels:
      app: maia
  endpoints:
    - port: http
      path: /metrics
      interval: 30s
```

---

## Production Checklist

### Security

- [ ] Enable TLS/HTTPS
- [ ] Configure API key authentication
- [ ] Set up namespace-level authorization
- [ ] Enable encryption at rest
- [ ] Configure rate limiting
- [ ] Review CORS settings
- [ ] Restrict network access

### Reliability

- [ ] Configure health checks
- [ ] Set up readiness probes
- [ ] Configure resource limits
- [ ] Set up persistent storage
- [ ] Configure backup strategy
- [ ] Test disaster recovery

### Observability

- [ ] Enable structured logging (JSON)
- [ ] Configure Prometheus metrics
- [ ] Set up OpenTelemetry tracing
- [ ] Create Grafana dashboards
- [ ] Configure alerting rules
- [ ] Set up log aggregation

### Performance

- [ ] Tune resource limits based on load testing
- [ ] Configure connection pooling
- [ ] Optimize token budgets
- [ ] Consider caching strategy
- [ ] Monitor latency percentiles

---

## Backup and Recovery

MAIA includes automated backup and restore scripts in the `scripts/` directory.

### Using Backup Scripts

```bash
# Create a backup
./scripts/backup.sh ./data ./backups

# Create an encrypted backup (will prompt for password)
./scripts/backup.sh ./data ./backups --encrypt

# Using Makefile
make backup
make backup-encrypted
```

The backup script:

- Creates timestamped compressed archives
- Validates backup integrity with checksums
- Supports optional AES-256 encryption
- Maintains backup metadata

### Using Restore Scripts

```bash
# Restore from backup (will stop MAIA, restore, and restart)
./scripts/restore.sh ./backups/maia-backup-20260120-120000.tar.gz ./data

# Using Makefile
make restore BACKUP_FILE=./backups/maia-backup-20260120-120000.tar.gz
```

The restore script:

- Validates backup integrity before restoring
- Creates a backup of current data before overwriting
- Handles encrypted backups automatically
- Verifies restore success

### Manual Backup Strategy

```bash
#!/bin/bash
# Custom backup script

DATE=$(date +%Y%m%d-%H%M%S)
BACKUP_DIR=/backups/maia

# Stop writes (optional, for consistency)
# curl -X POST http://localhost:8080/admin/maintenance

# Backup BadgerDB directory
tar -czf "$BACKUP_DIR/maia-data-$DATE.tar.gz" /var/lib/maia/

# Backup to S3
aws s3 cp "$BACKUP_DIR/maia-data-$DATE.tar.gz" s3://backups/maia/

# Resume writes
# curl -X DELETE http://localhost:8080/admin/maintenance
```

### Kubernetes Backup (Velero)

```yaml
# backup-schedule.yaml
apiVersion: velero.io/v1
kind: Schedule
metadata:
  name: maia-daily
spec:
  schedule: "0 2 * * *"
  template:
    includedNamespaces:
      - maia
    storageLocation: default
    volumeSnapshotLocations:
      - default
```

### Helm Chart Backup CronJob

When using the Helm chart, enable automated backups:

```yaml
# values.yaml
backup:
  enabled: true
  schedule: "0 2 * * *"  # Daily at 2 AM
  retention: 7           # Keep 7 days of backups
  storage:
    type: pvc           # pvc, s3, gcs
    pvc:
      size: 50Gi
```

---

## Database Migrations

The `maia-migrate` tool handles database schema migrations.

### Running Migrations

```bash
# Run all pending migrations
maia-migrate up

# Rollback last migration
maia-migrate down

# Check migration status
maia-migrate status

# Migrate to a specific version
maia-migrate goto 5

# Force set version (use with caution)
maia-migrate force 3
```

### Migration Configuration

```bash
# Using environment variables
export MAIA_DATA_DIR=/var/lib/maia
maia-migrate up

# Using config file
maia-migrate --config /etc/maia/config.yaml up
```

### Kubernetes Migration Job

For Kubernetes deployments, run migrations as a Job:

```yaml
apiVersion: batch/v1
kind: Job
metadata:
  name: maia-migrate
spec:
  template:
    spec:
      containers:
        - name: migrate
          image: ghcr.io/ar4mirez/maia:latest
          command: ["maia-migrate", "up"]
          env:
            - name: MAIA_DATA_DIR
              value: /data
          volumeMounts:
            - name: data
              mountPath: /data
      restartPolicy: Never
      volumes:
        - name: data
          persistentVolumeClaim:
            claimName: maia-data
```

---

## Troubleshooting

### Server Won't Start

```bash
# Check logs
journalctl -u maia -n 100

# Verify config
maia --config /etc/maia/config.yaml --validate

# Check data directory permissions
ls -la /var/lib/maia
```

### High Memory Usage

```bash
# Check BadgerDB memory usage
curl http://localhost:8080/v1/stats | jq '.storage'

# Trigger compaction
curl -X POST http://localhost:8080/admin/compact
```

### Slow Queries

```bash
# Enable debug logging
export MAIA_LOG_LEVEL=debug

# Check metrics
curl http://localhost:8080/metrics | grep maia_

# Profile (if enabled)
curl http://localhost:6060/debug/pprof/profile > profile.out
```

### Connection Issues

```bash
# Check if server is running
curl http://localhost:8080/health

# Check network connectivity
nc -zv localhost 8080

# Check TLS certificate
openssl s_client -connect maia.example.com:443
```

---

## Related Documentation

- [Configuration](configuration.md) - All configuration options
- [Architecture](architecture.md) - System design
- [Multi-Tenancy](multi-tenancy.md) - Tenant management
