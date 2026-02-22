# Design: ToolProvider/ToolAccess for MCP & AI Tool Credential Management

**Status:** Proposal (Phase 6 — not yet implemented)
**Author:** llmwarden maintainers
**Last updated:** 2026-02-22

---

## 1. Problem Statement

llmwarden Phase 1–4 solves credential lifecycle management for LLM providers. But AI agents don't just call LLM APIs. They call tools.

A modern agent deployed on Kubernetes with [kagent](https://github.com/kagent-dev/kagent) might connect to:

- GitHub (via MCP server) — for code search, PR creation
- Slack (via MCP server) — for notifications
- A PostgreSQL database (via MCP server) — for data access
- A vector database (via MCP server) — for retrieval

Each of these requires separate authentication. Today, every connection is handled manually:

- kagent `RemoteMCPServer` uses hardcoded URLs with tokens embedded or manual secrets
- Each MCP server has different auth requirements: API keys, OAuth2 tokens, mTLS client certs, basic auth
- No rotation, no scoping, no audit for any of these tool credentials
- An agent with 5 MCP servers = 5 manually-managed secrets, each with different lifecycles

### Concrete Example of the Problem

```yaml
# What kagent docs tell you to do today for each MCP server:
kubectl create secret generic github-mcp-token -n agents \
  --from-literal=token=ghp_REPLACE_WITH_REAL_TOKEN

kubectl create secret generic slack-mcp-token -n agents \
  --from-literal=token=xoxb-REPLACE_WITH_REAL_TOKEN

kubectl create secret generic postgres-password -n agents \
  --from-literal=password=REPLACE_WITH_REAL_PASSWORD
```

This is the same problem llmwarden solved for LLM provider keys — fragmented, manual, unaudited, unrotated. The solution is the same pattern: a platform-team-controlled declaration of what tools are available, and a dev-team-controlled request for access.

---

## 2. Proposed CRDs

### ToolProvider (cluster-scoped)

Platform team declares an available MCP server or AI tool, including its endpoint and auth configuration.

```yaml
apiVersion: llmwarden.io/v1alpha1
kind: ToolProvider
metadata:
  name: github-mcp
spec:
  # Tool type determines which provisioner handles credentials
  type: mcp                              # mcp | rest-api | database | custom

  # Where the tool is hosted
  endpoint:
    url: "https://mcp.github.internal/sse"

  # Authentication configuration
  auth:
    type: oauth2                         # apiKey | oauth2 | mtls | basicAuth

    # --- type: apiKey ---
    # apiKey:
    #   secretRef:
    #     name: github-pat
    #     namespace: llmwarden-system
    #     key: token
    #   headerName: "Authorization"      # default: "Authorization"
    #   headerPrefix: "Bearer"           # default: "Bearer"

    # --- type: oauth2 ---
    oauth2:
      flow: clientCredentials            # clientCredentials | authorizationCode
      tokenEndpoint: "https://github.com/login/oauth/access_token"
      clientCredentialRef:
        store: vault-backend             # ESO SecretStore or ClusterSecretStore
        key: github/oauth-client
      scopes: ["repo", "read:org"]
      rotationPolicy:
        interval: 90d                    # OAuth tokens typically short-lived

    # --- type: mtls ---
    # mtls:
    #   clientCertRef:
    #     name: github-mcp-client-cert   # cert-manager Certificate resource
    #     namespace: llmwarden-system
    #   caRef:
    #     name: github-mcp-ca-bundle
    #     namespace: llmwarden-system

    # --- type: basicAuth ---
    # basicAuth:
    #   secretRef:
    #     name: tool-basic-creds
    #     namespace: llmwarden-system
    #     usernameKey: username
    #     passwordKey: password

  # Which namespaces can create ToolAccess resources referencing this provider
  namespaceSelector:
    matchLabels:
      ai-tools: enabled

  # Capability scoping — platform team declares what operations are available
  # These map to MCP tool names or higher-level permission groups
  allowedCapabilities:
    - "read_file"
    - "search_code"
    - "create_pull_request"
    - "list_issues"

status:
  conditions:
    - type: Ready
      status: "True"
      reason: EndpointReachable
      message: "MCP server is reachable and credentials are valid"
      lastTransitionTime: "2026-02-22T10:00:00Z"
  accessCount: 3
  lastHealthCheck: "2026-02-22T10:00:00Z"
```

### ToolAccess (namespace-scoped)

Dev team requests credentials for a specific tool, scoped to specific capabilities, for a specific workload.

```yaml
apiVersion: llmwarden.io/v1alpha1
kind: ToolAccess
metadata:
  name: agent-github
  namespace: agents
spec:
  # Reference to cluster-scoped ToolProvider
  toolProviderRef:
    name: github-mcp

  # Where to put the credentials
  secretName: github-mcp-credentials     # llmwarden creates this Secret

  # Capability scoping — must be a subset of ToolProvider.spec.allowedCapabilities
  # An agent doing only code review should not get create_pull_request
  capabilities:
    - "read_file"
    - "search_code"

  # Which agent pods receive credential injection
  workloadSelector:
    matchLabels:
      kagent.dev/agent: code-review-agent

  # How to inject credentials into the agent pod
  injection:
    env:
      - name: GITHUB_TOKEN
        secretKey: token
      - name: MCP_GITHUB_URL
        secretKey: endpoint

status:
  conditions:
    - type: Ready
      status: "True"
      reason: CredentialProvisioned
      message: "Secret agents/github-mcp-credentials created and valid"
      lastTransitionTime: "2026-02-22T10:00:00Z"
  secretRef:
    name: github-mcp-credentials
    namespace: agents
  lastRotation: "2026-02-22T10:00:00Z"
  nextRotation: "2026-05-23T10:00:00Z"
  grantedCapabilities:
    - "read_file"
    - "search_code"
```

---

## 3. Auth Type Details

### `apiKey`

Simple bearer token or header-based authentication. Same implementation as `LLMProvider.spec.auth.type: apiKey`.

- **Provisioner**: Reads the master key from a K8s Secret in `llmwarden-system`, creates a namespace-scoped copy.
- **Rotation**: Calls a provider-specific admin API or regenerates from ESO.
- **Use cases**: GitHub PAT, Slack bot token, simple REST API tokens.

### `oauth2`

OAuth 2.0 client credentials or authorization code flows. Two sub-flows:

**Client Credentials** (service-to-service, no user interaction):
- Provisioner calls token endpoint with `client_id` + `client_secret` from referenced ESO store.
- Token stored in the namespace Secret. Refreshed before expiry.
- Suitable for: service accounts, automation tokens.

**Authorization Code** (user-delegated, requires manual consent):
- More complex. Provisioner generates an authorization URL and stores a pending state.
- Operator waits for a callback or manual token provision.
- Suitable for: user-context GitHub access, Slack user tokens.
- **MVP approach**: Support client credentials only; authorization code is out-of-scope for initial implementation.

Token refresh handling:
- Provisioner stores `access_token`, `refresh_token`, and `expires_at` in the Secret.
- Requeue before `expires_at` to refresh proactively.
- If refresh fails, set `Ready=False` and emit an event.

### `mtls`

Mutual TLS with a client certificate. Integrates with cert-manager.

- **Provisioner**: Creates a cert-manager `Certificate` resource in the target namespace, owned by the `ToolAccess`.
- cert-manager provisions the client cert and stores it in a Secret.
- `ToolAccess` status reflects the cert's expiry from the cert-manager `Certificate` status.
- **Rotation**: cert-manager handles automatic renewal; llmwarden monitors status.

### `basicAuth`

Username/password authentication.

- **Provisioner**: Same as `apiKey` — copies from `llmwarden-system` Secret to target namespace.
- **Rotation**: Requires provider-specific admin API or manual update to master secret.
- **Use cases**: PostgreSQL, legacy APIs, internal services.

---

## 4. Integration with kagent

### How ToolAccess provisions secrets for kagent `RemoteMCPServer`

Today, users configure kagent `RemoteMCPServer` with hardcoded credentials. With ToolAccess, llmwarden provisions the Secret and the `RemoteMCPServer` references it.

```yaml
# Step 1: Platform team declares the MCP server
apiVersion: llmwarden.io/v1alpha1
kind: ToolProvider
metadata:
  name: github-mcp
spec:
  type: mcp
  endpoint:
    url: "https://mcp.github.internal/sse"
  auth:
    type: apiKey
    apiKey:
      secretRef:
        name: github-mcp-master-token
        namespace: llmwarden-system
        key: token
  namespaceSelector:
    matchLabels:
      ai-tools: enabled
---
# Step 2: Dev team requests access in their agent namespace
apiVersion: llmwarden.io/v1alpha1
kind: ToolAccess
metadata:
  name: agent-github
  namespace: agents
spec:
  toolProviderRef:
    name: github-mcp
  secretName: github-mcp-creds           # llmwarden creates this
  capabilities: ["read_file", "search_code"]
  workloadSelector:
    matchLabels:
      kagent.dev/agent: code-review-agent
  injection:
    env:
      - name: GITHUB_TOKEN
        secretKey: token
---
# Step 3: kagent RemoteMCPServer references the llmwarden-managed secret
# (Hypothetical future kagent API — exact fields depend on kagent implementation)
apiVersion: kagent.dev/v1alpha2
kind: RemoteMCPServer
metadata:
  name: github-mcp
  namespace: agents
spec:
  url: "https://mcp.github.internal/sse"
  auth:
    type: bearer
    tokenSecretRef:
      name: github-mcp-creds             # matches ToolAccess.spec.secretName
      key: token
```

The same lifecycle properties apply: rotation without agent restart, namespace isolation, capability scoping, audit trail.

---

## 5. Relationship to LLMProvider/LLMAccess

ToolProvider/ToolAccess deliberately mirrors the LLMProvider/LLMAccess design:

| Aspect | LLMProvider/LLMAccess | ToolProvider/ToolAccess |
|--------|----------------------|------------------------|
| Scope | Cluster / Namespace | Cluster / Namespace |
| Platform control | `namespaceSelector` | `namespaceSelector` |
| Access scoping | `allowedModels` | `allowedCapabilities` |
| Credential output | K8s Secret | K8s Secret |
| Injection | Mutating webhook | Mutating webhook (same) |
| Status | Conditions + events | Conditions + events (same) |
| Rotation | Provider-specific | Provider-specific |
| Provisioner interface | `Provisioner` | `Provisioner` (same interface) |

**Shared infrastructure (Phase 6 reuse):**
- The `Provisioner` interface defined in `internal/provisioner/interface.go` is used unchanged.
- The mutating webhook in `internal/webhook/injector.go` is extended to handle `ToolAccess` resources alongside `LLMAccess`.
- The same status condition types (`Ready`, `CredentialProvisioned`, `RotationDue`, `Error`) apply.
- RBAC model is identical: cluster-scoped declaration, namespace-scoped request.

**Future consideration — unified Provider CRD:**

A possible long-term consolidation: merge `LLMProvider` into `ToolProvider` as `type: llm` vs `type: mcp`. This would simplify the operator surface. However, this is deferred to avoid breaking changes and because LLM providers have unique concerns (model allowlisting, provider-specific rotation APIs) that don't map cleanly to generic tool auth.

For Phase 6, keep `ToolProvider` and `LLMProvider` as separate CRDs. They share code, not API surface.

---

## 6. Dependency on MCP Auth Spec

The MCP authorization specification is actively evolving. As of early 2026:

- The MCP spec defines an OAuth 2.1-based authorization framework for MCP servers.
- The spec distinguishes between **user-level auth** (OAuth authorization code flow with PKCE) and **service-level auth** (client credentials or pre-provisioned tokens).
- The spec is not yet stable enough to build a production implementation against.

**This is the primary reason ToolProvider/ToolAccess is Phase 6, not Phase 3.**

The `auth.type` field in ToolProvider is designed to be extensible. When the MCP auth spec stabilizes, we add new auth types without breaking existing `ToolProvider` resources. The provisioner interface allows adding new implementations without changing the CRD schema.

**Alignment principles:**
- Prefer OAuth 2.1 flows where the MCP spec mandates them.
- Do not add MCP-specific credential types until the spec stabilizes.
- Track the MCP spec at [modelcontextprotocol.io](https://modelcontextprotocol.io/) and update this design before implementation.

---

## 7. Open Questions

The following questions need resolution before implementation begins.

### 7.1 MCP Server Discovery

**Question:** Should ToolProvider support MCP server discovery (SSE-based), or require explicit endpoint configuration?

**Options:**
- **Explicit only** (simpler): Platform team specifies the URL in `ToolProvider.spec.endpoint.url`. Discovery is out of scope.
- **Discovery via SSE**: ToolProvider fetches the MCP server's capability manifest at reconcile time and validates that `allowedCapabilities` are a subset of what the server exposes.

**Leaning:** Start with explicit-only. Add discovery as a feature flag later. Discovery adds complexity and requires the operator to maintain connections to MCP servers.

### 7.2 Capability Scoping Granularity

**Question:** Should `allowedCapabilities` map to MCP tool names (e.g., `"github.read_file"`) or to higher-level permission groups (e.g., `"read-only"`)?

**Options:**
- **MCP tool names**: Fine-grained, directly maps to the MCP tool manifest. Risk: names change across MCP server versions.
- **Permission groups**: Coarse-grained, stable, but requires a mapping layer that the platform team maintains.
- **Both**: ToolProvider supports both, with tool names taking precedence.

**Leaning:** Start with MCP tool names for precision, with a warning if a capability name doesn't match the server's tool manifest (requires discovery).

### 7.3 Multiple Tools per ToolAccess

**Question:** Should a single `ToolAccess` be able to reference multiple `ToolProvider` resources, or should it be one-to-one?

**Options:**
- **One-to-one** (current design): One `ToolAccess` per tool. An agent with 5 MCP servers needs 5 `ToolAccess` resources.
- **One-to-many**: A single `ToolAccess` requests credentials for multiple tools, injecting all into the same Secret or separate Secrets.

**Leaning:** One-to-one, matching the `LLMAccess` pattern. Multiple `ToolAccess` resources per agent is explicit and auditable. A future `AgentAccess` composite resource could bundle common combinations.

### 7.4 Per-User vs. Service-Level Auth

**Question:** How should ToolProvider handle MCP servers that require per-user OAuth (authorization code flow) vs. service-level auth (client credentials)?

**Context:** Some MCP servers (e.g., GitHub, Google Drive) support both flows. Authorization code flow requires user consent and produces user-context tokens, which cannot be pre-provisioned by an operator.

**Options:**
- **Service-level only (Phase 6 MVP)**: Only support `oauth2.flow: clientCredentials`. Per-user auth is out of scope.
- **User token bootstrap**: Add a `ToolAccess` state `PendingUserConsent` with a generated authorization URL in status. A human approves, the token is stored, and llmwarden manages refresh.
- **Delegate to an identity provider**: Integrate with Dex or similar OIDC broker to handle user consent flows.

**Leaning:** Service-level only for Phase 6 MVP. Per-user auth requires a significant UX component (consent flow, callback handling) that is better addressed in a dedicated Phase 7.

---

## Appendix: Incremental Implementation Plan

If approved, Phase 6 should be implemented incrementally:

1. **ToolProvider CRD** (types only, no controller) — unblock consumers from defining the API
2. **ToolAccess CRD** (types only) — same
3. **ToolProvider controller** — health checks, status conditions; `apiKey` auth type only
4. **ToolAccess controller** — provisioner for `apiKey` type; reuse existing provisioner interface
5. **Webhook extension** — extend `injector.go` to handle `ToolAccess`
6. **OAuth2 provisioner** — client credentials flow only
7. **mTLS provisioner** — cert-manager integration
8. **kagent e2e tests** — automated tests with kagent CRDs (requires kagent in test cluster)
