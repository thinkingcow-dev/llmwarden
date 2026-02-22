# kagent Integration Examples

These files demonstrate how to use llmwarden to manage credentials for [kagent](https://github.com/kagent-dev/kagent) agents.

## Prerequisites

- llmwarden operator installed (`helm install llmwarden charts/llmwarden -n llmwarden-system --create-namespace`)
- kagent operator installed (see kagent docs)
- `kagent` namespace exists

## Apply in Order

### 1. Create master key secrets (platform team, one-time)

```bash
# Replace with your actual keys — do NOT commit real keys
kubectl create secret generic anthropic-master-key \
  -n llmwarden-system \
  --from-literal=api-key=sk-ant-api03-REPLACE-WITH-REAL-KEY

kubectl create secret generic openai-master-key \
  -n llmwarden-system \
  --from-literal=api-key=sk-REPLACE-WITH-REAL-KEY
```

### 2. Label the kagent namespace

```bash
kubectl label namespace kagent llmwarden.io/ai-enabled=true
```

### 3. Apply LLMProviders (platform team, cluster-scoped)

```bash
kubectl apply -f llmprovider-anthropic.yaml
kubectl apply -f llmprovider-openai.yaml
```

### 4. Apply LLMAccess resources (dev team, namespace-scoped)

```bash
kubectl apply -f llmaccess-agent-anthropic.yaml
kubectl apply -f llmaccess-agent-openai.yaml
```

### 5. Verify secrets were provisioned

```bash
kubectl get llmaccess -n kagent
# NAME               READY   PROVISIONED   AGE
# agent-anthropic    True    True          10s
# agent-openai       True    True          10s

kubectl get secret kagent-anthropic kagent-openai -n kagent
# NAME               TYPE     DATA   AGE
# kagent-anthropic   Opaque   1      10s
# kagent-openai      Opaque   1      10s
```

### 6. Apply kagent resources

```bash
kubectl apply -f modelconfig-claude.yaml
kubectl apply -f modelconfig-gpt4o.yaml
kubectl apply -f agent-example.yaml
```

## File Reference

| File | Description |
|------|-------------|
| `llmprovider-anthropic.yaml` | LLMProvider for Anthropic Claude (cluster-scoped, platform team) |
| `llmprovider-openai.yaml` | LLMProvider for OpenAI GPT-4o (cluster-scoped, platform team) |
| `llmaccess-agent-anthropic.yaml` | LLMAccess requesting Anthropic credentials in `kagent` namespace |
| `llmaccess-agent-openai.yaml` | LLMAccess requesting OpenAI credentials in `kagent` namespace |
| `modelconfig-claude.yaml` | kagent ModelConfig referencing the llmwarden-managed Anthropic secret |
| `modelconfig-gpt4o.yaml` | kagent ModelConfig referencing the llmwarden-managed OpenAI secret |
| `agent-example.yaml` | kagent Agent using both ModelConfigs |

## How It Works

```
Platform team applies LLMProvider (once per provider)
    ↓
Dev team applies LLMAccess (once per agent namespace)
    ↓
llmwarden operator provisions K8s Secret automatically
    ↓
kagent ModelConfig references the secret by name
    ↓
llmwarden rotates the secret on schedule (no agent restart needed)
```

## Full Guide

See [docs/guides/kagent-integration.md](../../docs/guides/kagent-integration.md) for the complete integration guide including multi-model agent setup, rotation behavior, and namespace isolation details.
