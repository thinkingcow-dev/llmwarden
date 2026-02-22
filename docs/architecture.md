# llmwarden Architecture

## Problem Statement

Enterprise Kubernetes clusters have fragmented LLM provider access:

| Team | How they access LLMs | What's wrong |
|------|---------------------|--------------|
| Team A | Hardcoded `sk-*` in env var | Static secret, no rotation, leaked in pod spec |
| Team B | ESO pulls from Vault | Key in Vault was put there manually 6 months ago |
| Team C | IRSA for Bedrock | Over-scoped to `bedrock:*`, took 3 weeks to set up |
| Team D | Azure Workload Identity | Correct but unique snowflake config, not reproducible |
| Team E | ConfigMap with API key | Plain text, no rotation, visible to anyone with namespace read |

No tool provides a unified, declarative way to say: **"this workload needs access to GPT-4o"** and have the credential lifecycle handled automatically.

## Design Philosophy

1. **Declarative over imperative** — Define desired state, operator converges
2. **Delegate, don't reinvent** — ESO for secrets, cloud-native for identity, Kyverno for policy
3. **Provider-aware, not provider-locked** — Know LLM-specific patterns, support all providers
4. **Secure by default, easy by design** — The right way should be the easiest way
5. **Observable** — Every credential action is a K8s event, every state is a status condition

## CRD Specifications

### LLMProvider

Cluster-scoped resource. Platform team declares an available LLM provider.

```yaml
apiVersion: llmwarden.io/v1alpha1
kind: LLMProvider
metadata:
  name: openai-production
spec:
  # Which LLM provider
  provider: openai  # openai | anthropic | aws-bedrock | azure-openai | gcp-vertexai | custom

  # Authentication strategy
  auth:
    type: apiKey  # apiKey | externalSecret | workloadIdentity

    # --- type: apiKey ---
    # Direct reference to existing K8s Secret
    apiKey:
      secretRef:
        name: openai-api-key
        namespace: llmwarden-system    # where the master key lives
        key: api-key                  # key within the secret
      rotation:
        enabled: true
        interval: 30d                 # rotate every 30 days
        # Provider-specific: use admin API to rotate
        strategy: providerAPI         # providerAPI | recreateSecret

    # --- type: externalSecret ---
    # Delegate to External Secrets Operator
    externalSecret:
      store:
        name: vault-backend           # SecretStore or ClusterSecretStore name
        kind: ClusterSecretStore      # SecretStore | ClusterSecretStore
      remoteRef:
        key: secret/data/openai/production
        property: api-key
      refreshInterval: 1h

    # --- type: workloadIdentity ---
    # Cloud-native secretless auth
    workloadIdentity:
      # AWS
      aws:
        roleArn: arn:aws:iam::123456789012:role/bedrock-prod
        region: us-east-1
      # Azure
      azure:
        clientId: "xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx"
        tenantId: "xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx"
        # Managed Identity resource ID (optional, for user-assigned)
        managedIdentityResourceId: "/subscriptions/.../Microsoft.ManagedIdentity/..."
      # GCP
      gcp:
        serviceAccountEmail: bedrock-sa@project.iam.gserviceaccount.com
        projectId: my-project

  # Model access control
  allowedModels:
    - "gpt-4o"
    - "gpt-4o-mini"
    - "gpt-4-turbo"
  # Empty = all models allowed

  # Rate limiting (informational / enforced by admission webhook)
  rateLimit:
    requestsPerMinute: 1000
    tokensPerMinute: 100000

  # Which namespaces can create LLMAccess referencing this provider
  namespaceSelector:
    matchLabels:
      ai-tier: production
    # OR matchExpressions for more complex selection

  # Provider endpoint override (for proxies, private endpoints)
  endpoint:
    baseURL: ""                       # empty = provider default
    # e.g., "https://my-openai-proxy.internal.company.com/v1"

status:
  conditions:
    - type: Ready
      status: "True"
      reason: ProviderReachable
      message: "Provider endpoint is reachable and credentials are valid"
      lastTransitionTime: "2025-01-15T10:00:00Z"
    - type: CredentialValid
      status: "True"
      reason: KeyVerified
      message: "API key validated against provider"
      lastTransitionTime: "2025-01-15T10:00:00Z"
  lastCredentialCheck: "2025-01-15T10:00:00Z"
  accessCount: 12                     # number of LLMAccess resources referencing this
```

### LLMAccess

Namespace-scoped resource. Dev team requests access to an LLM provider for their workload.

```yaml
apiVersion: llmwarden.io/v1alpha1
kind: LLMAccess
metadata:
  name: chatbot-openai
  namespace: customer-facing
spec:
  # Reference to cluster-scoped LLMProvider
  providerRef:
    name: openai-production

  # What models this access needs (must be subset of provider's allowedModels)
  models:
    - "gpt-4o"

  # Where to put the credentials
  secretName: openai-credentials      # K8s Secret name to create in this namespace

  # Which workloads receive credential injection
  workloadSelector:
    matchLabels:
      app: chatbot-api
    # Pods matching this selector get env vars injected via mutating webhook

  # How to inject credentials into pods
  injection:
    # Environment variable mapping
    env:
      - name: OPENAI_API_KEY           # env var name in pod
        secretKey: apiKey              # key in the generated secret
      - name: OPENAI_ORG_ID
        secretKey: orgId
      - name: OPENAI_BASE_URL
        secretKey: baseUrl
    # Alternative: volume mount (for apps reading from file)
    # volume:
    #   mountPath: /etc/llmwarden/openai
    #   readOnly: true

  # Override rotation schedule (must be <= provider's interval)
  rotation:
    interval: 7d                       # optional override

status:
  conditions:
    - type: Ready
      status: "True"
      reason: CredentialProvisioned
      message: "Secret customer-facing/openai-credentials created and valid"
      lastTransitionTime: "2025-01-15T10:00:00Z"
    - type: CredentialProvisioned
      status: "True"
      reason: SecretCreated
      message: "K8s Secret created with 3 keys"
      lastTransitionTime: "2025-01-15T10:00:00Z"
    - type: InjectionReady
      status: "True"
      reason: WebhookConfigured
      message: "Mutating webhook configured for selector app=chatbot-api"
      lastTransitionTime: "2025-01-15T10:00:00Z"
  secretRef:
    name: openai-credentials
    namespace: customer-facing
    resourceVersion: "12345"
  lastRotation: "2025-01-15T10:00:00Z"
  nextRotation: "2025-01-22T10:00:00Z"
  provisionedModels:
    - "gpt-4o"
```

## Controller Architecture

### LLMProvider Controller

```
Watch: LLMProvider
Reconcile:
  1. Validate provider config (endpoint reachable, auth valid)
  2. For apiKey type: verify secret exists, optionally test key against provider API
  3. For workloadIdentity type: verify IAM role/managed identity exists
  4. For externalSecret type: verify SecretStore exists
  5. Update status conditions
  6. Requeue on interval for periodic health checks
Owns: nothing (cluster-scoped reference resource)
```

### LLMAccess Controller

```
Watch: LLMAccess, owned Secrets, owned ExternalSecrets
Reconcile:
  1. Fetch referenced LLMProvider
  2. Validate namespace allowed (namespaceSelector)
  3. Validate requested models are subset of allowedModels
  4. Determine auth strategy from provider's auth.type
  5. Call appropriate Provisioner:
     - ApiKeyProvisioner.Provision(ctx, provider, access) → creates/updates K8s Secret
     - ExternalSecretProvisioner.Provision(ctx, provider, access) → creates/updates ESO ExternalSecret
     - WorkloadIdentityProvisioner.Provision(ctx, provider, access) → annotates ServiceAccount
  6. Ensure Secret has owner reference to LLMAccess
  7. Update LLMAccess status
  8. Requeue before next rotation
Owns: Secrets, ExternalSecrets (via owner references)
```

### Mutating Webhook

```
Intercepts: Pod CREATE
Matches: Pods in namespaces with LLMAccess resources
Logic:
  1. List LLMAccess in pod's namespace
  2. For each LLMAccess, check if pod matches workloadSelector
  3. If match, patch pod spec:
     - Add env vars from LLMAccess.spec.injection.env
     - Reference the generated Secret
  4. Add annotation: llmwarden.io/injected-providers: "openai-production"
```

## Provisioner Interface

```go
type Provisioner interface {
    // Provision creates or updates credentials for the given LLMAccess
    Provision(ctx context.Context, provider *v1alpha1.LLMProvider, access *v1alpha1.LLMAccess) (*ProvisionResult, error)

    // Cleanup removes any resources created for the given LLMAccess
    Cleanup(ctx context.Context, provider *v1alpha1.LLMProvider, access *v1alpha1.LLMAccess) error

    // HealthCheck validates credentials are still valid
    HealthCheck(ctx context.Context, provider *v1alpha1.LLMProvider, access *v1alpha1.LLMAccess) (*HealthCheckResult, error)
}

type ProvisionResult struct {
    SecretName    string
    SecretKeys    []string
    ExpiresAt     *time.Time
    NeedsRotation bool
}
```

## Metrics

```
llmwarden_llmaccess_total{provider,namespace,status}           — Total LLMAccess resources by state
llmwarden_credential_rotations_total{provider,namespace}        — Credential rotation counter
llmwarden_credential_rotation_errors_total{provider,namespace}  — Rotation failures
llmwarden_credential_age_seconds{provider,namespace,name}       — Age of current credential
llmwarden_credential_next_rotation_seconds{provider,namespace}  — Time until next rotation
llmwarden_provider_health{provider,status}                      — Provider health check results
llmwarden_webhook_injections_total{namespace}                    — Webhook injection counter
```

## RBAC Model

### Operator ServiceAccount needs:
- Secrets: create, get, list, watch, update, delete (in all namespaces)
- ServiceAccounts: get, list, update (for workload identity annotations)
- ExternalSecrets (external-secrets.io): create, get, list, watch, update, delete
- LLMProviders, LLMAccess: get, list, watch, update/status
- Events: create, patch
- Pods: get, list (for webhook)

### For users (RBAC examples):
```yaml
# Platform team can manage LLMProviders
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: llmwarden-platform-admin
rules:
  - apiGroups: ["llmwarden.io"]
    resources: ["llmproviders"]
    verbs: ["*"]
  - apiGroups: ["llmwarden.io"]
    resources: ["llmaccesses"]
    verbs: ["get", "list", "watch"]

---
# Dev team can create LLMAccess in their namespace
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: llmwarden-developer
  namespace: my-team
rules:
  - apiGroups: ["llmwarden.io"]
    resources: ["llmaccesses"]
    verbs: ["get", "list", "watch", "create", "update", "delete"]
  - apiGroups: ["llmwarden.io"]
    resources: ["llmproviders"]
    verbs: ["get", "list"]   # read-only, to see what's available
```

## MVP Scope (Phase 1)

To ship something useful fast:

1. **LLMProvider CRD** with `apiKey` auth type only
2. **LLMAccess CRD** with basic secret creation
3. **LLMAccess controller** that:
   - Validates provider ref + namespace selector
   - Creates K8s Secret from provider's secretRef
   - Copies relevant key material into namespace-scoped secret
   - Sets owner reference for GC
   - Updates status conditions
4. **Mutating webhook** that injects env vars into matching pods
5. **Basic Prometheus metrics**
6. **Helm chart** for installation
7. **envtest-based integration tests**

NOT in MVP: ESO integration, workload identity, auto-rotation via provider APIs, Kyverno policies.

## Competitive Positioning

| vs. | Why llmwarden is different |
|-----|--------------------------|
| Wiz AI-SPM | Wiz detects. llmwarden provisions and manages. Wiz: $200K+. llmwarden: open source. |
| kagent | kagent runs agents. llmwarden manages credentials FOR kagent (and any agent framework). Complementary. |
| Envoy AI Gateway | Gateway proxies traffic. llmwarden manages credential lifecycle. Complementary. |
| ESO alone | ESO is generic. llmwarden adds LLM-specific rotation, model scoping, unified abstraction. |
| cert-manager | cert-manager: TLS certs. llmwarden: LLM credentials. Same philosophy, different domain. |
| Manual K8s Secrets | No rotation, no audit, no namespace isolation, no model scoping. |

## AI Agent & MCP Server Integration

### The Credential Problem is Worse for Agents

A standard workload calling an LLM API needs one credential: an API key. An AI agent is different:

- **Multiple LLM providers**: A single agent might use Anthropic for reasoning, OpenAI for embeddings, and a local model for classification.
- **Multiple MCP servers**: Each agent tool (GitHub, Slack, database) is an MCP server with its own auth — API keys, OAuth2 tokens, mTLS certificates.
- **Dynamic composition**: Agents add and remove tools at runtime, meaning credential requirements change more often than traditional workloads.

The result: an agent with 3 MCP servers and 2 LLM providers needs 5 separately-managed secrets, each with different rotation policies, different auth methods, and no unified audit trail.

### How llmwarden Integrates with kagent

[kagent](https://github.com/kagent-dev/kagent) is a Kubernetes-native agent framework (CNCF sandbox). Its `ModelConfig` CRD references a Kubernetes Secret by name:

```
┌────────────────────────────────────────────────────────────┐
│  Platform Team                                             │
│  kubectl apply -f llmprovider-anthropic.yaml               │
│         ↓                                                  │
│  LLMProvider (cluster-scoped)                              │
│  name: anthropic-production                                │
│  auth.type: apiKey → secret in llmwarden-system            │
└──────────────────────────┬─────────────────────────────────┘
                           │
┌──────────────────────────▼─────────────────────────────────┐
│  Dev Team                                                  │
│  kubectl apply -f llmaccess-agent.yaml (in kagent ns)      │
│         ↓                                                  │
│  LLMAccess (namespace-scoped, kagent namespace)            │
│  secretName: kagent-anthropic                              │
│         ↓                                                  │
│  llmwarden operator provisions K8s Secret automatically    │
│  name: kagent-anthropic, namespace: kagent                 │
└──────────────────────────┬─────────────────────────────────┘
                           │
┌──────────────────────────▼─────────────────────────────────┐
│  kagent                                                    │
│  ModelConfig references: secret kagent-anthropic           │
│  Agent uses ModelConfig → gets credentials transparently   │
│         ↓                                                  │
│  llmwarden rotates secret on schedule (no agent restart)   │
└────────────────────────────────────────────────────────────┘
```

See the [kagent integration guide](../guides/kagent-integration.md) for a complete walk-through with working YAML.

### Future: ToolProvider/ToolAccess for MCP Server Credentials

The same credential lifecycle problem applies to MCP server auth. The Phase 6 roadmap extends llmwarden's model to tool credentials:

- **ToolProvider** (cluster-scoped) — platform team declares available MCP servers with their auth config (API key, OAuth2, mTLS, basic auth).
- **ToolAccess** (namespace-scoped) — dev team requests credentials for a specific MCP server, scoped to specific capabilities.

This extends the same patterns (namespace isolation, rotation, audit, webhook injection) from LLM credentials to tool credentials. See the [ToolProvider/ToolAccess design doc](../design/tool-credential-management.md) for the full proposal.

## Security Architecture

### Threat Model

llmwarden protects against the following threat vectors:

1. **Secret Leakage**
   - Credentials exposed in pod specs, logs, or events
   - Secrets persisted in version control or backups
   - Unauthorized cross-namespace access to credentials

2. **Privilege Escalation**
   - Workloads accessing providers they shouldn't
   - Namespace label manipulation to bypass restrictions
   - RBAC bypass through direct secret access

3. **Credential Theft**
   - API keys stolen from running containers
   - Keys extracted from Kubernetes API
   - Man-in-the-middle attacks on credential provisioning

4. **Availability Attacks**
   - Resource exhaustion via excessive rotation requests
   - DoS through malformed CRD specs
   - Webhook blocking legitimate pod creation

### Security Controls

#### Defense in Depth

1. **Network Layer**
   - TLS 1.2+ enforced for all webhook and metrics endpoints
   - HTTP/2 disabled by default (CVE mitigation)
   - mTLS via cert-manager for webhook authentication

2. **API Layer**
   - Namespace isolation via `namespaceSelector` on LLMProvider
   - Model allowlisting prevents access to unauthorized models
   - Admission webhooks validate all LLMAccess configurations
   - CEL expressions for declarative validation in CRDs

3. **Secret Management**
   - Secrets never logged or exposed in events
   - Volume mounts enforced as read-only with 0400 permissions
   - Owner references ensure automatic cleanup on deletion
   - Cross-namespace secret copying uses temporary in-memory buffers

4. **RBAC**
   - Least privilege for operator ServiceAccount
   - Separate roles for platform admins vs. developers
   - Namespace-scoped LLMAccess resources
   - Cluster-scoped LLMProvider for centralized control

5. **Input Validation**
   - Duration strings capped at 365 days to prevent overflow
   - Env var names validated against POSIX standards
   - Mount paths validated to prevent conflicts
   - Reserved Kubernetes env vars protected from override

6. **Audit Trail**
   - All credential provisioning events logged to Kubernetes events API
   - Prometheus metrics track access patterns and rotation
   - Status conditions provide tamper-evident history
   - Labels track which LLMAccess created which Secret

#### Secure Defaults

- Webhook failure policy: `ignore` for pod injector (fail-open for availability)
- Webhook failure policy: `fail` for LLMAccess validator (fail-closed for security)
- Secret volume mounts: read-only with 0400 file permissions
- TLS: minimum version 1.2, prefer server cipher suites
- HTTP/2: disabled unless explicitly enabled
- Rotation: disabled by default (opt-in)

### Security Hardening Checklist

**Pre-Deployment**
- [ ] Review RBAC rules in `config/rbac/` for least privilege
- [ ] Enable cert-manager for webhook TLS certificates
- [ ] Configure namespace selectors on all LLMProviders
- [ ] Set model allowlists for production providers
- [ ] Enable rotation schedules appropriate for your threat model
- [ ] Configure Prometheus alerting on rotation failures

**Runtime**
- [ ] Monitor `llmwarden_webhook_injections_total` for unexpected spikes
- [ ] Alert on `llmwarden_credential_rotation_errors_total` > 0
- [ ] Audit `kubectl get llmaccess --all-namespaces` regularly
- [ ] Scan for Secrets without owner references (manual bypass)
- [ ] Review events: `kubectl get events --field-selector involvedObject.kind=LLMAccess`

**Incident Response**
- Revoke compromised credential: delete source Secret in provider namespace
- Rotate all downstream credentials: delete all LLMAccess resources, operator will recreate
- Identify affected workloads: `kubectl get pods -l llmwarden.io/injected-providers=<provider>`
- Forensics: check LLMAccess status conditions for last rotation timestamps

### Known Limitations (MVP)

- **No secret encryption at rest**: Relies on Kubernetes cluster configuration (enable encryption at rest on etcd)
- **No external KMS integration**: Secrets stored in etcd, not externalized (use ESO integration in Phase 2)
- **No automatic key revocation**: Rotation creates new keys but doesn't revoke old ones (Phase 4 feature)
- **No rate limiting enforcement**: RateLimit is informational only, not enforced by operator
- **No audit log export**: Events stored in Kubernetes only, no SIEM integration

### Compliance Considerations

**SOC 2 / ISO 27001**
- Credentials centrally managed (reduces shadow IT)
- Audit trail via Kubernetes events
- Separation of duties (platform team vs. dev team RBAC)
- Documented rotation policies

**PCI DSS**
- Encryption in transit (TLS 1.2+)
- Access controls (RBAC + namespace selectors)
- Audit trails (events + metrics)
- Key rotation capability

**GDPR**
- No PII stored in llmwarden resources
- API keys may contain customer data sent to LLM providers (review provider DPAs)

## Future Considerations

- **SPIRE integration**: Exchange JWT-SVID for cloud STS tokens, then for LLM provider tokens
- **Cost attribution**: Annotate pods with estimated cost based on model + rate limits
- **Gateway integration**: Auto-configure Envoy AI Gateway BackendSecurityPolicy from LLMProvider
- **Multi-cluster**: Use Liqo or Submariner patterns for cross-cluster LLMProvider federation
- **OPA/Gatekeeper**: Alternative to Kyverno for policy enforcement
- **Secrets encryption**: Integration with external KMS (AWS KMS, Azure Key Vault, GCP KMS)
- **Anomaly detection**: ML-based detection of unusual access patterns
- **Zero-trust networking**: Automatic service mesh policy generation based on LLMAccess
- **ToolProvider/ToolAccess CRDs**: Extend from LLM credentials to AI tool credentials (MCP server auth, OAuth tokens for GitHub/Slack/DB tools). Same philosophy — declarative, lifecycle-managed, auditable. See [design doc](../design/tool-credential-management.md).
- **kagent native integration**: Potential for kagent to support llmwarden as a first-class credential backend, eliminating manual secret creation entirely.
- **MCP auth spec alignment**: As the MCP authorization specification stabilizes (currently evolving), align ToolProvider auth types with the standard.
