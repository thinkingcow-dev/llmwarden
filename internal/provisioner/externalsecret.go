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
	"fmt"
	"maps"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	llmwardenv1alpha1 "github.com/thinkingcow-dev/llmwarden/api/v1alpha1"
	"github.com/thinkingcow-dev/llmwarden/internal/eso"
)

// ExternalSecretProvisioner implements the Provisioner interface for ESO-based authentication.
// It creates and manages ESO ExternalSecret resources that delegate secret synchronization
// from external stores (HashiCorp Vault, AWS Secrets Manager, GCP Secret Manager, etc.)
// into namespace-scoped Kubernetes Secrets.
//
// The adapter field decouples this provisioner from specific ESO API versions.
// Swap the adapter to target a different ESO API version without changing provisioner logic.
type ExternalSecretProvisioner struct {
	client  client.Client
	scheme  *runtime.Scheme
	adapter eso.Adapter
}

// NewExternalSecretProvisioner creates a new ExternalSecretProvisioner with the given ESO adapter.
// Use eso.NewV1Beta1Adapter() for production; inject a test adapter in unit tests.
func NewExternalSecretProvisioner(k8sClient client.Client, scheme *runtime.Scheme, adapter eso.Adapter) *ExternalSecretProvisioner {
	return &ExternalSecretProvisioner{
		client:  k8sClient,
		scheme:  scheme,
		adapter: adapter,
	}
}

// Provision creates or updates an ESO ExternalSecret that will sync credentials from the
// external store referenced in the LLMProvider's externalSecret config.
// The ExternalSecret is owned by the LLMAccess resource for automatic garbage collection.
func (p *ExternalSecretProvisioner) Provision(ctx context.Context, provider *llmwardenv1alpha1.LLMProvider, access *llmwardenv1alpha1.LLMAccess) (*ProvisionResult, error) {
	if provider.Spec.Auth.ExternalSecret == nil {
		return nil, fmt.Errorf("provider %s does not have externalSecret configuration", provider.Name)
	}

	esoConfig := provider.Spec.Auth.ExternalSecret

	// Determine the effective refresh interval:
	// LLMAccess rotation.interval takes precedence over the provider's refreshInterval.
	refreshInterval := p.effectiveRefreshInterval(access, esoConfig.RefreshInterval)

	// Build our internal ExternalSecret spec from the provider + access config.
	spec := eso.ExternalSecretSpec{
		RefreshInterval: refreshInterval,
		StoreRef: eso.StoreRef{
			Name: esoConfig.Store.Name,
			Kind: string(esoConfig.Store.Kind),
		},
		// The target secret name is driven by what LLMAccess declared it wants.
		Target: eso.ExternalSecretTarget{
			Name: access.Spec.SecretName,
			// "Owner" means the ExternalSecret owns the resulting Secret.
			// The Secret is deleted when the ExternalSecret is deleted.
			CreationPolicy: eso.SecretCreationPolicyOwner,
		},
		Data: []eso.ExternalSecretData{
			{
				// We expose the credential under the standard "apiKey" key so the
				// rest of the injection pipeline (webhook env var mapping) remains uniform.
				SecretKey: "apiKey",
				RemoteRef: eso.RemoteRef{
					Key:      esoConfig.RemoteRef.Key,
					Property: esoConfig.RemoteRef.Property,
				},
			},
		},
	}

	labels := p.standardLabels(provider, access)

	// ExternalSecret name matches the target secret name so it's easy to find.
	esName := access.Spec.SecretName

	// Use CreateOrUpdate so Provision is idempotent.
	existing := &unstructured.Unstructured{}
	existing.SetGroupVersionKind(p.adapter.GVK())
	existing.SetNamespace(access.Namespace)
	existing.SetName(esName)

	_, err := controllerutil.CreateOrUpdate(ctx, p.client, existing, func() error {
		// Build the desired spec from our adapter.
		desired := p.adapter.Build(access.Namespace, esName, labels, spec)

		// Preserve any existing annotations/labels set by other controllers,
		// then apply our labels on top.
		existingLabels := existing.GetLabels()
		if existingLabels == nil {
			existingLabels = make(map[string]string)
		}
		maps.Copy(existingLabels, labels)
		existing.SetLabels(existingLabels)

		// Apply spec from the desired object built by the adapter.
		existing.Object["spec"] = desired.Object["spec"]

		// Set owner reference so the ExternalSecret is garbage-collected when
		// the LLMAccess is deleted, and changes to the ExternalSecret trigger
		// reconciliation of the owning LLMAccess.
		return controllerutil.SetControllerReference(access, existing, p.scheme)
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create/update ExternalSecret %s/%s: %w", access.Namespace, esName, err)
	}

	// Read back sync status so we can surface it in the result metadata.
	syncStatus := p.adapter.ParseSyncStatus(existing)

	return &ProvisionResult{
		SecretName:      access.Spec.SecretName,
		SecretNamespace: access.Namespace,
		// The actual keys in the resulting Secret depend on ESO syncing.
		// We report "apiKey" as the expected key per our spec.
		SecretKeys:    []string{"apiKey"},
		ProvisionedAt: time.Now(),
		// ESO manages refresh via refreshInterval; we don't need additional rotation.
		NeedsRotation: false,
		Metadata: map[string]string{
			"provider":        provider.Name,
			"providerType":    string(provider.Spec.Provider),
			"authType":        string(provider.Spec.Auth.Type),
			"store":           esoConfig.Store.Name,
			"storeKind":       string(esoConfig.Store.Kind),
			"refreshInterval": refreshInterval,
			"syncReady":       fmt.Sprintf("%v", syncStatus.Ready),
			"syncMessage":     syncStatus.Message,
		},
	}, nil
}

// Cleanup deletes the ESO ExternalSecret created for the LLMAccess.
// The resulting Kubernetes Secret will also be deleted because the ExternalSecret
// uses CreationPolicy=Owner.
// Note: owner references handle cleanup automatically on LLMAccess deletion,
// but this method provides explicit cleanup when switching auth strategies.
func (p *ExternalSecretProvisioner) Cleanup(ctx context.Context, _ *llmwardenv1alpha1.LLMProvider, access *llmwardenv1alpha1.LLMAccess) error {
	esObj := &unstructured.Unstructured{}
	esObj.SetGroupVersionKind(p.adapter.GVK())
	esObj.SetNamespace(access.Namespace)
	esObj.SetName(access.Spec.SecretName)

	err := p.client.Delete(ctx, esObj)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil // Already deleted — idempotent
		}
		return fmt.Errorf("failed to delete ExternalSecret %s/%s: %w", access.Namespace, access.Spec.SecretName, err)
	}
	return nil
}

// HealthCheck reports whether the ESO ExternalSecret exists and has successfully synced.
// ESO reports sync status via status conditions on the ExternalSecret resource.
func (p *ExternalSecretProvisioner) HealthCheck(ctx context.Context, _ *llmwardenv1alpha1.LLMProvider, access *llmwardenv1alpha1.LLMAccess) (*HealthCheckResult, error) {
	result := &HealthCheckResult{
		LastChecked: time.Now(),
		Metadata:    make(map[string]string),
	}

	esObj := &unstructured.Unstructured{}
	esObj.SetGroupVersionKind(p.adapter.GVK())

	err := p.client.Get(ctx, types.NamespacedName{
		Namespace: access.Namespace,
		Name:      access.Spec.SecretName,
	}, esObj)
	if err != nil {
		if apierrors.IsNotFound(err) {
			result.Healthy = false
			result.Message = "ExternalSecret not found"
			return result, nil
		}
		return nil, fmt.Errorf("failed to get ExternalSecret %s/%s: %w", access.Namespace, access.Spec.SecretName, err)
	}

	syncStatus := p.adapter.ParseSyncStatus(esObj)
	result.Healthy = syncStatus.Ready
	result.Message = syncStatus.Message
	result.Metadata["syncReady"] = fmt.Sprintf("%v", syncStatus.Ready)

	if !syncStatus.Ready {
		result.Warnings = append(result.Warnings,
			fmt.Sprintf("ExternalSecret not yet synced by ESO: %s", syncStatus.Message))
	}

	return result, nil
}

// effectiveRefreshInterval returns the refresh interval to use for the ExternalSecret.
// LLMAccess.spec.rotation.interval takes precedence over the provider's refreshInterval.
// This is the "rotation policy passthrough" — we translate our rotation config into
// ESO's native refreshInterval mechanism.
func (p *ExternalSecretProvisioner) effectiveRefreshInterval(access *llmwardenv1alpha1.LLMAccess, providerInterval string) string {
	if access.Spec.Rotation != nil && access.Spec.Rotation.Interval != "" {
		return access.Spec.Rotation.Interval
	}
	if providerInterval != "" {
		return providerInterval
	}
	return "1h" // ESO default
}

// standardLabels returns the set of labels applied to all ExternalSecrets managed by llmwarden.
func (p *ExternalSecretProvisioner) standardLabels(provider *llmwardenv1alpha1.LLMProvider, access *llmwardenv1alpha1.LLMAccess) map[string]string {
	return map[string]string{
		"llmwarden.io/managed-by": "llmwarden",
		"llmwarden.io/provider":   provider.Name,
		"llmwarden.io/access":     access.Name,
		"llmwarden.io/auth-type":  string(provider.Spec.Auth.Type),
	}
}
