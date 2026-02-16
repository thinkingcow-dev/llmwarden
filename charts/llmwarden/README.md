# llmwarden Helm Chart

A Kubernetes operator that makes LLM provider access declarative, secretless, and auditable.

## Prerequisites

- Kubernetes 1.25+
- Helm 3.8+
- cert-manager 1.12+ (if using webhook with certificate management)

## Installing the Chart

To install the chart with the release name `llmwarden`:

```bash
helm install llmwarden ./charts/llmwarden -n llmwarden-system --create-namespace
```

## Uninstalling the Chart

To uninstall/delete the `llmwarden` deployment:

```bash
helm uninstall llmwarden -n llmwarden-system
```

The command removes all the Kubernetes components associated with the chart and deletes the release. CRDs are preserved by default.

## Configuration

The following table lists the configurable parameters of the llmwarden chart and their default values.

### General Parameters

| Parameter | Description | Default |
|-----------|-------------|---------|
| `replicaCount` | Number of replicas for the controller | `1` |
| `image.repository` | Image repository | `ghcr.io/tpbansal/llmwarden` |
| `image.pullPolicy` | Image pull policy | `IfNotPresent` |
| `image.tag` | Overrides the image tag | `""` (uses appVersion) |
| `imagePullSecrets` | Image pull secrets | `[]` |
| `nameOverride` | Override the name of the chart | `""` |
| `fullnameOverride` | Override the full name of the release | `""` |

### ServiceAccount Parameters

| Parameter | Description | Default |
|-----------|-------------|---------|
| `serviceAccount.create` | Specifies whether a service account should be created | `true` |
| `serviceAccount.automount` | Automatically mount a ServiceAccount's API credentials | `true` |
| `serviceAccount.annotations` | Annotations to add to the service account | `{}` |
| `serviceAccount.name` | The name of the service account to use | `""` |

### RBAC Parameters

| Parameter | Description | Default |
|-----------|-------------|---------|
| `rbac.create` | Specifies whether RBAC resources should be created | `true` |

### Controller Parameters

| Parameter | Description | Default |
|-----------|-------------|---------|
| `controller.leaderElection.enabled` | Enable leader election for high availability | `true` |
| `controller.healthProbeBindAddress` | Health probe bind address | `":8081"` |
| `controller.metricsBindAddress` | Metrics bind address | `":8080"` |

### Webhook Parameters

| Parameter | Description | Default |
|-----------|-------------|---------|
| `webhook.enabled` | Enable webhook | `true` |
| `webhook.service.type` | Service type for webhook | `ClusterIP` |
| `webhook.service.port` | Service port | `443` |
| `webhook.certificate.enabled` | Use cert-manager to generate certificates | `true` |
| `webhook.certificate.issuer` | Certificate issuer name | `""` (creates self-signed) |
| `webhook.certificate.issuerKind` | Certificate issuer kind | `ClusterIssuer` |
| `webhook.pod.enabled` | Enable pod mutation webhook | `true` |
| `webhook.pod.failurePolicy` | Failure policy for pod webhook | `Ignore` |
| `webhook.llmaccess.enabled` | Enable LLMAccess validation webhook | `true` |
| `webhook.llmaccess.failurePolicy` | Failure policy for LLMAccess webhook | `Fail` |

### Metrics Parameters

| Parameter | Description | Default |
|-----------|-------------|---------|
| `metrics.enabled` | Enable metrics service | `true` |
| `metrics.serviceMonitor.enabled` | Create ServiceMonitor for Prometheus Operator | `false` |
| `metrics.serviceMonitor.interval` | Scrape interval | `30s` |
| `metrics.serviceMonitor.scrapeTimeout` | Scrape timeout | `10s` |

### CRD Parameters

| Parameter | Description | Default |
|-----------|-------------|---------|
| `crds.install` | Install CRDs as part of the chart installation | `true` |
| `crds.keep` | Keep CRDs on chart uninstall | `true` |

### Resource Parameters

| Parameter | Description | Default |
|-----------|-------------|---------|
| `resources.limits.cpu` | CPU limit | `500m` |
| `resources.limits.memory` | Memory limit | `128Mi` |
| `resources.requests.cpu` | CPU request | `10m` |
| `resources.requests.memory` | Memory request | `64Mi` |

### Other Parameters

| Parameter | Description | Default |
|-----------|-------------|---------|
| `nodeSelector` | Node selector for pod assignment | `{}` |
| `tolerations` | Tolerations for pod assignment | `[]` |
| `affinity` | Affinity for pod assignment | `{}` |
| `logging.level` | Log level | `info` |
| `logging.format` | Log format | `json` |

## Examples

### Basic Installation

```bash
helm install llmwarden ./charts/llmwarden -n llmwarden-system --create-namespace
```

### With Prometheus ServiceMonitor

```bash
helm install llmwarden ./charts/llmwarden \
  -n llmwarden-system \
  --create-namespace \
  --set metrics.serviceMonitor.enabled=true
```

### With Custom cert-manager Issuer

```bash
helm install llmwarden ./charts/llmwarden \
  -n llmwarden-system \
  --create-namespace \
  --set webhook.certificate.issuer=letsencrypt-prod \
  --set webhook.certificate.issuerKind=ClusterIssuer
```

### High Availability Setup

```bash
helm install llmwarden ./charts/llmwarden \
  -n llmwarden-system \
  --create-namespace \
  --set replicaCount=3 \
  --set controller.leaderElection.enabled=true
```

## Values File Example

```yaml
replicaCount: 2

image:
  repository: ghcr.io/tpbansal/llmwarden
  tag: "v0.1.0"

resources:
  limits:
    cpu: 1000m
    memory: 256Mi
  requests:
    cpu: 100m
    memory: 128Mi

metrics:
  serviceMonitor:
    enabled: true
    interval: 15s

webhook:
  certificate:
    issuer: letsencrypt-prod
    issuerKind: ClusterIssuer

nodeSelector:
  kubernetes.io/os: linux
```

## License

Apache 2.0
