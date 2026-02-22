# kagent Integration Guide

## Overview

[kagent](https://github.com/kagent-dev/kagent) is a Kubernetes-native AI agent framework (CNCF sandbox). It runs agents as pods and references LLM credentials via Kubernetes Secrets. llmwarden automates the credential lifecycle for those secrets — provisioning, rotation, and namespace isolation — so you never touch `kubectl create secret` again.

---

## The Problem

Every kagent `ModelConfig` requires a Kubernetes Secret containing an API key:

```bash
# What kagent docs tell you to do today:
kubectl create secret generic kagent-anthropic -n kagent \
  --from-literal ANTHROPIC_API_KEY=$ANTHROPIC_API_KEY

kubectl create secret generic kagent-openai -n kagent \
  --from-literal OPENAI_API_KEY=$OPENAI_API_KEY
```

This creates several problems:

- **No rotation**: The key stays static until someone manually rotates it.
- **No audit trail**: No record of when the key was created, changed, or who approved it.
- **No namespace isolation**: Nothing prevents a team from creating secrets for providers they shouldn't access.
- **No lifecycle management**: When the agent team changes, secrets are forgotten and linger.
- **Manual proliferation**: Each environment (dev, staging, prod) needs manual recreation.

---

## How llmwarden Solves This

llmwarden introduces two CRDs that model the platform/developer split:

1. **`LLMProvider`** (cluster-scoped) — platform team declares an available LLM provider and its master credentials.
2. **`LLMAccess`** (namespace-scoped) — dev team requests access to a provider for their agent workload.

llmwarden provisions the Kubernetes Secret automatically. kagent's `ModelConfig` just references the secret name — the same name it would have used if you'd created it manually.

When llmwarden rotates the credential (automatically, on schedule), it updates the secret in place. kagent never restarts; it reads the updated secret on the next API call.

---

## Step-by-Step Setup

### Step 1: Platform team creates an LLMProvider

The platform team creates a cluster-scoped `LLMProvider` resource. This declares that the `anthropic-production` provider is available and points to the master API key secret in the `llmwarden-system` namespace.

```yaml
# platform-team applies this once, cluster-wide
apiVersion: llmwarden.io/v1alpha1
kind: LLMProvider
metadata:
  name: anthropic-production
spec:
  provider: anthropic
  auth:
    type: apiKey
    apiKey:
      secretRef:
        name: anthropic-master-key        # master key lives in llmwarden-system
        namespace: llmwarden-system
        key: api-key
      rotation:
        enabled: true
        interval: 30d
  allowedModels:
    - "claude-opus-4-6"
    - "claude-sonnet-4-6"
    - "claude-haiku-4-5"
  namespaceSelector:
    matchLabels:
      llmwarden.io/ai-enabled: "true"    # only namespaces with this label can request access
```

Label the `kagent` namespace so it can request access:

```bash
kubectl label namespace kagent llmwarden.io/ai-enabled=true
```

### Step 2: Dev/agent team creates an LLMAccess

The dev team creates a namespace-scoped `LLMAccess` resource in the `kagent` namespace. The `secretName` field must match what kagent's `ModelConfig` will reference.

```yaml
# dev team applies this in the kagent namespace
apiVersion: llmwarden.io/v1alpha1
kind: LLMAccess
metadata:
  name: agent-anthropic
  namespace: kagent
spec:
  providerRef:
    name: anthropic-production
  models:
    - "claude-sonnet-4-6"
  secretName: kagent-anthropic             # llmwarden creates this secret
  workloadSelector:
    matchLabels:
      kagent.dev/agent: reasoning-agent    # pods with this label get env injection
  injection:
    env:
      - name: ANTHROPIC_API_KEY
        secretKey: apiKey
```

### Step 3: llmwarden provisions the secret

After `LLMAccess` is created, the llmwarden operator:

1. Validates the `kagent` namespace is allowed by `anthropic-production`'s `namespaceSelector`.
2. Validates that `claude-sonnet-4-6` is in `allowedModels`.
3. Creates a Kubernetes Secret named `kagent-anthropic` in the `kagent` namespace.
4. Sets an owner reference so the secret is deleted if the `LLMAccess` is deleted.

```bash
# Verify the secret was created:
kubectl get secret kagent-anthropic -n kagent
# NAME               TYPE     DATA   AGE
# kagent-anthropic   Opaque   1      5s

# Check LLMAccess status:
kubectl get llmaccess agent-anthropic -n kagent
# NAME               READY   PROVISIONED   AGE
# agent-anthropic    True    True          10s
```

### Step 4: kagent ModelConfig references the llmwarden-managed secret

```yaml
apiVersion: kagent.dev/v1alpha2
kind: ModelConfig
metadata:
  name: claude-sonnet
  namespace: kagent
spec:
  provider: Anthropic
  model: claude-sonnet-4-6
  anthropicConfig:
    apiKeySecret: kagent-anthropic         # matches LLMAccess.spec.secretName
    apiKeySecretKey: apiKey                # matches LLMAccess.spec.injection.env[0].secretKey
```

### Step 5: kagent Agent references the ModelConfig

```yaml
apiVersion: kagent.dev/v1alpha2
kind: Agent
metadata:
  name: reasoning-agent
  namespace: kagent
spec:
  systemPrompt: "You are a helpful assistant."
  modelConfigRef:
    name: claude-sonnet
  tools: []
```

---

## Complete Working Example

The following three resources together form a complete, working integration. Apply them in order.

```yaml
---
# 1. LLMProvider (cluster-scoped) — platform team applies once
apiVersion: llmwarden.io/v1alpha1
kind: LLMProvider
metadata:
  name: anthropic-production
spec:
  provider: anthropic
  auth:
    type: apiKey
    apiKey:
      secretRef:
        name: anthropic-master-key
        namespace: llmwarden-system
        key: api-key
      rotation:
        enabled: true
        interval: 30d
  allowedModels:
    - "claude-opus-4-6"
    - "claude-sonnet-4-6"
    - "claude-haiku-4-5"
  namespaceSelector:
    matchLabels:
      llmwarden.io/ai-enabled: "true"
---
# 2. LLMAccess (namespace-scoped) — dev team applies in kagent namespace
apiVersion: llmwarden.io/v1alpha1
kind: LLMAccess
metadata:
  name: agent-anthropic
  namespace: kagent
spec:
  providerRef:
    name: anthropic-production
  models:
    - "claude-sonnet-4-6"
  secretName: kagent-anthropic
  workloadSelector:
    matchLabels:
      kagent.dev/agent: reasoning-agent
  injection:
    env:
      - name: ANTHROPIC_API_KEY
        secretKey: apiKey
---
# 3. kagent ModelConfig — references the secret llmwarden creates
apiVersion: kagent.dev/v1alpha2
kind: ModelConfig
metadata:
  name: claude-sonnet
  namespace: kagent
spec:
  provider: Anthropic
  model: claude-sonnet-4-6
  anthropicConfig:
    apiKeySecret: kagent-anthropic
    apiKeySecretKey: apiKey
---
# 4. kagent Agent — references the ModelConfig
apiVersion: kagent.dev/v1alpha2
kind: Agent
metadata:
  name: reasoning-agent
  namespace: kagent
spec:
  systemPrompt: "You are a helpful assistant that can reason about complex topics."
  modelConfigRef:
    name: claude-sonnet
  tools: []
```

---

## Multi-Model Agent Example

Many agent architectures use different models for different tasks — a fast cheap model for classification, a powerful model for reasoning, a specialized model for embeddings. Here's how to configure a kagent agent that needs both OpenAI and Anthropic:

```yaml
---
# OpenAI provider (cluster-scoped)
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
  allowedModels:
    - "gpt-4o"
    - "gpt-4o-mini"
    - "text-embedding-3-large"
  namespaceSelector:
    matchLabels:
      llmwarden.io/ai-enabled: "true"
---
# Anthropic provider (cluster-scoped)
apiVersion: llmwarden.io/v1alpha1
kind: LLMProvider
metadata:
  name: anthropic-production
spec:
  provider: anthropic
  auth:
    type: apiKey
    apiKey:
      secretRef:
        name: anthropic-master-key
        namespace: llmwarden-system
        key: api-key
      rotation:
        enabled: true
        interval: 30d
  allowedModels:
    - "claude-opus-4-6"
    - "claude-sonnet-4-6"
  namespaceSelector:
    matchLabels:
      llmwarden.io/ai-enabled: "true"
---
# LLMAccess for OpenAI (embeddings)
apiVersion: llmwarden.io/v1alpha1
kind: LLMAccess
metadata:
  name: agent-openai
  namespace: kagent
spec:
  providerRef:
    name: openai-production
  models:
    - "text-embedding-3-large"
  secretName: kagent-openai
  workloadSelector:
    matchLabels:
      kagent.dev/agent: code-review-agent
  injection:
    env:
      - name: OPENAI_API_KEY
        secretKey: apiKey
---
# LLMAccess for Anthropic (reasoning)
apiVersion: llmwarden.io/v1alpha1
kind: LLMAccess
metadata:
  name: agent-anthropic
  namespace: kagent
spec:
  providerRef:
    name: anthropic-production
  models:
    - "claude-opus-4-6"
  secretName: kagent-anthropic
  workloadSelector:
    matchLabels:
      kagent.dev/agent: code-review-agent
  injection:
    env:
      - name: ANTHROPIC_API_KEY
        secretKey: apiKey
---
# ModelConfig for OpenAI embeddings
apiVersion: kagent.dev/v1alpha2
kind: ModelConfig
metadata:
  name: gpt4o-embeddings
  namespace: kagent
spec:
  provider: OpenAI
  model: text-embedding-3-large
  openAIConfig:
    apiKeySecret: kagent-openai
    apiKeySecretKey: apiKey
---
# ModelConfig for Anthropic reasoning
apiVersion: kagent.dev/v1alpha2
kind: ModelConfig
metadata:
  name: claude-opus
  namespace: kagent
spec:
  provider: Anthropic
  model: claude-opus-4-6
  anthropicConfig:
    apiKeySecret: kagent-anthropic
    apiKeySecretKey: apiKey
---
# Agent that uses both models
apiVersion: kagent.dev/v1alpha2
kind: Agent
metadata:
  name: code-review-agent
  namespace: kagent
spec:
  systemPrompt: |
    You are a code review expert. Use embeddings for similarity search,
    then Claude Opus for deep reasoning and detailed review.
  modelConfigRef:
    name: claude-opus                      # primary reasoning model
  tools: []
```

llmwarden manages both secrets (`kagent-openai` and `kagent-anthropic`) independently. Each rotates on its own schedule. The agent sees no disruption during rotation.

---

## Benefits Summary

| Benefit | Without llmwarden | With llmwarden |
|---------|------------------|----------------|
| **Credential rotation** | Manual, requires human action | Automatic, no agent restart required |
| **Namespace isolation** | Anyone with kubectl can create secrets | Platform team controls which namespaces access which providers |
| **Model scoping** | No restrictions; agent could call any model | `allowedModels` enforces access control |
| **Audit trail** | No record of secret creation or changes | K8s events for every provision, rotation, and error |
| **Multi-environment** | 4 secrets to create manually per environment | One `LLMAccess` per environment, automatic provisioning |
| **Rotation without downtime** | Secret swap requires pod restart | llmwarden updates secret in place; no restart needed |

### Rotation Without Restarts

When llmwarden rotates a credential, it updates the Kubernetes Secret data in place. Kubernetes propagates the updated secret to all mounted volumes automatically (within the `kubelet` sync period, typically 60 seconds). Pods do not need to restart.

For env var injection: the env var value is fixed at pod start time. The llmwarden webhook injects credentials at pod creation. If your agent does not cache the API key at startup and instead reads it from the secret volume, use `injection.volume` in the `LLMAccess` spec for live rotation support.

### Namespace Isolation

The `namespaceSelector` on `LLMProvider` ensures that only labeled namespaces can create `LLMAccess` resources against that provider. A `staging` agent cannot request production credentials even if a developer tries to create the `LLMAccess` in the wrong namespace.

### Model Scoping

The `allowedModels` list on `LLMProvider` and the `models` list on `LLMAccess` enforce a two-layer access control. The platform team controls which models are available; the dev team requests a subset. A cost-sensitive agent restricted to `claude-haiku-4-5` cannot accidentally use `claude-opus-4-6`.

---

## See Also

- [examples/kagent/](../../examples/kagent/) — Ready-to-apply YAML files for this guide
- [docs/architecture.md](../architecture.md) — llmwarden architecture and CRD specs
- [kagent documentation](https://github.com/kagent-dev/kagent) — kagent CRDs and getting started