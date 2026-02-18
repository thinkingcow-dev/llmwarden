# Local Development Guide

This guide shows you how to develop and test llmwarden locally using kind (Kubernetes in Docker) with a sample AI-powered chat application.

> **⚠️ Important:** llmwarden's core feature is **webhook-based credential injection**. To test the full functionality, you must deploy the operator **inside the cluster**, not run it locally. The recommended command is `make deploy-local IMG=llmwarden:dev`.

## Quick Start: Which Development Mode?

| Feature | Deploy to Cluster | Run Locally (No Webhooks) | Run Locally WITH Webhooks |
|---------|------------------|---------------------------|---------------------------|
| **Command** | `make deploy-local` | `make run` | `make run-with-webhooks` |
| **Setup Time** | ~30 seconds | ~5 seconds | ~5 minutes (one-time) |
| **Iteration Speed** | 10-30s rebuild | Instant (Ctrl+C, restart) | Instant (Ctrl+C, restart) |
| **Webhook Injection** | ✅ Yes | ❌ No | ✅ Yes (complex setup) |
| **Controller Logic** | ✅ Yes | ✅ Yes | ✅ Yes |
| **Debugger Support** | ❌ No | ✅ Yes (Delve) | ✅ Yes (Delve) |
| **TLS Certificate Management** | ✅ Auto (cert-manager) | N/A | ⚠️ Manual (self-signed) |
| **Production Parity** | ✅ Exact match | ❌ Partial | ⚠️ Close but not exact |
| **Best For** | Full integration testing | Controller development | Webhook debugging |
| **Recommended For** | ✅ Most developers | Controller-only changes | Advanced webhook dev |

**Our recommendation:** Start with **Deploy to Cluster** (Option 1) for your first time. Use **Run Locally** (Option 2) once you're familiar and only working on controller logic.

## Table of Contents

- [Prerequisites](#prerequisites)
- [Setting Up kind Cluster](#setting-up-kind-cluster)
- [Installing llmwarden Locally](#installing-llmwarden-locally)
  - [Option 1: Deploy to Cluster (Recommended)](#option-1-deploy-operator-in-cluster-recommended---full-feature-testing)
  - [Option 2: Run Locally (Fast Iteration, No Webhooks)](#option-2-run-operator-locally-fast-iteration-no-webhooks)
  - [Option 3: Run Locally WITH Webhooks (Advanced)](#option-3-run-operator-locally-with-webhooks-advanced)
- [Viewing Operator Logs](#viewing-operator-logs)
- [Deploying a Sample AI Application](#deploying-a-sample-ai-application)
- [Testing the Integration](#testing-the-integration)
- [Development Workflow](#development-workflow)
- [Debugging](#debugging)

## Prerequisites

Install the following tools:

1. **Docker Desktop** or **Docker Engine**
   ```bash
   docker --version
   # Should output: Docker version 20.10.0 or higher
   ```

2. **kind** (Kubernetes in Docker)
   ```bash
   # macOS
   brew install kind

   # Linux
   curl -Lo ./kind https://kind.sigs.k8s.io/dl/v0.20.0/kind-linux-amd64
   chmod +x ./kind
   sudo mv ./kind /usr/local/bin/kind

   # Verify
   kind --version
   ```

3. **kubectl**
   ```bash
   # macOS
   brew install kubectl

   # Linux
   curl -LO "https://dl.k8s.io/release/$(curl -L -s https://dl.k8s.io/release/stable.txt)/bin/linux/amd64/kubectl"
   chmod +x kubectl
   sudo mv kubectl /usr/local/bin/

   # Verify
   kubectl version --client
   ```

4. **Go 1.23+** (for building the operator)
   ```bash
   go version
   # Should output: go version go1.23.0 or higher
   ```

5. **Make**
   ```bash
   make --version
   ```

## Setting Up kind Cluster

### Create a kind Cluster

Create a kind cluster with proper configuration for webhook support:

```bash
# Create a cluster config file
cat > kind-config.yaml <<EOF
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
name: llmwarden-dev
nodes:
- role: control-plane
  extraPortMappings:
  - containerPort: 30080
    hostPort: 8080
    protocol: TCP
  - containerPort: 30443
    hostPort: 8443
    protocol: TCP
EOF

# Create the cluster
kind create cluster --config ./examples/kind-config.yaml
```

**Expected output:**
```
Creating cluster "llmwarden-dev" ...
 ✓ Ensuring node image (kindest/node:v1.27.3) 🖼
 ✓ Preparing nodes 📦
 ✓ Writing configuration 📜
 ✓ Starting control-plane 🕹️
 ✓ Installing CNI 🔌
 ✓ Installing StorageClass 💾
Set kubectl context to "kind-llmwarden-dev"
```

Verify the cluster:

```bash
kubectl cluster-info --context kind-llmwarden-dev
kubectl get nodes
```

**Expected output:**
```
NAME                           STATUS   ROLES           AGE   VERSION
llmwarden-dev-control-plane    Ready    control-plane   1m    v1.27.3
```

### Install cert-manager

llmwarden webhooks require TLS certificates. Install cert-manager:

```bash
kubectl apply -f https://github.com/cert-manager/cert-manager/releases/download/v1.18.5/cert-manager.yaml

# Wait for cert-manager to be ready
kubectl wait --for=condition=ready pod -l app.kubernetes.io/instance=cert-manager -n cert-manager --timeout=300s
```

**Expected output:**
```
namespace/cert-manager created
customresourcedefinition.apiextensions.k8s.io/certificates.cert-manager.io created
...
pod/cert-manager-webhook-xxxxx condition met
```

## Installing llmwarden Locally

### Option 1: Deploy Operator in Cluster (Recommended - Full Feature Testing)

**This is the recommended approach** because it enables webhooks, which are essential for llmwarden's credential injection feature.

```bash
# Navigate to the project directory
cd /path/to/llmwarden

# Build, load, and deploy operator to kind cluster (single command)
make deploy-local IMG=llmwarden:dev
```

This command will:
1. Build the Docker image with your latest code
2. Load the image into the kind cluster
3. Deploy the operator with webhooks enabled
4. Configure TLS certificates via cert-manager

**Expected output:**
```
docker build -t llmwarden:dev .
Loading image into kind cluster...
kind load docker-image llmwarden:dev --name llmwarden-dev
Deploying operator...
namespace/llmwarden-system configured
deployment.apps/llmwarden-controller-manager created
mutatingwebhookconfiguration.admissionregistration.k8s.io/llmwarden-mutating-webhook-configuration created
...
```

Verify the operator is running:

```bash
kubectl get pods -n llmwarden-system
kubectl wait --for=condition=ready pod -l control-plane=controller-manager -n llmwarden-system --timeout=120s
```

**Expected output:**
```
NAME                                            READY   STATUS    RESTARTS   AGE
llmwarden-controller-manager-6f67c5bc76-xxxxx   1/1     Running   0          30s
```

### Option 2: Run Operator Locally (Fast Iteration, No Webhooks)

**Use this only for testing controller logic without webhook injection.** Webhooks are disabled in this mode.

```bash
# Install CRDs first
make install

# Run the operator on your local machine
make run
```

**⚠️ Important:** With this approach, the mutating webhook will NOT inject credentials into pods. You'll need to manually add environment variables to test credential provisioning, or use Option 1 for full integration testing.

**When to use this:**
- Testing LLMProvider/LLMAccess controller reconciliation logic
- Testing secret provisioning (Secret creation/updates)
- Debugging controller errors without webhook complexity
- Quick iteration on controller code changes

### Option 3: Run Operator Locally WITH Webhooks (Advanced)

**This is an advanced option** that enables webhooks while running locally. It's complex to set up and **not recommended** unless you're specifically developing webhook logic and need breakpoint debugging.

```bash
# One-time setup: Generate webhook certificates and configuration
make setup-webhooks

# Run the operator with webhooks enabled
make run-with-webhooks
```

**Prerequisites:**
- You must configure your kind cluster to allow API server → localhost communication
- This requires special network configuration (see script output for details)
- TLS certificates are self-signed and not managed by cert-manager

**When to use this:**
- Active development of webhook mutation/validation logic
- Need to debug webhook code with breakpoints (Delve)
- Testing webhook-specific edge cases

**Why it's complex:**
- Kubernetes API server (in kind) must reach your local machine (9443 port)
- Requires manual webhook configuration with self-signed certs
- No automatic certificate rotation
- Network setup varies by OS/Docker configuration

**For most webhook development, Option 1 (deploy-local) is faster and more reliable.**

## Viewing Operator Logs

Once the operator is deployed to the cluster, you can view its logs:

```bash
# View operator logs
kubectl logs -n llmwarden-system -l control-plane=controller-manager -f

# Or view recent logs
kubectl logs -n llmwarden-system -l control-plane=controller-manager --tail=50
```

**Expected log output:**
```
2025-01-15T10:00:00Z	INFO	setup	starting manager
2025-01-15T10:00:00Z	INFO	controller-runtime.metrics	Starting metrics server
2025-01-15T10:00:00Z	INFO	controller-runtime.webhook	Starting webhook server
2025-01-15T10:00:00Z	INFO	LLMProvider reconciled	{"name": "openai-dev"}
2025-01-15T10:00:00Z	INFO	LLMAccess reconciled	{"namespace": "chatbot-dev", "name": "chatbot-access"}
```

## Deploying a Sample AI Application

Let's deploy a realistic AI chat application that uses OpenAI's API.

### Step 1: Create Test API Key Secret

```bash
# Create llmwarden-system namespace
kubectl create namespace llmwarden-system

# Create a test OpenAI API key secret
# Replace 'sk-test-...' with your actual OpenAI API key
kubectl create secret generic openai-master-key \
  -n llmwarden-system \
  --from-literal=api-key=sk-test-your-actual-key-here

# Or use a dummy key for testing (will fail API calls but demonstrates the flow)
kubectl create secret generic openai-master-key \
  -n llmwarden-system \
  --from-literal=api-key=sk-dummy-key-for-local-testing-only
```

### Step 2: Create LLMProvider

Create `examples/local-dev/llmprovider.yaml`:

```yaml
apiVersion: llmwarden.io/v1alpha1
kind: LLMProvider
metadata:
  name: openai-dev
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
    - "gpt-3.5-turbo"
  namespaceSelector:
    matchLabels:
      llmwarden.io/ai-enabled: "true"
```

Apply it:

```bash
mkdir -p examples/local-dev
# Save the YAML above to examples/local-dev/llmprovider.yaml
kubectl apply -f examples/local-dev/llmprovider.yaml
```

Check the operator logs (in the first terminal) for reconciliation messages:

```
2025-01-15T10:00:00Z	INFO	LLMProvider reconciled	{"name": "openai-dev"}
```

### Step 3: Create Application Namespace

```bash
# Create namespace for the sample app
kubectl create namespace chatbot-dev

# Label it to match the provider's namespaceSelector
kubectl label namespace chatbot-dev llmwarden.io/ai-enabled=true
```

### Step 4: Create LLMAccess

Create `examples/local-dev/llmaccess.yaml`:

```yaml
apiVersion: llmwarden.io/v1alpha1
kind: LLMAccess
metadata:
  name: chatbot-access
  namespace: chatbot-dev
spec:
  providerRef:
    name: openai-dev
  models:
    - "gpt-4o-mini"
  secretName: openai-api-credentials
  workloadSelector:
    matchLabels:
      app: ai-chatbot
  injection:
    env:
      - name: OPENAI_API_KEY
        secretKey: apiKey
```

Apply it:

```bash
kubectl apply -f examples/local-dev/llmaccess.yaml
```

Verify the secret was created:

```bash
kubectl get secret openai-api-credentials -n chatbot-dev
kubectl describe llmaccess chatbot-access -n chatbot-dev
```

### Step 5: Deploy Sample Chat Application

Create `examples/local-dev/chatbot-app.yaml`:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: chatbot-script
  namespace: chatbot-dev
data:
  chat.py: |
    #!/usr/bin/env python3
    import os
    import json
    from http.server import HTTPServer, BaseHTTPRequestHandler

    API_KEY = os.getenv('OPENAI_API_KEY', 'not-set')

    class ChatHandler(BaseHTTPRequestHandler):
        def do_GET(self):
            if self.path == '/health':
                self.send_response(200)
                self.send_header('Content-type', 'application/json')
                self.end_headers()
                response = {
                    'status': 'healthy',
                    'api_key_configured': API_KEY != 'not-set',
                    'api_key_prefix': API_KEY[:10] if API_KEY != 'not-set' else 'none'
                }
                self.wfile.write(json.dumps(response).encode())
            elif self.path == '/':
                self.send_response(200)
                self.send_header('Content-type', 'text/html')
                self.end_headers()
                html = f"""
                <html>
                <head><title>AI Chatbot</title></head>
                <body>
                    <h1>AI Chatbot Demo</h1>
                    <p>Status: {'✅ Configured' if API_KEY != 'not-set' else '❌ Not Configured'}</p>
                    <p>API Key: {API_KEY[:15]}... (injected by llmwarden)</p>
                    <p>Model: gpt-4o-mini</p>
                    <hr>
                    <p>This is a demo app showing credential injection.</p>
                    <p>Check <a href="/health">/health</a> for JSON status.</p>
                </body>
                </html>
                """
                self.wfile.write(html.encode())
            else:
                self.send_response(404)
                self.end_headers()

    if __name__ == '__main__':
        port = 8080
        print(f'Starting chatbot server on port {port}...')
        print(f'API Key configured: {API_KEY != "not-set"}')
        server = HTTPServer(('0.0.0.0', port), ChatHandler)
        server.serve_forever()
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: ai-chatbot
  namespace: chatbot-dev
spec:
  replicas: 1
  selector:
    matchLabels:
      app: ai-chatbot
  template:
    metadata:
      labels:
        app: ai-chatbot
    spec:
      containers:
      - name: chatbot
        image: python:3.11-slim
        command: ["python3", "/app/chat.py"]
        ports:
        - containerPort: 8080
          name: http
        volumeMounts:
        - name: script
          mountPath: /app
        # Note: OPENAI_API_KEY will be injected by llmwarden webhook
        livenessProbe:
          httpGet:
            path: /health
            port: 8080
          initialDelaySeconds: 10
          periodSeconds: 10
        readinessProbe:
          httpGet:
            path: /health
            port: 8080
          initialDelaySeconds: 5
          periodSeconds: 5
      volumes:
      - name: script
        configMap:
          name: chatbot-script
          defaultMode: 0755
---
apiVersion: v1
kind: Service
metadata:
  name: ai-chatbot
  namespace: chatbot-dev
spec:
  type: NodePort
  selector:
    app: ai-chatbot
  ports:
  - port: 8080
    targetPort: 8080
    nodePort: 30080
    protocol: TCP
```

Apply it:

```bash
kubectl apply -f examples/local-dev/chatbot-app.yaml
```

Wait for the pod to be ready:

```bash
kubectl wait --for=condition=ready pod -l app=ai-chatbot -n chatbot-dev --timeout=60s
```

## Testing the Integration

### Verify Credential Injection

Check the pod to see if the environment variable was injected:

```bash
# Get the pod name
POD=$(kubectl get pod -n chatbot-dev -l app=ai-chatbot -o jsonpath='{.items[0].metadata.name}')

# Check environment variables
kubectl exec -n chatbot-dev $POD -- env | grep OPENAI_API_KEY

# Check pod annotations (should show llmwarden injection)
kubectl get pod $POD -n chatbot-dev -o jsonpath='{.metadata.annotations}' | jq
```

**Expected output:**
```bash
OPENAI_API_KEY=sk-test-...

{
  "llmwarden.io/injected": "true",
  "llmwarden.io/injected-providers": "openai-dev"
}
```

### Access the Sample Application

Since we're using kind with NodePort, access the app via localhost:

```bash
# Check the service
kubectl get svc ai-chatbot -n chatbot-dev

# Access via browser or curl
curl http://localhost:8080/

# Check health endpoint
curl http://localhost:8080/health | jq
```

**Expected output:**
```json
{
  "status": "healthy",
  "api_key_configured": true,
  "api_key_prefix": "sk-test-12"
}
```

Or open in browser: http://localhost:8080/

### Check Operator Logs

In the terminal where you ran `make run`, you should see:

```
2025-01-15T10:00:00Z	INFO	LLMAccess reconciled	{"namespace": "chatbot-dev", "name": "chatbot-access"}
2025-01-15T10:00:00Z	INFO	Secret created	{"namespace": "chatbot-dev", "secret": "openai-api-credentials"}
2025-01-15T10:00:00Z	INFO	Pod injection webhook called	{"namespace": "chatbot-dev", "pod": "ai-chatbot-xxx"}
```

## Development Workflow

Choose your workflow based on what you're developing:

### Workflow A: Full Integration Testing (Recommended First-Time)

**Best for:** Testing complete functionality, webhook changes, first-time setup

1. **Edit code** in `internal/controller/`, `internal/provisioner/`, `internal/webhook/`, or `api/v1alpha1/`

2. **Regenerate code** if you changed CRD types:
   ```bash
   make generate manifests
   ```

3. **Rebuild and redeploy** to test your changes:
   ```bash
   make deploy-local IMG=llmwarden:dev
   ```

4. **Wait for new pod to be ready**:
   ```bash
   kubectl rollout status deployment/llmwarden-controller-manager -n llmwarden-system
   ```

5. **Test your changes**:
   ```bash
   # For webhook changes: Delete and recreate test pods
   kubectl delete pod -n chatbot-dev -l app=ai-chatbot
   kubectl wait --for=condition=ready pod -l app=ai-chatbot -n chatbot-dev --timeout=60s

   # For controller changes: Delete and recreate LLMAccess
   kubectl delete llmaccess chatbot-access -n chatbot-dev
   kubectl apply -f examples/local-dev/llmaccess.yaml

   # Watch the operator logs
   kubectl logs -n llmwarden-system -l control-plane=controller-manager -f
   ```

**Iteration time:** ~10-30 seconds per change

### Workflow B: Fast Controller Iteration (No Webhooks)

**Best for:** Controller reconciliation logic, secret provisioning, status updates

1. **One-time setup:**
   ```bash
   make install  # Install CRDs
   ```

2. **Start the operator locally:**
   ```bash
   make run
   ```
   Keep this running in a terminal.

3. **In another terminal, edit code** in `internal/controller/` or `internal/provisioner/`

4. **Restart the operator:**
   - Press Ctrl+C in the first terminal
   - Run `make run` again

5. **Test your changes:**
   ```bash
   # Trigger controller reconciliation
   kubectl delete llmaccess chatbot-access -n chatbot-dev
   kubectl apply -f examples/local-dev/llmaccess.yaml

   # Verify the secret was created/updated
   kubectl get secret openai-api-credentials -n chatbot-dev
   kubectl describe llmaccess chatbot-access -n chatbot-dev
   ```

**Iteration time:** ~2-5 seconds per change

**⚠️ Limitation:** Webhook injection won't work. Pods won't have `OPENAI_API_KEY` injected. To test end-to-end, switch to Workflow A.

### Workflow C: Advanced Webhook Debugging (With Delve)

**Best for:** Debugging webhook mutation logic with breakpoints

1. **One-time setup:**
   ```bash
   make setup-webhooks
   # Follow script instructions to configure kind networking
   ```

2. **Install delve:**
   ```bash
   go install github.com/go-delve/delve/cmd/dlv@latest
   ```

3. **Run operator with debugger:**
   ```bash
   dlv debug ./cmd/main.go -- --webhook-cert-path=tmp/webhook-certs
   ```

4. **Set breakpoints in webhook code:**
   ```
   (dlv) break internal/webhook/v1alpha1/pod_injector.go:HandlePodMutation
   (dlv) continue
   ```

5. **Trigger webhook in another terminal:**
   ```bash
   kubectl delete pod -n chatbot-dev -l app=ai-chatbot
   # Debugger will pause at breakpoint when pod is created
   ```

**Iteration time:** Instant (just restart debugger)

**⚠️ Complexity:** Requires understanding of TLS certificates and kind networking.

### Development Tips

**Quick Reference:**

| What You're Working On | Use This Workflow | Why |
|------------------------|-------------------|-----|
| Adding a new feature | Workflow A | Need to test end-to-end |
| Fixing a controller bug | Workflow B → A | Quick iteration, then validate |
| Changing webhook logic | Workflow A | Need webhook to run |
| Debugging webhook with breakpoints | Workflow C | Need debugger access |
| Updating CRD schema | Workflow B or A | Either works |
| Testing secret rotation | Workflow B | Controllers only, no webhooks needed |

**Pro Tips:**

1. **Start with Workflow A** - Get the full system working first
2. **Switch to Workflow B** - Once familiar, use for faster controller iterations
3. **Validate with Workflow A** - Before committing, always test full integration
4. **Use Workflow C sparingly** - Only when you truly need breakpoint debugging

**Common mistakes:**
- ❌ Using `make run` and expecting webhook injection to work
- ❌ Forgetting to run `make install` after changing CRDs
- ❌ Not waiting for pod rollout after `make deploy-local`
- ✅ Use the comparison table at the top to choose the right workflow

### Running Tests

```bash
# Run unit tests
make test

# Run tests with coverage
make test
go tool cover -html=cover.out

# Run specific test
go test -v ./internal/controller -run TestLLMAccessReconcile

# Run e2e tests
make test-e2e
```

## Debugging

### Debug Operator Locally with Delve

```bash
# Install delve
go install github.com/go-delve/delve/cmd/dlv@latest

# Run operator with debugger
dlv debug ./cmd/main.go
```

Set breakpoints and step through code:
```
(dlv) break internal/controller/llmaccess_controller.go:120
(dlv) continue
```

### Common Issues

#### Issue: Webhook Connection Refused

**Symptom:**
```
Error: failed calling webhook: connection refused
```

**Solution:**
Webhooks only work when the operator runs in-cluster. Deploy using `make deploy` instead of `make run`.

#### Issue: CRD Validation Failed

**Symptom:**
```
error: error validating "llmaccess.yaml": error validating data: ValidationError...
```

**Solution:**
```bash
# Reinstall CRDs with latest schema
make install

# Or manually update
kubectl apply -f config/crd/bases/
```

#### Issue: Image Pull Error in kind

**Symptom:**
```
Failed to pull image "llmwarden:dev": rpc error: image not found
```

**Solution:**
```bash
# Rebuild and reload image
make docker-build IMG=llmwarden:dev
kind load docker-image llmwarden:dev --name llmwarden-dev

# Restart the pod
kubectl rollout restart deployment llmwarden-controller-manager -n llmwarden-system
```

#### Issue: Secret Not Created

**Check operator logs:**
```bash
kubectl logs -n llmwarden-system -l control-plane=controller-manager --tail=100
```

**Check LLMAccess status:**
```bash
kubectl describe llmaccess chatbot-access -n chatbot-dev
```

Look for error conditions in the status section.

### Useful Debugging Commands

```bash
# Watch all resources
kubectl get llmproviders,llmaccesses,secrets,pods -A --watch

# Describe everything
kubectl describe llmprovider openai-dev
kubectl describe llmaccess chatbot-access -n chatbot-dev
kubectl describe pod -n chatbot-dev -l app=ai-chatbot

# Get events
kubectl get events -n chatbot-dev --sort-by='.lastTimestamp'

# Check controller logs (if running in cluster)
kubectl logs -n llmwarden-system deployment/llmwarden-controller-manager -f

# Port-forward to metrics
kubectl port-forward -n llmwarden-system svc/llmwarden-controller-manager-metrics 8080:8443
curl http://localhost:8080/metrics | grep llmwarden
```

## Cleanup

### Clean Up Test Resources

```bash
# Delete the sample app
kubectl delete -f examples/local-dev/chatbot-app.yaml

# Delete LLMAccess
kubectl delete -f examples/local-dev/llmaccess.yaml

# Delete LLMProvider
kubectl delete -f examples/local-dev/llmprovider.yaml

# Delete namespace
kubectl delete namespace chatbot-dev
```

### Uninstall llmwarden

If running locally:
```bash
# Stop the operator (Ctrl+C)
# Uninstall CRDs
make uninstall
```

If deployed in cluster:
```bash
make undeploy
```

### Delete kind Cluster

```bash
kind delete cluster --name llmwarden-dev
```

## Next Steps

- **Read the architecture:** [architecture.md](./architecture.md)
- **Explore advanced auth methods:** Workload identity, ExternalSecrets
- **Add more providers:** AWS Bedrock, Anthropic, Azure OpenAI
- **Build real applications:** Integrate with LangChain, LlamaIndex, etc.
- **Contribute:** See [CONTRIBUTING.md](../CONTRIBUTING.md)

## Additional Resources

- kind documentation: https://kind.sigs.k8s.io/
- Kubebuilder book: https://book.kubebuilder.io/
- controller-runtime: https://pkg.go.dev/sigs.k8s.io/controller-runtime
- OpenAI API docs: https://platform.openai.com/docs/api-reference
