# llmwarden Documentation

Complete documentation for the llmwarden Kubernetes operator.

## Getting Started

New to llmwarden? Start here:

- **[Getting Started Guide](./getting-started.md)** - Install llmwarden and create your first LLM provider integration
- **[Local Development Guide](./local-development.md)** - Set up a local kind cluster with a sample AI application
- **[Developer Cheat Sheet](./dev-cheatsheet.md)** - Quick reference for common commands and workflows

## Core Documentation

- **[Architecture](./architecture.md)** - Deep dive into llmwarden's design, CRD specs, and controller flow
- **[CLAUDE.md](../CLAUDE.md)** - Project overview, tech stack, coding standards, and phase plan

## Quick Links

### Installation

- **Production:** [Getting Started - Installation](./getting-started.md#installation)
- **Local Development:** [Local Development - Setting Up kind](./local-development.md#setting-up-kind-cluster)

### Examples

- **Sample Manifests:** [config/samples/](../config/samples/)
- **Local Dev Examples:** [examples/local-dev/](../examples/local-dev/)

### Common Tasks

- **Create an LLMProvider:** [Getting Started - Creating Your First LLM Provider](./getting-started.md#creating-your-first-llm-provider)
- **Request Access:** [Getting Started - Requesting Access](./getting-started.md#requesting-access-from-a-workload)
- **Deploy Sample App:** [Local Development - Deploying Sample App](./local-development.md#deploying-a-sample-ai-application)
- **Troubleshooting:** [Getting Started - Troubleshooting](./getting-started.md#troubleshooting)

## Documentation Map

```
docs/
├── README.md                    # This file - documentation index
├── getting-started.md           # Production installation and first steps
├── local-development.md         # Local kind cluster setup with sample app
├── dev-cheatsheet.md            # Quick reference commands
├── architecture.md              # Deep technical documentation
└── claude-code-quickstart.md    # AI-assisted development guide

examples/
└── local-dev/
    ├── README.md                # Local development example overview
    ├── llmprovider.yaml         # Sample LLMProvider
    ├── llmaccess.yaml           # Sample LLMAccess
    └── chatbot-app.yaml         # Demo AI chatbot application

config/
└── samples/
    ├── llmprovider-openai.yaml  # OpenAI provider example
    ├── llmprovider-bedrock.yaml # AWS Bedrock provider example
    └── llmaccess-basic.yaml     # Basic access request example
```

## Learning Path

### 1. Understand the Concepts

Read the [Architecture](./architecture.md) document to understand:
- What problem llmwarden solves
- Core CRDs (LLMProvider, LLMAccess)
- How credential provisioning works
- Authentication strategies (apiKey, externalSecret, workloadIdentity)

### 2. Try It Locally

Follow the [Local Development Guide](./local-development.md):
- Set up a kind cluster
- Install llmwarden
- Deploy a sample AI chatbot
- See credential injection in action

### 3. Deploy to Production

Use the [Getting Started Guide](./getting-started.md):
- Install on a real cluster
- Configure your LLM providers
- Grant access to development teams
- Monitor and troubleshoot

### 4. Develop Features

Reference the [Developer Cheat Sheet](./dev-cheatsheet.md):
- Common kubectl commands
- Testing workflows
- Debugging techniques
- Development best practices

## API Reference

Explore the CRD specifications:

```bash
# View full LLMProvider spec
kubectl explain llmprovider.spec

# View auth options
kubectl explain llmprovider.spec.auth

# View LLMAccess spec
kubectl explain llmaccess.spec

# View injection configuration
kubectl explain llmaccess.spec.injection
```

Detailed API documentation: [architecture.md - CRD Specifications](./architecture.md#crd-specifications)

## Development

### Prerequisites

- Go 1.23+
- kubectl
- kind or access to a K8s cluster
- Helm 3 (optional)

### Quick Setup

```bash
# Clone the repo
git clone https://github.com/yourusername/llmwarden.git
cd llmwarden

# Create kind cluster
kind create cluster --name llmwarden-dev

# Install CRDs
make install

# Run operator locally
make run
```

Full guide: [Local Development Guide](./local-development.md)

## Architecture Overview

```
Platform Team                    Development Team
     │                                │
     ▼                                ▼
 LLMProvider                      LLMAccess
(cluster-scoped)              (namespace-scoped)
     │                                │
     └────────┬───────────────────────┘
              ▼
      llmwarden operator
              │
    ┌─────────┼─────────┐
    ▼         ▼         ▼
  Secret   SA Annot  Events
  (env)   (workload  (audit)
          identity)
```

Detailed flow: [architecture.md - Controller Architecture](./architecture.md#controller-architecture)

## CRD Quick Reference

### LLMProvider

Cluster-scoped resource declaring available LLM providers.

```yaml
apiVersion: llmwarden.io/v1alpha1
kind: LLMProvider
metadata:
  name: openai-production
spec:
  provider: openai
  auth:
    type: apiKey  # or: externalSecret, workloadIdentity
  allowedModels: ["gpt-4o", "gpt-4o-mini"]
  namespaceSelector:
    matchLabels:
      ai-tier: production
```

Full spec: [architecture.md - LLMProvider](./architecture.md#llmprovider)

### LLMAccess

Namespace-scoped resource requesting access to a provider.

```yaml
apiVersion: llmwarden.io/v1alpha1
kind: LLMAccess
metadata:
  name: my-app-openai
  namespace: my-namespace
spec:
  providerRef:
    name: openai-production
  models: ["gpt-4o"]
  secretName: openai-credentials
  workloadSelector:
    matchLabels:
      app: my-app
  injection:
    env:
      - name: OPENAI_API_KEY
        secretKey: apiKey
```

Full spec: [architecture.md - LLMAccess](./architecture.md#llmaccess)

## Supported Providers

Current support (Phase 1 - MVP):

- ✅ **OpenAI** - API key auth
- ✅ **Anthropic** - API key auth
- ✅ **AWS Bedrock** - API key auth (workload identity in Phase 3)
- ⚠️  **Azure OpenAI** - API key auth (full support in Phase 3)
- ⚠️  **GCP Vertex AI** - API key auth (full support in Phase 3)

Roadmap: [CLAUDE.md - Phase Plan](../CLAUDE.md#phase-plan)

## Authentication Methods

### Phase 1 (Available Now)

- **apiKey** - K8s Secret reference with optional rotation

### Phase 2 (Planned)

- **externalSecret** - Integration with External Secrets Operator

### Phase 3 (Planned)

- **workloadIdentity** - AWS IRSA, Azure WI, GCP WIF

Details: [architecture.md - CRD Specifications](./architecture.md#crd-specifications)

## Common Use Cases

### Use Case 1: Platform Team - Enable OpenAI Access

1. Store master key: `kubectl create secret`
2. Create LLMProvider with namespace selector
3. Dev teams can now request access

Guide: [Getting Started - Creating Your First LLM Provider](./getting-started.md#creating-your-first-llm-provider)

### Use Case 2: Dev Team - Add AI to Existing App

1. Request LLMAccess in your namespace
2. Add label to your pods
3. Credentials auto-injected on pod creation

Guide: [Getting Started - Requesting Access](./getting-started.md#requesting-access-from-a-workload)

### Use Case 3: Security Team - Audit AI Usage

```bash
# List all providers
kubectl get llmproviders

# List all access grants
kubectl get llmaccesses -A

# Find all pods using AI
kubectl get pods -A -l llmwarden.io/injected=true
```

### Use Case 4: Developer - Test Locally

1. Create kind cluster
2. Deploy sample AI chatbot
3. Verify credential injection

Guide: [Local Development Guide](./local-development.md)

## Troubleshooting

Common issues and solutions:

- [Provider not ready](./getting-started.md#problem-llmprovider-shows-readyfalse)
- [Namespace not allowed](./getting-started.md#problem-llmaccess-shows-readyfalse-with-namespace-not-allowed)
- [Env var not injected](./getting-started.md#problem-environment-variable-not-injected-into-pod)
- [Webhook certificate errors](./getting-started.md#problem-x509-certificate-signed-by-unknown-authority-webhook-errors)

Full troubleshooting guide: [Getting Started - Troubleshooting](./getting-started.md#troubleshooting)

## Contributing

We welcome contributions! See [CONTRIBUTING.md](../CONTRIBUTING.md) for:

- Code of conduct
- Development workflow
- Coding standards
- PR process

Also review: [CLAUDE.md - Coding Standards](../CLAUDE.md#coding-standards)

## Support

- **Issues:** [GitHub Issues](https://github.com/yourusername/llmwarden/issues)
- **Discussions:** [GitHub Discussions](https://github.com/yourusername/llmwarden/discussions)

## License

Apache License 2.0 - See [LICENSE](../LICENSE)
