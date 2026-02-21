# CLAUDE.md — llmwarden

## Project Overview

**llmwarden** is a Kubernetes operator that makes LLM provider access declarative, secretless where possible, and auditable by default. Think "cert-manager, but for LLM API credentials."

It solves the fragmentation problem: every team in a K8s cluster accesses LLM providers differently — hardcoded API keys, manually-created secrets, cloud-specific workload identity setups that took weeks to configure. llmwarden provides a single declarative interface that abstracts credential lifecycle management across all LLM providers and auth methods.

**What llmwarden is NOT:**
- Not an LLM gateway/proxy (use Envoy AI Gateway, LiteLLM, etc. for that)
- Not a security scanner (use Wiz AI-SPM, Kubescape, etc.)
- Not an agent framework (use kagent, LangGraph, etc.)

**What llmwarden IS:**
- A credential lifecycle operator for LLM provider access
- A unified multi-cloud workload identity abstraction
- A platform engineering tool that makes the secure path the easy path

## Architecture

### Core CRDs

```
LLMProvider (cluster-scoped)     — Platform team declares available LLM providers + auth config
LLMAccess (namespace-scoped)     — Dev team requests access to a provider for their workload
LLMAccessStatus                  — Operator reports credential state, rotation timestamps, health
```

### Controller Flow

```
LLMAccess created
  → Validate against LLMProvider (namespace selector, model allowlist, RBAC)
  → Determine auth strategy from LLMProvider.spec.auth.type:
     ├── "externalSecret" → Create/manage ESO ExternalSecret resource
     ├── "workloadIdentity" → Annotate ServiceAccount for IRSA/Azure WI/GCP WIF
     └── "apiKey" → Direct K8s Secret with rotation via provider admin APIs
  → Inject credentials into target workload (mutating webhook patches env vars)
  → Monitor rotation schedule, update status
  → Emit events on rotation, expiry warnings, errors
```

### Key Design Decisions

1. **Delegate heavy lifting** — ESO for secret sync, cloud-native for workload identity, Kyverno for policy. llmwarden is thin orchestration.
2. **Provider-aware** — Unlike ESO which treats all secrets as opaque strings, llmwarden knows OpenAI keys start with `sk-`, knows the OpenAI admin API for rotation, knows Bedrock needs specific IAM actions per model.
3. **Namespace isolation** — LLMProvider uses `namespaceSelector` to control which namespaces can request access. LLMAccess is namespace-scoped for RBAC.
4. **Status-driven** — All state visible via `kubectl get llmaccess` with conditions (Ready, CredentialProvisioned, RotationDue, Error).

## Tech Stack

- **Language:** Go 1.23+
- **Operator SDK:** Kubebuilder v4 (controller-runtime, controller-gen)
- **CRD Validation:** CEL expressions in CRD markers where possible, webhook validation for complex rules
- **Testing:** envtest for integration, fake client for unit, chainsaw for e2e
- **CI:** GitHub Actions
- **Container:** Distroless base image (gcr.io/distroless/static:nonroot)
- **Helm:** Helm chart for installation

## Project Structure

```
llmwarden/
├── CLAUDE.md                    # This file
├── README.md
├── LICENSE                      # Apache 2.0
├── Makefile
├── Dockerfile
├── go.mod / go.sum
├── PROJECT                      # Kubebuilder project metadata
├── cmd/
│   └── main.go                  # Entrypoint
├── api/
│   └── v1alpha1/
│       ├── groupversion_info.go
│       ├── llmprovider_types.go
│       ├── llmaccess_types.go
│       └── zz_generated.deepcopy.go
├── internal/
│   ├── controller/
│   │   ├── llmprovider_controller.go
│   │   ├── llmaccess_controller.go
│   │   └── llmaccess_controller_test.go
│   ├── provisioner/              # Auth strategy implementations
│   │   ├── interface.go          # Provisioner interface
│   │   ├── externalsecret.go     # ESO integration
│   │   ├── workloadidentity.go   # Cloud WI abstraction
│   │   ├── apikey.go             # Direct secret + rotation
│   │   └── provisioner_test.go
│   ├── webhook/
│   │   ├── injector.go           # Mutating webhook for env injection
│   │   └── validator.go          # Validating webhook for LLMAccess
│   ├── rotation/
│   │   ├── manager.go            # Rotation scheduler
│   │   └── providers/            # Provider-specific rotation
│   │       ├── openai.go
│   │       ├── anthropic.go
│   │       └── aws.go
│   └── metrics/
│       └── metrics.go            # Prometheus metrics
├── config/
│   ├── crd/                      # Generated CRD manifests
│   ├── manager/                  # Controller manager deployment
│   ├── rbac/                     # RBAC for operator
│   ├── webhook/                  # Webhook configuration
│   ├── samples/                  # Example CRs
│   └── default/                  # Kustomize default overlay
├── charts/
│   └── llmwarden/                 # Helm chart
│       ├── Chart.yaml
│       ├── values.yaml
│       └── templates/
├── docs/
│   ├── architecture.md
│   └── getting-started.md
└── test/
    ├── e2e/
    └── integration/
```

## Coding Standards

### Go Conventions
- Follow standard Go project layout and Kubebuilder conventions
- Use `sigs.k8s.io/controller-runtime` patterns (Reconciler interface, client.Client, etc.)
- Error handling: wrap errors with `fmt.Errorf("context: %w", err)` for chain
- Logging: use `logr` from controller-runtime (`log.FromContext(ctx)`)
- Context: always pass and respect `context.Context`
- No global state; inject dependencies via struct fields

### Controller Patterns
- Reconcilers must be idempotent — running twice produces same result
- Use `controllerutil.CreateOrUpdate` for owned resources
- Set owner references for garbage collection
- Use status conditions following K8s conventions (`metav1.Condition`)
- Requeue with backoff on transient errors, don't requeue on permanent errors
- Use finalizers for cleanup of external resources

### Testing
- Unit tests: use fake client (`fake.NewClientBuilder()`) for controller logic
- Integration tests: use envtest (real API server, no real cluster)
- Test file naming: `*_test.go` in same package
- Table-driven tests preferred
- Mock external APIs (OpenAI admin, AWS STS) with httptest

### CRD Design
- Use `+kubebuilder:validation` markers for field validation
- Use CEL for cross-field validation where possible
- Status subresource enabled on all CRDs
- Print columns for useful `kubectl get` output
- Short names: `llmp` for LLMProvider, `llma` for LLMAccess

### Security
- Principle of least privilege for RBAC
- No secrets logged or exposed in events (API keys are redacted from all logs)
- Audit-relevant actions emit K8s events
- Webhook TLS via cert-manager
- TLS 1.2+ minimum version enforced
- HTTP/2 disabled by default to mitigate CVEs
- Input validation prevents DoS via duration overflow (max 365 days)
- Context propagation prevents context leakage
- Secret volume mounts enforced as read-only with 0400 permissions
- Mount path conflict detection prevents accidental overwrites
- Reserved env vars protected from override
- Env var names validated against POSIX standards

## Phase Plan

### Phase 1: MVP (current target)
- [x] Kubebuilder scaffold with LLMProvider + LLMAccess CRDs
- [x] LLMProvider controller (validation, status)
- [x] LLMAccess controller with `apiKey` strategy (creates K8s Secret from inline secretRef)
- [x] Basic mutating webhook (inject env vars into matching pods)
- [x] Status conditions, events, basic metrics
- [x] Makefile, Dockerfile, basic Helm chart
- [x] Unit + integration tests with envtest
- [x] README with getting-started guide

### Phase 2: ESO Integration
- [ ] ExternalSecret provisioner (creates ESO ExternalSecret CRs)
- [ ] SecretStore/ClusterSecretStore reference support
- [ ] Rotation policy passthrough to ESO

### Phase 3: Workload Identity Abstraction
- [ ] AWS IRSA provisioner (annotate SA, create IAM role binding)
- [ ] Azure Workload Identity provisioner (federated credential setup)
- [ ] GCP Workload Identity Federation provisioner
- [ ] Unified CRD spec that abstracts cloud differences

### Phase 4: Provider-Aware Rotation
- [ ] OpenAI admin API key rotation
- [ ] Anthropic admin API key rotation
- [ ] Rotation scheduler with jitter
- [ ] Pre-rotation health check (validate new key before swapping)

### Phase 5: Policy & Audit
- [ ] Kyverno policy generation ("no hardcoded LLM keys")
- [ ] `kubectl llmwarden audit` — inventory of all LLM access
- [ ] Cost attribution labels/annotations
- [ ] SPIRE integration for OIDC token exchange (stretch)

## API Reference

See `docs/architecture.md` for full CRD specs with examples.

## Common Commands

```bash
# Generate CRDs and deepcopy
make generate
make manifests

# Run locally against cluster
make install    # Install CRDs
make run        # Run controller locally

# Test
make test              # Unit + integration
make test-e2e          # E2E with chainsaw

# Build
make docker-build IMG=ghcr.io/thinkingcow-dev/llmwarden:latest
make docker-push IMG=ghcr.io/thinkingcow-dev/llmwarden:latest

# Deploy
make deploy IMG=ghcr.io/thinkingcow-dev/llmwarden:latest

# Helm
helm install llmwarden charts/llmwarden -n llmwarden-system --create-namespace
```

## Key External References

- Kubebuilder book: https://book.kubebuilder.io/
- controller-runtime: https://pkg.go.dev/sigs.k8s.io/controller-runtime
- External Secrets Operator: https://external-secrets.io/
- OpenAI Admin API: https://platform.openai.com/docs/api-reference/administration
- cert-manager architecture (inspiration): https://cert-manager.io/docs/concepts/
