# Development Scripts

This directory contains helper scripts for llmwarden development.

## setup-local-webhooks.sh

Sets up self-signed TLS certificates and webhook configurations for running the operator locally with webhooks enabled.

### Usage

```bash
make setup-webhooks
```

This script:
1. Generates self-signed TLS certificates for the webhook server
2. Creates webhook configuration manifests
3. Sets up a service to route webhook traffic to your local machine

### ⚠️ Important Notes

**Local webhook development is complex and NOT recommended for most use cases.**

The script will generate the necessary certificates and configurations, but you'll still need to:

1. Configure your kind cluster to allow the API server to reach your local machine (`host.docker.internal`)
2. Ensure port 9443 is accessible from within the kind cluster
3. Apply the generated webhook configurations manually

### Why is this complex?

- The Kubernetes API server (running in kind) needs to make HTTPS calls to your local machine
- Docker networking requires special configuration to allow container → host communication
- Self-signed certificates require manual CA bundle management
- No automatic certificate rotation

### Better Alternative

For most development scenarios, **deploying to the cluster** is simpler and faster:

```bash
make deploy-local IMG=llmwarden:dev
```

This approach:
- ✅ Handles all TLS certificates automatically via cert-manager
- ✅ Provides full webhook functionality
- ✅ Matches production environment exactly
- ✅ Only takes ~10-30 seconds to rebuild and redeploy

### When to use local webhooks

Only use local webhook setup when:
- You're actively developing webhook mutation/validation logic
- You need to debug webhook code with breakpoints (using Delve)
- You're making frequent changes to webhook handlers and need instant feedback
- You understand Kubernetes networking and TLS certificate management

### Troubleshooting

If you run into issues:

1. **Webhook server won't start**
   - Check that certificates exist: `ls tmp/webhook-certs/`
   - Verify certificate validity: `openssl x509 -in tmp/webhook-certs/tls.crt -text -noout`

2. **API server can't reach webhook**
   - Check kind cluster networking: `docker inspect llmwarden-dev-control-plane`
   - Verify host.docker.internal resolves from inside the cluster
   - Check firewall rules allowing port 9443

3. **Certificate errors**
   - Regenerate certificates: `rm -rf tmp/webhook-certs && make setup-webhooks`
   - Ensure CA bundle in webhook config matches certificate

4. **Still having issues?**
   - Use `make deploy-local` instead - it's much more reliable
   - Check the [local-development.md](../docs/local-development.md) guide
   - File an issue if you think there's a bug in the setup script

## Other Scripts

(Placeholder for future development scripts)
