# MAIA Audit Logging

MAIA provides comprehensive audit logging for compliance, security monitoring, and debugging.

---

## Overview

The audit logging system captures:

- **All memory operations** — Create, read, update, delete, search
- **Namespace management** — Create, update, delete, list
- **Tenant operations** — Create, update, delete, suspend, resume
- **Authentication events** — Success, failure, scope denied
- **Context assembly** — Query and result tracking
- **System events** — Startup, shutdown, backup, restore

---

## Enabling Audit Logging

Configure audit logging in your `config.yaml`:

```yaml
audit:
  enabled: true
  level: write           # all, write, admin
  backend:
    type: file
    file_path: ./logs/audit.log
  redact_fields:
    - password
    - api_key
    - secret
    - token
```

---

## Audit Levels

Control which events are logged:

| Level | Events Captured |
|-------|-----------------|
| `all` | All operations including reads |
| `write` | All writes, deletes, and admin (default) |
| `admin` | Only admin and authentication events |

### Level Examples

```yaml
# Development: Log everything
audit:
  enabled: true
  level: all

# Production: Writes and admin only
audit:
  enabled: true
  level: write

# High-security: Admin actions only
audit:
  enabled: true
  level: admin
```

---

## Event Types

### Memory Events

| Event | Description | Level |
|-------|-------------|-------|
| `memory.create` | Memory created | write |
| `memory.read` | Memory retrieved | all |
| `memory.update` | Memory updated | write |
| `memory.delete` | Memory deleted | write |
| `memory.search` | Search performed | all |

### Namespace Events

| Event | Description | Level |
|-------|-------------|-------|
| `namespace.create` | Namespace created | write |
| `namespace.read` | Namespace retrieved | all |
| `namespace.update` | Namespace updated | write |
| `namespace.delete` | Namespace deleted | write |
| `namespace.list` | Namespaces listed | all |

### Tenant Events

| Event | Description | Level |
|-------|-------------|-------|
| `tenant.create` | Tenant created | admin |
| `tenant.update` | Tenant updated | admin |
| `tenant.delete` | Tenant deleted | admin |
| `tenant.suspend` | Tenant suspended | admin |
| `tenant.resume` | Tenant resumed | admin |

### API Key Events

| Event | Description | Level |
|-------|-------------|-------|
| `apikey.create` | API key created | admin |
| `apikey.revoke` | API key revoked | admin |
| `apikey.rotate` | API key rotated | admin |

### Context Events

| Event | Description | Level |
|-------|-------------|-------|
| `context.assemble` | Context assembled | all |

### Authentication Events

| Event | Description | Level |
|-------|-------------|-------|
| `auth.success` | Authentication succeeded | write |
| `auth.failure` | Authentication failed | write |
| `auth.scope_denied` | Scope check failed | write |

### System Events

| Event | Description | Level |
|-------|-------------|-------|
| `system.startup` | Server started | admin |
| `system.shutdown` | Server stopped | admin |
| `system.backup` | Backup created | admin |
| `system.restore` | Restore completed | admin |

---

## Audit Log Format

Each audit entry contains:

```json
{
  "timestamp": "2026-01-20T10:30:00.000Z",
  "event_type": "memory.create",
  "actor": {
    "type": "api_key",
    "id": "maia_abc123...",
    "tenant_id": "acme-corp"
  },
  "resource": {
    "type": "memory",
    "id": "mem_xyz789",
    "namespace": "default"
  },
  "action": "create",
  "outcome": "success",
  "details": {
    "memory_type": "semantic",
    "content_length": 128,
    "tags": ["preference", "ui"]
  },
  "request": {
    "method": "POST",
    "path": "/v1/memories",
    "ip": "192.168.1.100",
    "user_agent": "maia-sdk/1.0"
  },
  "duration_ms": 12
}
```

### Fields

| Field | Description |
|-------|-------------|
| `timestamp` | ISO 8601 timestamp |
| `event_type` | Event category and action |
| `actor` | Who performed the action |
| `resource` | What was affected |
| `action` | The action performed |
| `outcome` | success, failure, denied |
| `details` | Event-specific details |
| `request` | HTTP request metadata |
| `duration_ms` | Operation duration |

---

## Actor Types

| Type | Description |
|------|-------------|
| `api_key` | Authenticated via API key |
| `tenant` | Identified by tenant header |
| `system` | Internal system operation |
| `anonymous` | No authentication |

---

## Sensitive Field Redaction

Configure fields to redact from logs:

```yaml
audit:
  redact_fields:
    - password
    - api_key
    - secret
    - token
    - authorization
    - x-api-key
```

Redacted fields appear as `[REDACTED]` in logs:

```json
{
  "details": {
    "api_key": "[REDACTED]",
    "name": "production-key"
  }
}
```

---

## Backend Configuration

### File Backend

Write to a local file with rotation:

```yaml
audit:
  backend:
    type: file
    file_path: /var/log/maia/audit.log
    max_size: 100         # Max file size in MB
    max_backups: 5        # Number of backup files
    max_age: 30           # Max age in days
    compress: true        # Compress rotated files
```

### Stdout Backend

Write to standard output (for container deployments):

```yaml
audit:
  backend:
    type: stdout
```

---

## Querying Audit Logs

The audit system supports log queries:

```go
// Query last 24 hours of tenant events
events, err := auditor.Query(ctx, &audit.QueryOptions{
    EventTypes: []string{"tenant.*"},
    StartTime:  time.Now().Add(-24 * time.Hour),
    EndTime:    time.Now(),
    Limit:      100,
})
```

### File-based Queries

For file-based logs, use standard tools:

```bash
# Find all failed authentications
grep '"auth.failure"' /var/log/maia/audit.log

# Find all tenant deletions
grep '"tenant.delete"' /var/log/maia/audit.log | jq .

# Find events for specific tenant
grep '"tenant_id":"acme-corp"' /var/log/maia/audit.log | jq .

# Events in last hour
grep "$(date -u -d '-1 hour' +%Y-%m-%dT%H)" /var/log/maia/audit.log
```

---

## Integration with SIEM

### Splunk

Forward logs to Splunk:

```bash
# Forward audit logs
tail -F /var/log/maia/audit.log | \
  curl -k "https://splunk:8088/services/collector/event" \
  -H "Authorization: Splunk $SPLUNK_TOKEN" \
  -d @-
```

### Elasticsearch

Index logs in Elasticsearch:

```bash
# Using Filebeat
filebeat:
  inputs:
    - type: log
      paths:
        - /var/log/maia/audit.log
      json.keys_under_root: true
```

### Datadog

```yaml
# datadog.yaml
logs:
  - type: file
    path: /var/log/maia/audit.log
    service: maia
    source: maia-audit
```

---

## Compliance Considerations

### GDPR

Audit logs may contain personal data. Consider:

- Setting appropriate `max_age` for data retention
- Implementing log deletion procedures
- Encrypting audit log storage
- Redacting personally identifiable information

### SOC 2

Audit logging helps meet SOC 2 requirements:

- **CC6.1**: Logical access security
- **CC6.2**: User authentication
- **CC6.3**: Authorization control
- **CC7.1**: Detection of changes
- **CC7.2**: Incident response

### HIPAA

For healthcare applications:

- Enable full audit logging (`level: all`)
- Configure appropriate retention (`max_age: 365`)
- Ensure encrypted storage
- Implement access controls for logs

---

## Performance Considerations

### Batching

The audit logger batches writes for performance:

```yaml
audit:
  batch_size: 100       # Entries per batch
  batch_interval: 1s    # Max time between flushes
```

### Async Writing

Audit logging is asynchronous and non-blocking. Failed writes are:

1. Logged to error log
2. Retried with backoff
3. Dropped after max retries (configurable)

### Storage Impact

Estimate storage requirements:

| Events/Day | Avg Entry Size | Daily Storage |
|------------|----------------|---------------|
| 10,000 | 500 bytes | ~5 MB |
| 100,000 | 500 bytes | ~50 MB |
| 1,000,000 | 500 bytes | ~500 MB |

---

## Troubleshooting

### No Logs Generated

1. Check `audit.enabled: true`
2. Verify log directory is writable
3. Check audit level includes your events
4. Review MAIA logs for audit errors

### Missing Events

1. Ensure event type matches audit level
2. Check for dropped events in error logs
3. Verify log rotation isn't removing recent logs

### Performance Impact

If audit logging impacts performance:

1. Increase `batch_size`
2. Reduce audit `level` to `write` or `admin`
3. Use faster storage for log files
4. Consider stdout backend with external aggregation

---

## Related Documentation

- [Configuration](configuration.md) - Full configuration reference
- [Multi-Tenancy](multi-tenancy.md) - Tenant management
- [Deployment](deployment.md) - Production deployment
