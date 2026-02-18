# Contributing to llmwarden

Thanks for your interest in contributing to llmwarden.

## Before You Start

**Open an issue first.** For anything beyond typo fixes or documentation corrections, please open a GitHub issue describing what you want to change and why. This saves everyone time by ensuring your approach aligns with the project direction before you write code.

## Developer Certificate of Origin (DCO)

All commits must be signed off to certify the [Developer Certificate of Origin](https://developercertificate.org/):

```bash
git commit -s -m "your commit message"
```

This adds a `Signed-off-by: Your Name <your@email.com>` line to your commit. It certifies that you wrote the code or have the right to submit it under the Apache 2.0 license. Unsigned commits will not be merged.

If you forgot to sign off, you can amend:

```bash
git commit --amend -s --no-edit
```

## Development Setup

### Prerequisites

- Go 1.23+
- kubectl
- A local K8s cluster ([kind](https://kind.sigs.k8s.io/) recommended)
- [cert-manager](https://cert-manager.io/docs/installation/) installed in your cluster (for webhook TLS)

### Build and Test

```bash
# Clone
git clone https://github.com/thinkingcow-dev/llmwarden.git
cd llmwarden

# Generate CRDs and deepcopy
make generate
make manifests

# Run tests
make test

# Run locally against your cluster
make install   # install CRDs
make run       # start controller
```

### Code Style

- Follow standard Go conventions and Kubebuilder patterns
- Use `logr` for logging (via `log.FromContext(ctx)`)
- Wrap errors with context: `fmt.Errorf("doing thing: %w", err)`
- Write table-driven tests
- See `CLAUDE.md` for detailed coding standards

## Submitting a PR

1. Fork the repo and create a branch from `main`
2. Make your changes, keeping commits focused and atomic
3. Sign off every commit (`git commit -s`)
4. Ensure `make test` passes
5. Ensure `make generate && make manifests` produces no diff
6. Open a PR referencing the issue it addresses

## What Makes a Good Contribution

- Bug fixes with a regression test
- Tests for uncovered code paths
- New provisioner implementations (ExternalSecret, WorkloadIdentity)
- Documentation improvements
- Provider-specific rotation support (OpenAI, Anthropic, AWS admin APIs)

## Questions?

Open a GitHub issue or discussion. There's no Slack or Discord yet — GitHub is the single source of truth.
