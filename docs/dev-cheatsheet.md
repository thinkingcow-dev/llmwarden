# Developer Cheat Sheet

Quick reference for common llmwarden development tasks.

## kind Cluster Management

```bash
# Create cluster
kind create cluster --name llmwarden-dev

# Delete cluster
kind delete cluster --name llmwarden-dev

# List clusters
kind get clusters

# Load image into cluster
kind load docker-image llmwarden:dev --name llmwarden-dev

# Get kubeconfig
kind get kubeconfig --name llmwarden-dev
```

## Development Workflow

```bash
# Generate code after CRD changes
make generate

# Generate manifests after CRD changes
make manifests

# Install CRDs into cluster
make install

# Uninstall CRDs
make uninstall

# Run operator locally
make run

# Run tests
make test

# Run tests with coverage
make test
go tool cover -html=cover.out

# Build Docker image
make docker-build IMG=llmwarden:dev

# Deploy to cluster
make deploy IMG=llmwarden:dev

# Undeploy from cluster
make undeploy

# Run linters
make lint

# Format code
make fmt

# Vet code
make vet
```

## kubectl Quick Commands

### CRD Operations

```bash
# List all CRDs
kubectl get crds | grep llmwarden

# Explain CRD fields
kubectl explain llmprovider.spec
kubectl explain llmaccess.spec.injection

# Get API resources
kubectl api-resources | grep llmwarden
```

### LLMProvider Operations

```bash
# List providers
kubectl get llmproviders
kubectl get llmp  # short name

# Describe provider
kubectl describe llmprovider openai-production

# Get provider YAML
kubectl get llmprovider openai-production -o yaml

# Watch providers
kubectl get llmproviders --watch

# Delete provider
kubectl delete llmprovider openai-production
```

### LLMAccess Operations

```bash
# List all LLMAccess in all namespaces
kubectl get llmaccesses -A
kubectl get llma -A  # short name

# List in specific namespace
kubectl get llmaccess -n chatbot-dev

# Describe access
kubectl describe llmaccess chatbot-openai -n chatbot-dev

# Get access status
kubectl get llmaccess chatbot-openai -n chatbot-dev -o jsonpath='{.status.conditions[*].type}'

# Watch access
kubectl get llmaccess -n chatbot-dev --watch

# Delete access
kubectl delete llmaccess chatbot-openai -n chatbot-dev
```

### Secret Operations

```bash
# List secrets in namespace
kubectl get secrets -n chatbot-dev

# Describe secret (doesn't show values)
kubectl describe secret openai-credentials -n chatbot-dev

# Get secret keys
kubectl get secret openai-credentials -n chatbot-dev -o jsonpath='{.data}' | jq 'keys'

# Decode secret value (use carefully!)
kubectl get secret openai-credentials -n chatbot-dev -o jsonpath='{.data.apiKey}' | base64 -d

# Watch secrets
kubectl get secrets -n chatbot-dev --watch
```

### Pod Operations

```bash
# List pods with labels
kubectl get pods -A -l llmwarden.io/injected=true

# Get pod with env vars
kubectl get pod <pod-name> -n <namespace> -o jsonpath='{.spec.containers[*].env}' | jq

# Check pod annotations
kubectl get pod <pod-name> -n <namespace> -o jsonpath='{.metadata.annotations}' | jq

# Execute command in pod
kubectl exec -it <pod-name> -n <namespace> -- env | grep OPENAI_API_KEY

# View pod logs
kubectl logs <pod-name> -n <namespace>
kubectl logs <pod-name> -n <namespace> -f  # follow
```

## Debugging

### Operator Logs

```bash
# Local run (make run output)
# Just check terminal where you ran 'make run'

# In-cluster deployment
kubectl logs -n llmwarden-system deployment/llmwarden-controller-manager -c manager
kubectl logs -n llmwarden-system deployment/llmwarden-controller-manager -c manager -f  # follow
kubectl logs -n llmwarden-system deployment/llmwarden-controller-manager -c manager --tail=100

# All operator pods
kubectl logs -n llmwarden-system -l control-plane=controller-manager --all-containers=true

# Previous container logs (if crashed)
kubectl logs -n llmwarden-system deployment/llmwarden-controller-manager -c manager --previous
```

### Events

```bash
# All events in namespace
kubectl get events -n chatbot-dev

# Sorted by time
kubectl get events -n chatbot-dev --sort-by='.lastTimestamp'

# Watch events
kubectl get events -n chatbot-dev --watch

# Events for specific resource
kubectl describe llmaccess chatbot-openai -n chatbot-dev | grep Events: -A 10
```

### Webhook Debugging

```bash
# List webhook configurations
kubectl get mutatingwebhookconfigurations | grep llmwarden
kubectl get validatingwebhookconfigurations | grep llmwarden

# Describe webhook config
kubectl describe mutatingwebhookconfiguration llmwarden-mutating-webhook-configuration

# Check webhook service
kubectl get svc -n llmwarden-system

# Check webhook endpoint
kubectl get endpoints -n llmwarden-system

# Test webhook (create test pod)
kubectl run test-webhook --image=nginx -n chatbot-dev --labels="app=ai-chatbot"
kubectl get pod test-webhook -n chatbot-dev -o yaml | grep -A 5 "env:"
kubectl delete pod test-webhook -n chatbot-dev
```

### Metrics

```bash
# Port-forward to metrics endpoint
kubectl port-forward -n llmwarden-system svc/llmwarden-controller-manager-metrics 8080:8443

# Query metrics
curl http://localhost:8080/metrics
curl http://localhost:8080/metrics | grep llmwarden

# Specific metric
curl http://localhost:8080/metrics | grep llmwarden_llmaccess_total
```

## Testing Scenarios

### Test Provider Creation

```bash
# Create master secret
kubectl create secret generic openai-test-key \
  -n llmwarden-system \
  --from-literal=api-key=sk-test-123

# Apply provider
cat <<EOF | kubectl apply -f -
apiVersion: llmwarden.io/v1alpha1
kind: LLMProvider
metadata:
  name: test-provider
spec:
  provider: openai
  auth:
    type: apiKey
    apiKey:
      secretRef:
        name: openai-test-key
        namespace: llmwarden-system
        key: api-key
  namespaceSelector:
    matchLabels:
      test: "true"
EOF

# Verify
kubectl get llmprovider test-provider
kubectl describe llmprovider test-provider
```

### Test Access Request

```bash
# Create and label namespace
kubectl create namespace test-ns
kubectl label namespace test-ns test=true

# Request access
cat <<EOF | kubectl apply -f -
apiVersion: llmwarden.io/v1alpha1
kind: LLMAccess
metadata:
  name: test-access
  namespace: test-ns
spec:
  providerRef:
    name: test-provider
  models: ["gpt-4o"]
  secretName: test-credentials
  workloadSelector:
    matchLabels:
      app: test-app
  injection:
    env:
      - name: OPENAI_API_KEY
        secretKey: apiKey
EOF

# Verify secret created
kubectl get secret test-credentials -n test-ns
```

### Test Injection

```bash
# Create test pod
kubectl run test-app --image=busybox -n test-ns --labels="app=test-app" \
  --command -- sh -c "echo Key: \$OPENAI_API_KEY && sleep 3600"

# Check injection
kubectl get pod test-app -n test-ns -o jsonpath='{.metadata.annotations}' | jq
kubectl logs test-app -n test-ns
kubectl exec test-app -n test-ns -- env | grep OPENAI_API_KEY

# Cleanup
kubectl delete pod test-app -n test-ns
```

### Cleanup Test Resources

```bash
kubectl delete llmaccess test-access -n test-ns
kubectl delete llmprovider test-provider
kubectl delete namespace test-ns
kubectl delete secret openai-test-key -n llmwarden-system
```

## Common Issue Fixes

### Restart Operator

```bash
# If running locally with 'make run'
# Ctrl+C and run again

# If deployed in cluster
kubectl rollout restart deployment llmwarden-controller-manager -n llmwarden-system

# Watch rollout
kubectl rollout status deployment llmwarden-controller-manager -n llmwarden-system
```

### Reinstall CRDs After Schema Change

```bash
# Uninstall old CRDs
make uninstall

# Or manually
kubectl delete crd llmproviders.llmwarden.io
kubectl delete crd llmaccesses.llmwarden.io

# Regenerate and install
make generate manifests install
```

### Force Reconciliation

```bash
# Annotate to trigger reconcile
kubectl annotate llmaccess chatbot-openai -n chatbot-dev reconcile.llmwarden.io/force="$(date +%s)"

# Or delete and recreate
kubectl get llmaccess chatbot-openai -n chatbot-dev -o yaml > /tmp/access.yaml
kubectl delete llmaccess chatbot-openai -n chatbot-dev
kubectl apply -f /tmp/access.yaml
```

### Clear Stuck Finalizers

```bash
# View finalizers
kubectl get llmaccess chatbot-openai -n chatbot-dev -o jsonpath='{.metadata.finalizers}'

# Remove finalizer (use carefully!)
kubectl patch llmaccess chatbot-openai -n chatbot-dev \
  -p '{"metadata":{"finalizers":null}}' --type=merge
```

## Useful Aliases

Add to your `~/.bashrc` or `~/.zshrc`:

```bash
# kubectl aliases
alias k='kubectl'
alias kgp='kubectl get pods'
alias kgs='kubectl get svc'
alias kgn='kubectl get nodes'
alias kd='kubectl describe'
alias kdel='kubectl delete'
alias kl='kubectl logs'
alias kx='kubectl exec -it'

# llmwarden specific
alias kgllmp='kubectl get llmproviders'
alias kgllma='kubectl get llmaccesses -A'
alias kdllmp='kubectl describe llmprovider'
alias kdllma='kubectl describe llmaccess'

# Operator logs
alias llmwarden-logs='kubectl logs -n llmwarden-system deployment/llmwarden-controller-manager -c manager -f'

# Quick test
alias llmwarden-test='cd /path/to/llmwarden && make test'
```

## Environment Variables

Useful when developing:

```bash
# Use specific kubeconfig
export KUBECONFIG=~/.kube/kind-llmwarden-dev

# Disable webhook in local development
export ENABLE_WEBHOOKS=false

# Enable debug logging
export LOG_LEVEL=debug

# Custom image
export IMG=llmwarden:dev
```

## Go Testing

```bash
# Run all tests
go test ./...

# Run tests in specific package
go test ./internal/controller
go test ./internal/provisioner

# Run specific test
go test ./internal/controller -run TestLLMAccessReconcile

# Verbose output
go test -v ./internal/controller

# With coverage
go test -cover ./...
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out -o coverage.html

# Run race detector
go test -race ./...

# Short mode (skip long tests)
go test -short ./...

# Clean test cache
go clean -testcache
```

## Git Workflow

```bash
# Create feature branch
git checkout -b feature/my-feature

# Stage changes
git add .

# Commit with conventional commit
git commit -m "feat: add new provisioner for X"
git commit -m "fix: resolve secret creation bug"
git commit -m "docs: update getting started guide"

# Push branch
git push origin feature/my-feature

# Update from main
git fetch origin
git rebase origin/main
```

## Documentation

```bash
# Generate Go docs
godoc -http=:6060
# Visit http://localhost:6060

# View CRD documentation
kubectl explain llmprovider
kubectl explain llmprovider.spec
kubectl explain llmprovider.spec.auth
kubectl explain llmprovider.spec.auth.apiKey
kubectl explain llmaccess.spec.injection
```

## Performance Profiling

```bash
# CPU profile
go test -cpuprofile=cpu.prof ./internal/controller
go tool pprof cpu.prof

# Memory profile
go test -memprofile=mem.prof ./internal/controller
go tool pprof mem.prof

# Benchmark
go test -bench=. ./internal/controller
go test -bench=. -benchmem ./internal/controller
```

## Quick Validation

```bash
# Validate manifests
kubectl apply --dry-run=client -f config/samples/

# Validate with server
kubectl apply --dry-run=server -f config/samples/

# Diff changes
kubectl diff -f config/samples/llmprovider-openai.yaml

# Validate CRD schema
kubectl apply --validate=true -f config/crd/bases/
```

## References

- [Getting Started](./getting-started.md)
- [Local Development](./local-development.md)
- [Architecture](./architecture.md)
- [Kubebuilder Docs](https://book.kubebuilder.io/)
- [controller-runtime](https://pkg.go.dev/sigs.k8s.io/controller-runtime)
