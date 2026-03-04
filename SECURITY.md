# Security Policy

## Supported Versions

| Version | Supported |
|---------|-----------|
| 0.1.x (main) | ✅ |

llmwarden is currently in early development (v0.1.x / v1alpha1 APIs). Security fixes are applied to the `main` branch only.

## Reporting a Vulnerability

**Please do not report security vulnerabilities through public GitHub issues.**

Report vulnerabilities privately via GitHub's [Security Advisories](https://github.com/llmwarden/llmwarden/security/advisories/new) feature. This keeps the report confidential until a fix is ready.

Alternatively, email **security@llmwarden.io** with:

1. A description of the vulnerability and its impact
2. Steps to reproduce (ideally a minimal test case)
3. The version(s) affected
4. Any suggested mitigations you are aware of

**Expected response timeline:**
- Acknowledgement within **2 business days**
- Initial assessment within **5 business days**
- Fix or mitigation plan within **30 days** for critical/high severity
- Fix or mitigation plan within **90 days** for medium/low severity

## Scope

The following are **in scope**:

- The llmwarden controller binary and container image
- Kubernetes CRD admission webhooks (validating and mutating)
- Credential provisioning logic (ApiKeyProvisioner, ExternalSecretProvisioner)
- RBAC permissions granted to the operator ServiceAccount
- Helm chart configuration that leads to privilege escalation or insecure defaults
- Credential leakage in logs, events, or Kubernetes object metadata

The following are **out of scope**:

- Vulnerabilities in Kubernetes itself, External Secrets Operator, or other dependencies (report upstream)
- Vulnerabilities requiring physical access or access to the Kubernetes control plane
- Issues in `docs/` or `examples/` that have no runtime impact

## Security Architecture

### Privilege Model

The llmwarden operator requires cluster-wide `Secrets` access (create, get, list, watch, update, patch, delete). This is inherent to the architecture: the operator copies credential material from a provider namespace (e.g., `llmwarden-system`) into workload namespaces. Without cross-namespace secret access, this delegation pattern cannot work.

**Recommendations for production deployments:**

1. **Dedicated namespace**: Deploy the operator in an isolated namespace (e.g., `llmwarden-system`) with restricted access.
2. **Audit cluster-scoped bindings**: Use `kubectl get clusterrolebindings` to verify no unexpected subjects are bound to the `llmwarden-manager` ClusterRole.
3. **Restrict LLMProvider to trusted namespaces**: Use `spec.namespaceSelector` on every LLMProvider to limit which namespaces can consume credentials.
4. **Enable Kubernetes audit logging**: The operator emits Kubernetes events for all credential operations; enable audit logging to capture API server-level access.
5. **Enable secret encryption at rest**: llmwarden stores API keys as Kubernetes Secrets; ensure your cluster has encryption at rest enabled for the `secrets` resource type.

### Secrets Handling

- API keys are never logged or exposed in Kubernetes events
- Secret values are handled only in memory during provisioning
- Owner references ensure secrets are garbage-collected when LLMAccess is deleted
- Volume mounts for injected credentials are enforced as read-only with 0400 permissions

### Webhook Security

- Admission webhooks use TLS (managed by cert-manager in production)
- The validating webhook for LLMAccess uses `failurePolicy: Fail` (fail-closed)
- The mutating pod injector uses `failurePolicy: Ignore` (fail-open for availability)
- Minimum TLS 1.2 is enforced; HTTP/2 is disabled by default (CVE mitigation)

## Known Limitations

- **No automatic key revocation**: Rotation creates new keys but does not revoke old ones (planned for Phase 4)
- **No workload identity support yet**: AWS IRSA, Azure WI, GCP WIF provisioners are in Phase 3 (not yet implemented)
- **No SIEM integration**: Audit trail is Kubernetes events only

See [architecture.md](docs/architecture.md#security-architecture) for the full threat model and security controls.
