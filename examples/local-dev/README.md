# Local Development Examples

This directory contains example manifests for testing llmwarden locally with a kind cluster.

## Files

- **[llmprovider.yaml](./llmprovider.yaml)** - Sample LLMProvider for OpenAI in development
- **[llmaccess.yaml](./llmaccess.yaml)** - Sample LLMAccess requesting OpenAI credentials
- **[chatbot-app.yaml](./chatbot-app.yaml)** - Demo AI chatbot application that uses injected credentials

## Quick Start

1. Follow the [Local Development Guide](../../docs/local-development.md) to set up your kind cluster

2. Create the master API key secret:
   ```bash
   kubectl create namespace llmwarden-system
   kubectl create secret generic openai-master-key \
     -n llmwarden-system \
     --from-literal=api-key=sk-your-key-here
   ```

3. Apply the provider:
   ```bash
   kubectl apply -f llmprovider.yaml
   ```

4. Create and label the namespace:
   ```bash
   kubectl create namespace chatbot-dev
   kubectl label namespace chatbot-dev llmwarden.io/ai-enabled=true
   ```

5. Apply the access and app:
   ```bash
   kubectl apply -f llmaccess.yaml
   kubectl apply -f chatbot-app.yaml
   ```

6. Access the demo app:
   ```bash
   # Wait for pod to be ready
   kubectl wait --for=condition=ready pod -l app=ai-chatbot -n chatbot-dev --timeout=60s

   # Access via browser or curl
   curl http://localhost:8080/
   ```

## What It Demonstrates

- **LLMProvider** configuration with apiKey auth
- **LLMAccess** requesting specific models
- **Credential injection** via mutating webhook
- **Sample application** showing env var usage
- **Namespace isolation** with label-based selectors

## See Also

- [Getting Started Guide](../../docs/getting-started.md)
- [Local Development Guide](../../docs/local-development.md)
- [Architecture Documentation](../../docs/architecture.md)
