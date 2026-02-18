#!/usr/bin/env bash

# Copyright 2026.
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.

set -e

PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
WEBHOOK_DIR="${PROJECT_ROOT}/tmp/webhook-certs"
WEBHOOK_SERVICE="llmwarden-webhook-service"
WEBHOOK_NAMESPACE="llmwarden-system"

echo "=========================================="
echo "Setting up local webhook certificates"
echo "=========================================="

# Create directory for certificates
mkdir -p "${WEBHOOK_DIR}"

# Generate self-signed certificates
echo "Generating self-signed certificates..."
openssl req -x509 -newkey rsa:4096 -nodes \
  -keyout "${WEBHOOK_DIR}/tls.key" \
  -out "${WEBHOOK_DIR}/tls.crt" \
  -days 365 \
  -subj "/CN=${WEBHOOK_SERVICE}.${WEBHOOK_NAMESPACE}.svc" \
  -addext "subjectAltName=DNS:${WEBHOOK_SERVICE}.${WEBHOOK_NAMESPACE}.svc,DNS:${WEBHOOK_SERVICE}.${WEBHOOK_NAMESPACE}.svc.cluster.local,DNS:localhost,IP:127.0.0.1"

echo "✓ Certificates generated in ${WEBHOOK_DIR}"

# Get CA bundle (base64 encoded)
CA_BUNDLE=$(cat "${WEBHOOK_DIR}/tls.crt" | base64 | tr -d '\n')

# Create namespace if it doesn't exist
kubectl create namespace "${WEBHOOK_NAMESPACE}" --dry-run=client -o yaml | kubectl apply -f -

# Create a local webhook configuration for testing
cat > "${WEBHOOK_DIR}/webhook-config.yaml" <<EOF
apiVersion: admissionregistration.k8s.io/v1
kind: MutatingWebhookConfiguration
metadata:
  name: llmwarden-mutating-webhook-configuration
webhooks:
- admissionReviewVersions:
  - v1
  clientConfig:
    # For local development, we'll use a service that port-forwards to localhost
    # You'll need to run: kubectl port-forward -n llmwarden-system svc/llmwarden-webhook-service 9443:443
    # OR configure your kind cluster to reach host.docker.internal
    caBundle: ${CA_BUNDLE}
    service:
      name: ${WEBHOOK_SERVICE}
      namespace: ${WEBHOOK_NAMESPACE}
      path: /mutate-v1-pod
      port: 443
  failurePolicy: Fail
  name: pod.mutate.llmwarden.io
  rules:
  - apiGroups:
    - ""
    apiVersions:
    - v1
    operations:
    - CREATE
    resources:
    - pods
  sideEffects: None
- admissionReviewVersions:
  - v1
  clientConfig:
    caBundle: ${CA_BUNDLE}
    service:
      name: ${WEBHOOK_SERVICE}
      namespace: ${WEBHOOK_NAMESPACE}
      path: /mutate-llmwarden-io-v1alpha1-llmaccess
      port: 443
  failurePolicy: Fail
  name: mllmaccess.kb.io
  rules:
  - apiGroups:
    - llmwarden.io
    apiVersions:
    - v1alpha1
    operations:
    - CREATE
    - UPDATE
    resources:
    - llmaccesses
  sideEffects: None
EOF

echo "✓ Webhook configuration created in ${WEBHOOK_DIR}/webhook-config.yaml"

# Create a dummy service to route webhook traffic
cat > "${WEBHOOK_DIR}/webhook-service.yaml" <<EOF
apiVersion: v1
kind: Service
metadata:
  name: ${WEBHOOK_SERVICE}
  namespace: ${WEBHOOK_NAMESPACE}
spec:
  type: ExternalName
  externalName: host.docker.internal
  ports:
  - name: webhook
    port: 443
    targetPort: 9443
    protocol: TCP
EOF

echo ""
echo "=========================================="
echo "⚠️  IMPORTANT: Local webhook setup"
echo "=========================================="
echo ""
echo "Local webhook development is COMPLEX and NOT RECOMMENDED for most cases."
echo ""
echo "If you still want to proceed, you need to:"
echo ""
echo "1. Apply the webhook service:"
echo "   kubectl apply -f ${WEBHOOK_DIR}/webhook-service.yaml"
echo ""
echo "2. Apply the webhook configuration:"
echo "   kubectl apply -f ${WEBHOOK_DIR}/webhook-config.yaml"
echo ""
echo "3. Run the operator with webhook certificates:"
echo "   WEBHOOK_CERT_PATH=${WEBHOOK_DIR} make run-with-webhooks"
echo ""
echo "4. Configure your kind cluster to use host.docker.internal:"
echo "   - This may require recreating the cluster with extraPortMappings"
echo "   - See: https://kind.sigs.k8s.io/docs/user/configuration/"
echo ""
echo "=========================================="
echo "EASIER ALTERNATIVE: Deploy to cluster"
echo "=========================================="
echo ""
echo "Instead of complex local setup, use:"
echo "   make deploy-local IMG=llmwarden:dev"
echo ""
echo "This deploys to the kind cluster with proper TLS setup via cert-manager."
echo "Rebuilds take ~10-30 seconds, which is fast enough for most development."
echo ""
