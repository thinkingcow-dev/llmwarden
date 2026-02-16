/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package provisioner

import (
	"context"
	"time"

	llmwardenv1alpha1 "github.com/tpbansal/llmwarden/api/v1alpha1"
)

// Provisioner is the interface for credential provisioning strategies.
// Different implementations handle different authentication methods:
// - ApiKeyProvisioner: Copies secrets from provider namespace
// - ExternalSecretProvisioner: Creates ESO ExternalSecret resources
// - WorkloadIdentityProvisioner: Configures cloud workload identity
type Provisioner interface {
	// Provision creates or updates credentials for the given LLMAccess.
	// It should be idempotent - running multiple times produces the same result.
	// Returns a ProvisionResult with metadata about the provisioned credentials.
	Provision(ctx context.Context, provider *llmwardenv1alpha1.LLMProvider, access *llmwardenv1alpha1.LLMAccess) (*ProvisionResult, error)

	// Cleanup removes any resources created for the given LLMAccess.
	// This is called when the LLMAccess is deleted or when switching auth strategies.
	// Should be idempotent - safe to call multiple times.
	Cleanup(ctx context.Context, provider *llmwardenv1alpha1.LLMProvider, access *llmwardenv1alpha1.LLMAccess) error

	// HealthCheck validates that the provisioned credentials are still valid.
	// Returns nil error if credentials are healthy, error otherwise.
	// The HealthCheckResult contains detailed status information.
	HealthCheck(ctx context.Context, provider *llmwardenv1alpha1.LLMProvider, access *llmwardenv1alpha1.LLMAccess) (*HealthCheckResult, error)
}

// ProvisionResult contains metadata about provisioned credentials.
type ProvisionResult struct {
	// SecretName is the name of the Kubernetes Secret containing credentials
	SecretName string

	// SecretNamespace is the namespace of the Secret
	SecretNamespace string

	// SecretKeys lists the keys available in the secret
	SecretKeys []string

	// ExpiresAt indicates when the credentials expire (nil if no expiry)
	ExpiresAt *time.Time

	// NeedsRotation indicates if credentials should be rotated soon
	NeedsRotation bool

	// ProvisionedAt is when the credentials were provisioned
	ProvisionedAt time.Time

	// Metadata contains provider-specific information
	Metadata map[string]string
}

// HealthCheckResult contains health check status information.
type HealthCheckResult struct {
	// Healthy indicates if the credentials are valid and working
	Healthy bool

	// Message provides details about the health status
	Message string

	// LastChecked is when the health check was performed
	LastChecked time.Time

	// Warnings contains non-critical issues (e.g., "credentials expire soon")
	Warnings []string

	// Metadata contains provider-specific health information
	Metadata map[string]string
}
