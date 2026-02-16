# llmwarden

**cert-manager, but for LLM API credentials.**

A Kubernetes operator that makes LLM provider access declarative, secretless where possible, and auditable by default.

## The Problem

Every team in your cluster accesses LLM providers differently:

```
Team A → hardcoded sk-* in env var          ❌ static, no rotation
Team B → ESO pulls from Vault               ⚠️  key was manually created 6 months ago
Team C → IRSA for Bedrock                   ⚠️  over-scoped to bedrock:*, took 3 weeks
Team D → Azure Workload Identity            ⚠️  snowflake config, not reproducible
Team E → API key in ConfigMap               ❌ plain text, no rotation
```

Nobody has a unified, declarative way to say: **"this workload needs access to GPT-4o."**

## The Solution

```yaml
# Platform team: declare what's available
apiVersion: llmwarden.io/v1alpha1
kind: LLMProvider
metadata:
  name: openai-production
spec:
  provider: openai
  auth:
    type: apiKey
    apiKey:
      secretRef:
        name: openai-master-key
        namespace: llmwarden-system
        key: api-key
      rotation:
        enabled: true
        interval: 30d
  allowedModels: ["gpt-4o", "gpt-4o-mini"]
  namespaceSelector:
    matchLabels:
      ai-tier: production
---
# Dev team: request access
apiVersion: llmwarden.io/v1alpha1
kind: LLMAccess
metadata:
  name: chatbot-openai
  namespace: customer-facing
spec:
  providerRef:
    name: openai-production
  models: ["gpt-4o"]
  secretName: openai-credentials
  workloadSelector:
    matchLabels:
      app: chatbot-api
  injection:
    env:
      - name: OPENAI_API_KEY
        secretKey: apiKey
```

llmwarden handles: credential provisioning, namespace isolation, model access control, env injection via webhook, rotation scheduling, and status reporting.

## Features

- **Declarative LLM access** — One CRD to request credentials, any provider
- **Namespace isolation** — Platform team controls which namespaces access which providers
- **Model scoping** — Restrict access to specific models per provider
- **Automatic injection** — Mutating webhook injects credentials as env vars
- **Rotation-aware** — Tracks credential age, schedules rotation
- **Status-driven** — `kubectl get llmaccess` shows credential health
- **Auditable** — K8s events for every credential action

### Roadmap

- [ ] External Secrets Operator integration
- [ ] Multi-cloud workload identity (AWS IRSA, Azure WI, GCP WIF) via unified CRD
- [ ] Auto-rotation via provider admin APIs (OpenAI, Anthropic)
- [ ] Kyverno policy generation
- [ ] SPIRE integration for OIDC token exchange

## Quick Start

```bash
# Install CRDs
kubectl apply -f config/crd/

# Install operator
helm install llmwarden charts/llmwarden -n llmwarden-system --create-namespace

# Create a master API key secret
kubectl create secret generic openai-master-key \
  -n llmwarden-system \
  --from-literal=api-key=sk-your-key-here

# Create an LLMProvider
kubectl apply -f config/samples/llmprovider-openai.yaml

# Create an LLMAccess in your namespace
kubectl apply -f config/samples/llmaccess-basic.yaml

# Check status
kubectl get llmproviders
kubectl get llmaccess -A
```

## Architecture

```
┌─────────────────────────────────────────────────┐
│                  llmwarden operator                │
│                                                   │
│  ┌──────────────┐  ┌───────────────┐             │
│  │  LLMProvider  │  │   LLMAccess   │             │
│  │  Controller   │  │  Controller   │             │
│  └──────┬───────┘  └───────┬───────┘             │
│         │                  │                      │
│         │    ┌─────────────┼─────────────┐       │
│         │    │             │             │        │
│         ▼    ▼             ▼             ▼        │
│  ┌──────────────┐ ┌──────────────┐ ┌──────────┐ │
│  │   ApiKey      │ │ ExtSecret    │ │ Workload │ │
│  │  Provisioner  │ │ Provisioner  │ │ Identity │ │
│  └──────────────┘ └──────────────┘ └──────────┘ │
│                                                   │
│  ┌──────────────────────────────────────────┐    │
│  │         Mutating Webhook                  │    │
│  │    (inject env vars into matching pods)   │    │
│  └──────────────────────────────────────────┘    │
└─────────────────────────────────────────────────┘
         │                │                │
         ▼                ▼                ▼
    K8s Secrets     ESO Resources    SA Annotations
```

llmwarden is a thin orchestration layer. It delegates to battle-tested CNCF projects:

| Function | Delegated to | What llmwarden adds |
|----------|-------------|-------------------|
| Secret storage/sync | External Secrets Operator | LLM-aware templates, rotation policies |
| Workload identity | Cloud-native (IRSA, Azure WI, GCP WIF) | Unified CRD abstraction |
| Policy enforcement | Kyverno / OPA | Ships default policies |
| Credential injection | K8s mutating webhooks | Standard pattern |

## Development

```bash
# Prerequisites: Go 1.23+, kubectl, kind/minikube

# Generate code and manifests
make generate manifests

# Run tests
make test

# Run locally against a cluster
make install   # Install CRDs
make run       # Run controller

# Build container
make docker-build IMG=ghcr.io/youruser/llmwarden:dev
```

## Contributing

Contributions welcome. See [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines.

## License

Apache License 2.0 — see [LICENSE](LICENSE).
