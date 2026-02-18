# Getting Started with llmwarden

This guide walks you through installing llmwarden and setting up your first LLM provider integration in a Kubernetes cluster.

## Table of Contents

- [Prerequisites](#prerequisites)
- [Installation](#installation)
- [Creating Your First LLM Provider](#creating-your-first-llm-provider)
- [Requesting Access from a Workload](#requesting-access-from-a-workload)
- [Verifying the Integration](#verifying-the-integration)
- [Understanding the Flow](#understanding-the-flow)
- [Troubleshooting](#troubleshooting)
- [Next Steps](#next-steps)

> **💡 For local development:** If you want to test llmwarden on your local machine with a kind cluster and sample AI application, see the [Local Development Guide](./local-development.md).

## Prerequisites

Before you begin, ensure you have:

1. **Kubernetes cluster** (v1.25+)
   - Local: kind, minikube, or Docker Desktop
   - Cloud: EKS, GKE, AKS, or any standard K8s cluster

2. **kubectl** configured to access your cluster
   ```bash
   kubectl version --short
   kubectl cluster-info
   ```

3. **Helm 3** (optional, for Helm-based installation)
   ```bash
   helm version
   ```

4. **An LLM provider API key** (for this guide, we'll use OpenAI)
   - OpenAI: Get your API key from [platform.openai.com](https://platform.openai.com/api-keys)
   - Anthropic: Get your API key from [console.anthropic.com](https://console.anthropic.com/)
   - AWS Bedrock: Ensure you have IAM permissions configured

## Installation

llmwarden can be installed using either Helm or raw manifests via kubectl.

### Option 1: Install via Helm (Recommended)

```bash
# Create the llmwarden-system namespace
kubectl create namespace llmwarden-system

# Install llmwarden using Helm
helm install llmwarden charts/llmwarden \
  -n llmwarden-system \
  --create-namespace
```

**Expected output:**
```
NAME: llmwarden
LAST DEPLOYED: [timestamp]
NAMESPACE: llmwarden-system
STATUS: deployed
REVISION: 1
TEST SUITE: None
```

### Option 2: Install via kubectl

```bash
# Install the CRDs
kubectl apply -f config/crd/

# Install RBAC, operator deployment, and webhooks
kubectl apply -k config/default/
```

### Verify Installation

Check that the llmwarden operator is running:

```bash
kubectl get pods -n llmwarden-system
```

**Expected output:**
```
NAME                                    READY   STATUS    RESTARTS   AGE
llmwarden-controller-manager-xxxxx-xxx  2/2     Running   0          30s
```

Verify the CRDs are installed:

```bash
kubectl get crds | grep llmwarden
```

**Expected output:**
```
llmaccesses.llmwarden.io         2025-01-15T10:00:00Z
llmproviders.llmwarden.io        2025-01-15T10:00:00Z
```

## Creating Your First LLM Provider

The platform/infrastructure team creates `LLMProvider` resources to declare which LLM providers are available in the cluster.

### Step 1: Create the Master API Key Secret

First, store your LLM provider's API key in a Kubernetes secret in the `llmwarden-system` namespace:

```bash
kubectl create secret generic openai-master-key \
  -n llmwarden-system \
  --from-literal=api-key=sk-your-actual-openai-key-here
```

**Expected output:**
```
secret/openai-master-key created
```

**Security note:** This master key stays in the `llmwarden-system` namespace. llmwarden will create derived secrets in application namespaces.

### Step 2: Create an LLMProvider Resource

Create a file named `openai-provider.yaml`:

```yaml
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
        enabled: false
  allowedModels:
    - "gpt-4o"
    - "gpt-4o-mini"
    - "gpt-4-turbo"
  namespaceSelector:
    matchLabels:
      ai-tier: production
```

Apply the provider:

```bash
kubectl apply -f openai-provider.yaml
```

**Expected output:**
```
llmprovider.llmwarden.io/openai-production created
```

### Step 3: Verify the Provider

Check the provider status:

```bash
kubectl get llmprovider openai-production
```

**Expected output:**
```
NAME                PROVIDER   READY   AGE
openai-production   openai     True    30s
```

For detailed status information:

```bash
kubectl describe llmprovider openai-production
```

**Expected output should include:**
```
Status:
  Conditions:
    Type:    Ready
    Status:  True
    Reason:  ProviderConfigured
    Message: Provider configured successfully
  Access Count: 0
```

## Requesting Access from a Workload

Development teams create `LLMAccess` resources in their namespaces to request access to an LLM provider.

### Step 1: Prepare the Namespace

Create and label a namespace for your application:

```bash
# Create the namespace
kubectl create namespace customer-facing

# Label it to match the provider's namespaceSelector
kubectl label namespace customer-facing ai-tier=production
```

**Expected output:**
```
namespace/customer-facing created
namespace/customer-facing labeled
```

**Note:** The `ai-tier=production` label is required because our `openai-production` provider has a `namespaceSelector` that matches this label.

### Step 2: Create an LLMAccess Resource

Create a file named `chatbot-access.yaml`:

```yaml
apiVersion: llmwarden.io/v1alpha1
kind: LLMAccess
metadata:
  name: chatbot-openai
  namespace: customer-facing
spec:
  providerRef:
    name: openai-production
  models:
    - "gpt-4o"
  secretName: openai-credentials
  workloadSelector:
    matchLabels:
      app: chatbot-api
  injection:
    env:
      - name: OPENAI_API_KEY
        secretKey: apiKey
```

Apply the access request:

```bash
kubectl apply -f chatbot-access.yaml
```

**Expected output:**
```
llmaccess.llmwarden.io/chatbot-openai created
```

### Step 3: Verify the Access

Check the LLMAccess status:

```bash
kubectl get llmaccess -n customer-facing
```

**Expected output:**
```
NAME              PROVIDER            READY   AGE
chatbot-openai    openai-production   True    15s
```

Verify the secret was created:

```bash
kubectl get secret openai-credentials -n customer-facing
```

**Expected output:**
```
NAME                  TYPE     DATA   AGE
openai-credentials    Opaque   1      20s
```

View the secret keys (without exposing values):

```bash
kubectl describe secret openai-credentials -n customer-facing
```

**Expected output:**
```
Name:         openai-credentials
Namespace:    customer-facing
Labels:       llmwarden.io/managed-by=llmwarden
              llmwarden.io/provider=openai-production
Annotations:  <none>

Type:  Opaque

Data
====
apiKey:  51 bytes
```

## Verifying the Integration

### Step 4: Deploy a Test Application

Create a simple pod that uses the injected credentials. Create `test-pod.yaml`:

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: chatbot-api-test
  namespace: customer-facing
  labels:
    app: chatbot-api
spec:
  containers:
  - name: app
    image: busybox:1.36
    command: ["sh", "-c", "echo API Key present: $OPENAI_API_KEY | head -c 50 && sleep 3600"]
    # Note: The OPENAI_API_KEY env var will be injected by llmwarden webhook
```

Deploy the test pod:

```bash
kubectl apply -f test-pod.yaml
```

**Expected output:**
```
pod/chatbot-api-test created
```

### Step 5: Verify Credential Injection

Check that the environment variable was injected:

```bash
kubectl logs chatbot-api-test -n customer-facing
```

**Expected output:**
```
API Key present: sk-...
```

Inspect the pod to see the injected environment variable:

```bash
kubectl get pod chatbot-api-test -n customer-facing -o jsonpath='{.spec.containers[0].env}' | jq
```

**Expected output:**
```json
[
  {
    "name": "OPENAI_API_KEY",
    "valueFrom": {
      "secretKeyRef": {
        "key": "apiKey",
        "name": "openai-credentials"
      }
    }
  }
]
```

Check for the injection annotation:

```bash
kubectl get pod chatbot-api-test -n customer-facing -o jsonpath='{.metadata.annotations}' | jq
```

**Expected output should include:**
```json
{
  "llmwarden.io/injected": "true",
  "llmwarden.io/injected-providers": "openai-production"
}
```

## Understanding the Flow

Here's what happened when you created the LLMAccess:

```
1. LLMAccess created in customer-facing namespace
   └─> LLMAccess controller reconciles

2. Controller validates access:
   ├─> Checks LLMProvider exists (openai-production) ✓
   ├─> Validates namespace label matches (ai-tier=production) ✓
   └─> Validates requested models are allowed (gpt-4o) ✓

3. Controller provisions credentials:
   ├─> Reads master key from llmwarden-system/openai-master-key
   ├─> Creates customer-facing/openai-credentials secret
   └─> Sets owner reference (LLMAccess owns Secret)

4. Controller updates LLMAccess status:
   └─> Sets Ready=True, CredentialProvisioned=True

5. When pod is created with label app=chatbot-api:
   ├─> Mutating webhook intercepts Pod CREATE
   ├─> Finds matching LLMAccess (workloadSelector)
   ├─> Patches pod spec to inject OPENAI_API_KEY env var
   └─> Adds llmwarden.io/injected annotation
```

## Troubleshooting

### Problem: LLMProvider shows Ready=False

**Check the provider status:**
```bash
kubectl describe llmprovider openai-production
```

**Common causes:**
- Secret not found: Ensure `openai-master-key` exists in `llmwarden-system` namespace
- Wrong secret key: Ensure the secret has a key named `api-key` (or match `spec.auth.apiKey.secretRef.key`)

**Solution:**
```bash
# Verify secret exists
kubectl get secret openai-master-key -n llmwarden-system

# Check secret keys
kubectl describe secret openai-master-key -n llmwarden-system
```

### Problem: LLMAccess shows Ready=False with "namespace not allowed"

**Check the error message:**
```bash
kubectl describe llmaccess chatbot-openai -n customer-facing
```

**Common causes:**
- Namespace doesn't have required label

**Solution:**
```bash
# Check namespace labels
kubectl get namespace customer-facing --show-labels

# Add the required label
kubectl label namespace customer-facing ai-tier=production
```

### Problem: LLMAccess shows Ready=False with "model not allowed"

**Common causes:**
- Requested model not in provider's `allowedModels` list

**Solution:**
Edit the LLMAccess to request an allowed model, or update the LLMProvider to allow the model:
```bash
kubectl edit llmprovider openai-production
```

### Problem: Secret not created

**Check operator logs:**
```bash
kubectl logs -n llmwarden-system deployment/llmwarden-controller-manager -c manager
```

**Common causes:**
- RBAC permissions missing
- Controller not running
- Error in reconciliation loop

**Solution:**
```bash
# Verify operator is running
kubectl get pods -n llmwarden-system

# Check operator logs for errors
kubectl logs -n llmwarden-system -l control-plane=controller-manager --tail=50
```

### Problem: Environment variable not injected into pod

**Check webhook is running:**
```bash
kubectl get mutatingwebhookconfigurations | grep llmwarden
```

**Common causes:**
- Pod created before LLMAccess
- Pod labels don't match `workloadSelector`
- Webhook not configured

**Solution:**
```bash
# Verify pod labels match workloadSelector
kubectl get pod chatbot-api-test -n customer-facing --show-labels

# Delete and recreate the pod (webhooks only run on CREATE)
kubectl delete pod chatbot-api-test -n customer-facing
kubectl apply -f test-pod.yaml

# Check webhook logs
kubectl logs -n llmwarden-system deployment/llmwarden-controller-manager -c webhook
```

### Problem: "x509: certificate signed by unknown authority" webhook errors

**Common causes:**
- Webhook TLS certificates not configured
- cert-manager not installed

**Solution:**
llmwarden's webhook requires TLS certificates. Install cert-manager first:
```bash
kubectl apply -f https://github.com/cert-manager/cert-manager/releases/download/v1.14.0/cert-manager.yaml
```

Wait for cert-manager to be ready, then reinstall llmwarden.

## Next Steps

### Explore Advanced Features

1. **Try other providers:**
   - See [config/samples/llmprovider-bedrock.yaml](../config/samples/llmprovider-bedrock.yaml) for AWS Bedrock example
   - Anthropic, Azure OpenAI, and GCP Vertex AI are supported

2. **Enable rotation:**
   ```yaml
   spec:
     auth:
       apiKey:
         rotation:
           enabled: true
           interval: 30d
   ```

3. **Use workload identity (AWS IRSA, Azure WI, GCP WIF):**
   ```yaml
   spec:
     auth:
       type: workloadIdentity
       workloadIdentity:
         aws:
           roleArn: arn:aws:iam::123456789012:role/bedrock-access
           region: us-east-1
   ```

4. **Integrate with External Secrets Operator:**
   ```yaml
   spec:
     auth:
       type: externalSecret
       externalSecret:
         store:
           name: vault-backend
           kind: ClusterSecretStore
   ```

### Monitor Your Deployment

Check metrics (requires Prometheus):
```bash
kubectl port-forward -n llmwarden-system svc/llmwarden-controller-manager-metrics 8080:8443
curl http://localhost:8080/metrics | grep llmwarden
```

View events:
```bash
kubectl get events -n customer-facing --sort-by='.lastTimestamp'
```

### Audit LLM Access

List all providers:
```bash
kubectl get llmproviders
```

List all access grants:
```bash
kubectl get llmaccess -A
```

Check which workloads are using credentials:
```bash
kubectl get pods -A -l llmwarden.io/injected=true
```

### Learn More

- **Architecture details:** See [architecture.md](./architecture.md)
- **API reference:** Run `kubectl explain llmprovider.spec` or `kubectl explain llmaccess.spec`
- **CRD samples:** Browse [config/samples/](../config/samples/)
- **Project overview:** See [CLAUDE.md](../CLAUDE.md)

### Clean Up

To remove the test resources:
```bash
kubectl delete pod chatbot-api-test -n customer-facing
kubectl delete llmaccess chatbot-openai -n customer-facing
kubectl delete llmprovider openai-production
kubectl delete secret openai-master-key -n llmwarden-system
kubectl delete namespace customer-facing
```

To uninstall llmwarden:
```bash
# If installed via Helm
helm uninstall llmwarden -n llmwarden-system

# If installed via kubectl
kubectl delete -k config/default/
kubectl delete -f config/crd/

# Delete the namespace
kubectl delete namespace llmwarden-system
```

## Getting Help

- **Issues:** [GitHub Issues](https://github.com/yourusername/llmwarden/issues)
- **Discussions:** [GitHub Discussions](https://github.com/yourusername/llmwarden/discussions)
- **Documentation:** [docs/](./README.md)
