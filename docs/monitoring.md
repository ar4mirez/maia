# MAIA Monitoring & Observability

MAIA provides comprehensive monitoring through Prometheus metrics, OpenTelemetry tracing, and Grafana dashboards.

---

## Overview

MAIA exposes:

- **Prometheus metrics** — HTTP, memory operations, search, context assembly
- **OpenTelemetry tracing** — Distributed request tracing
- **Grafana dashboards** — Pre-built visualization dashboards
- **Alerting rules** — Prometheus alerts for quotas, errors, latency

---

## Prometheus Metrics

### Endpoint

Metrics are available at `/metrics`:

```bash
curl http://localhost:8080/metrics
```

### Available Metrics

#### HTTP Metrics

| Metric | Type | Description |
|--------|------|-------------|
| `maia_http_requests_total` | Counter | Total HTTP requests by method, path, status |
| `maia_http_request_duration_seconds` | Histogram | Request latency distribution |
| `maia_http_requests_in_flight` | Gauge | Currently processing requests |

#### Memory Metrics

| Metric | Type | Description |
|--------|------|-------------|
| `maia_memory_operations_total` | Counter | Memory operations by type (create, read, update, delete) |
| `maia_memory_operation_duration_seconds` | Histogram | Memory operation latency |

#### Search Metrics

| Metric | Type | Description |
|--------|------|-------------|
| `maia_search_operations_total` | Counter | Search operations count |
| `maia_search_operation_duration_seconds` | Histogram | Search operation latency |
| `maia_search_results_total` | Histogram | Results per search |

#### Context Metrics

| Metric | Type | Description |
|--------|------|-------------|
| `maia_context_assembly_total` | Counter | Context assembly operations |
| `maia_context_assembly_duration_seconds` | Histogram | Context assembly latency |
| `maia_context_tokens_total` | Histogram | Tokens used per context |

#### Embedding Metrics

| Metric | Type | Description |
|--------|------|-------------|
| `maia_embedding_operations_total` | Counter | Embedding generation count |
| `maia_embedding_operation_duration_seconds` | Histogram | Embedding latency |

#### Storage Metrics

| Metric | Type | Description |
|--------|------|-------------|
| `maia_storage_size_bytes` | Gauge | Total storage size |
| `maia_storage_memories_total` | Gauge | Total memory count |
| `maia_storage_namespaces_total` | Gauge | Total namespace count |

#### Tenant Metrics

| Metric | Type | Description |
|--------|------|-------------|
| `maia_tenant_memories_total` | Gauge | Memories per tenant |
| `maia_tenant_storage_bytes` | Gauge | Storage per tenant |
| `maia_tenant_requests_total` | Counter | Requests per tenant |
| `maia_tenant_quota_usage_ratio` | Gauge | Quota usage (0.0-1.0) by tenant and resource |
| `maia_tenants_active_total` | Gauge | Active tenant count |
| `maia_tenant_operations_total` | Counter | Tenant management operations |

#### Rate Limiting Metrics

| Metric | Type | Description |
|--------|------|-------------|
| `maia_rate_limit_rejections_total` | Counter | Rate limit rejections |
| `maia_rate_limit_tokens_available` | Gauge | Available rate limit tokens |

---

## Prometheus Configuration

### Scrape Config

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

### Kubernetes ServiceMonitor

For Prometheus Operator:

```yaml
apiVersion: monitoring.coreos.com/v1
kind: ServiceMonitor
metadata:
  name: maia
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

## OpenTelemetry Tracing

### Configuration

Enable tracing in `config.yaml`:

```yaml
tracing:
  enabled: true
  service_name: maia
  environment: production
  exporter_type: otlp-http    # otlp-http, otlp-grpc, noop
  endpoint: jaeger:4318
  sample_rate: 1.0            # 0.0 to 1.0
  insecure: false
```

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `MAIA_TRACING_ENABLED` | `false` | Enable tracing |
| `MAIA_TRACING_SERVICE_NAME` | `maia` | Service name in traces |
| `MAIA_TRACING_ENVIRONMENT` | `development` | Environment tag |
| `MAIA_TRACING_EXPORTER_TYPE` | `otlp-http` | Exporter type |
| `MAIA_TRACING_ENDPOINT` | `localhost:4318` | Collector endpoint |
| `MAIA_TRACING_SAMPLE_RATE` | `1.0` | Sampling rate |
| `MAIA_TRACING_INSECURE` | `true` | Skip TLS verification |

### Span Attributes

MAIA traces include:

| Attribute | Description |
|-----------|-------------|
| `maia.request_id` | Unique request ID |
| `maia.namespace` | Target namespace |
| `maia.tenant_id` | Tenant identifier |
| `maia.memory_type` | Memory type (semantic, episodic, working) |
| `maia.token_count` | Token count for context |

---

## Grafana Dashboards

MAIA includes pre-built Grafana dashboards.

### Dashboard Files

Located in `deployments/grafana/dashboards/`:

- `maia-overview.json` — System overview
- `maia-tenant-metrics.json` — Per-tenant metrics

### Auto-Provisioning

For Docker Compose:

```yaml
services:
  grafana:
    image: grafana/grafana:latest
    volumes:
      - ./deployments/grafana/dashboards:/var/lib/grafana/dashboards
      - ./deployments/grafana/provisioning:/etc/grafana/provisioning
```

### System Overview Dashboard

Includes:

- Request rate and error rate
- P50, P90, P99 latency percentiles
- HTTP traffic by method and status
- Memory operations by type
- Search and context operation latency
- Storage metrics

### Tenant Metrics Dashboard

Includes:

- Active tenants overview
- Per-tenant memory counts
- Per-tenant storage usage
- Per-tenant request rates
- Quota usage gauges (with thresholds)
- Tenant operations tracking

---

## Alerting Rules

Pre-configured Prometheus alerts in `deployments/prometheus/alerts.yaml`:

### Quota Alerts

| Alert | Severity | Threshold |
|-------|----------|-----------|
| `MAIATenantMemoryQuotaWarning` | warning | 70% |
| `MAIATenantMemoryQuotaCritical` | critical | 85% |
| `MAIATenantMemoryQuotaExhausted` | critical | 95% |
| `MAIATenantStorageQuotaWarning` | warning | 70% |
| `MAIATenantStorageQuotaCritical` | critical | 85% |
| `MAIATenantRPMQuotaWarning` | warning | 70% |

### Error Alerts

| Alert | Severity | Description |
|-------|----------|-------------|
| `MAIAHighErrorRate` | warning | Error rate > 5% |
| `MAIACriticalErrorRate` | critical | Error rate > 10% |
| `MAIATenantHighErrorRate` | warning | Per-tenant error rate > 10% |

### Latency Alerts

| Alert | Severity | Threshold |
|-------|----------|-----------|
| `MAIAHighP99Latency` | warning | P99 > 500ms |
| `MAIAMemoryOperationSlow` | warning | P99 > 100ms |
| `MAIASearchOperationSlow` | warning | P99 > 200ms |
| `MAIAContextAssemblySlow` | warning | P99 > 300ms |
| `MAIACriticalLatency` | critical | P99 > 1s |

### System Health Alerts

| Alert | Severity | Description |
|-------|----------|-------------|
| `MAIANoRequests` | warning | No requests in 5 minutes |
| `MAIAHighInflightRequests` | warning | > 100 concurrent requests |
| `MAIAStorageGrowthHigh` | warning | > 100MB growth in 1 hour |
| `MAIATenantSuspended` | info | Tenant suspended |
| `MAIATenantPendingDeletion` | info | Tenant pending deletion |
| `MAIAMultipleTenantsSuspended` | warning | > 2 tenants suspended |

### Rate Limit Alerts

| Alert | Severity | Description |
|-------|----------|-------------|
| `MAIATenantRateLimited` | warning | Rate limit rejections |
| `MAIAGlobalRateLimiting` | critical | High global rejections |

### Auth Alerts

| Alert | Severity | Description |
|-------|----------|-------------|
| `MAIAAuthFailures` | warning | > 10 failures in 5 min |
| `MAIAScopeDenied` | warning | > 5 scope denials in 5 min |

### Loading Alerts

Add to Prometheus:

```yaml
# prometheus.yaml
rule_files:
  - /etc/prometheus/alerts.yaml

alerting:
  alertmanagers:
    - static_configs:
        - targets: ['alertmanager:9093']
```

---

## Example Prometheus Queries

### Request Metrics

```promql
# Request rate
rate(maia_http_requests_total[5m])

# Error rate
sum(rate(maia_http_requests_total{status=~"5.."}[5m])) /
sum(rate(maia_http_requests_total[5m])) * 100

# P99 latency
histogram_quantile(0.99, rate(maia_http_request_duration_seconds_bucket[5m]))
```

### Memory Operations

```promql
# Memory creates per minute
rate(maia_memory_operations_total{operation="create"}[1m]) * 60

# Average memory operation latency
rate(maia_memory_operation_duration_seconds_sum[5m]) /
rate(maia_memory_operation_duration_seconds_count[5m])
```

### Tenant Metrics

```promql
# Memory count per tenant
maia_tenant_memories_total

# Quota usage by tenant
maia_tenant_quota_usage_ratio{resource="memories"}

# Request rate per tenant
rate(maia_tenant_requests_total[5m])
```

### Search and Context

```promql
# Search latency P99
histogram_quantile(0.99, rate(maia_search_operation_duration_seconds_bucket[5m]))

# Context assembly latency
histogram_quantile(0.95, rate(maia_context_assembly_duration_seconds_bucket[5m]))

# Average tokens per context
rate(maia_context_tokens_total_sum[5m]) /
rate(maia_context_tokens_total_count[5m])
```

---

## Docker Compose with Monitoring

```yaml
version: '3.8'

services:
  maia:
    image: ghcr.io/ar4mirez/maia:latest
    ports:
      - "8080:8080"
    volumes:
      - maia-data:/data
    environment:
      MAIA_TRACING_ENABLED: "true"
      MAIA_TRACING_ENDPOINT: jaeger:4318

  prometheus:
    image: prom/prometheus:latest
    ports:
      - "9090:9090"
    volumes:
      - ./deployments/prometheus.yaml:/etc/prometheus/prometheus.yml
      - ./deployments/prometheus/alerts.yaml:/etc/prometheus/alerts.yaml

  grafana:
    image: grafana/grafana:latest
    ports:
      - "3000:3000"
    volumes:
      - ./deployments/grafana/dashboards:/var/lib/grafana/dashboards
      - ./deployments/grafana/provisioning:/etc/grafana/provisioning
    environment:
      GF_SECURITY_ADMIN_PASSWORD: admin

  jaeger:
    image: jaegertracing/all-in-one:latest
    ports:
      - "16686:16686"  # UI
      - "4318:4318"    # OTLP HTTP

volumes:
  maia-data:
```

Start with:

```bash
docker-compose up -d
```

Access:
- MAIA: http://localhost:8080
- Prometheus: http://localhost:9090
- Grafana: http://localhost:3000
- Jaeger: http://localhost:16686

---

## Health Endpoints

| Endpoint | Description |
|----------|-------------|
| `GET /health` | Basic health check |
| `GET /ready` | Readiness check (includes storage) |
| `GET /metrics` | Prometheus metrics |

### Health Response

```json
{
  "status": "healthy",
  "service": "maia"
}
```

### Ready Response

```json
{
  "status": "ready",
  "checks": {
    "storage": "ok"
  }
}
```

---

## Related Documentation

- [Configuration](configuration.md) - Full configuration reference
- [Deployment](deployment.md) - Production deployment
- [Multi-Tenancy](multi-tenancy.md) - Tenant metrics
