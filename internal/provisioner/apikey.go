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
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	llmwardenv1alpha1 "github.com/tpbansal/llmwarden/api/v1alpha1"
)

// ApiKeyProvisioner implements the Provisioner interface for API key-based authentication.
// It copies credentials from a provider's master secret into namespace-scoped secrets
// for LLMAccess resources.
type ApiKeyProvisioner struct {
	client client.Client
	scheme *runtime.Scheme
}

// NewApiKeyProvisioner creates a new ApiKeyProvisioner.
func NewApiKeyProvisioner(client client.Client, scheme *runtime.Scheme) *ApiKeyProvisioner {
	return &ApiKeyProvisioner{
		client: client,
		scheme: scheme,
	}
}

// Provision creates or updates a Kubernetes Secret with credentials copied from
// the provider's master secret.
func (p *ApiKeyProvisioner) Provision(ctx context.Context, provider *llmwardenv1alpha1.LLMProvider, access *llmwardenv1alpha1.LLMAccess) (*ProvisionResult, error) {
	// Validate provider has apiKey configuration
	if provider.Spec.Auth.APIKey == nil {
		return nil, fmt.Errorf("provider %s does not have apiKey configuration", provider.Name)
	}

	// Fetch the source secret from the provider's namespace
	sourceSecret := &corev1.Secret{}
	sourceKey := types.NamespacedName{
		Name:      provider.Spec.Auth.APIKey.SecretRef.Name,
		Namespace: provider.Spec.Auth.APIKey.SecretRef.Namespace,
	}
	if err := p.client.Get(ctx, sourceKey, sourceSecret); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, fmt.Errorf("provider secret %s/%s not found: %w", sourceKey.Namespace, sourceKey.Name, err)
		}
		return nil, fmt.Errorf("failed to get provider secret: %w", err)
	}

	// Verify the key exists in the source secret
	secretKey := provider.Spec.Auth.APIKey.SecretRef.Key
	apiKeyData, exists := sourceSecret.Data[secretKey]
	if !exists {
		return nil, fmt.Errorf("key %s not found in secret %s/%s", secretKey, sourceKey.Namespace, sourceKey.Name)
	}

	// Prepare secret data with standard keys
	secretData := make(map[string][]byte)
	secretData["apiKey"] = apiKeyData

	// Prepare string data for metadata
	stringData := make(map[string]string)

	// Add base URL if configured
	if provider.Spec.Endpoint != nil && provider.Spec.Endpoint.BaseURL != "" {
		stringData["baseUrl"] = provider.Spec.Endpoint.BaseURL
	}

	// Add provider type
	stringData["provider"] = string(provider.Spec.Provider)

	// Collect keys for result
	secretKeys := []string{"apiKey"}
	if _, ok := stringData["baseUrl"]; ok {
		secretKeys = append(secretKeys, "baseUrl")
	}
	secretKeys = append(secretKeys, "provider")

	// Create or update the target secret in the LLMAccess namespace
	targetSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      access.Spec.SecretName,
			Namespace: access.Namespace,
		},
	}

	_, err := controllerutil.CreateOrUpdate(ctx, p.client, targetSecret, func() error {
		// Set owner reference for garbage collection
		if err := controllerutil.SetControllerReference(access, targetSecret, p.scheme); err != nil {
			return fmt.Errorf("failed to set owner reference: %w", err)
		}

		// Set data
		if targetSecret.Data == nil {
			targetSecret.Data = make(map[string][]byte)
		}
		for k, v := range secretData {
			targetSecret.Data[k] = v
		}

		if targetSecret.StringData == nil {
			targetSecret.StringData = make(map[string]string)
		}
		for k, v := range stringData {
			targetSecret.StringData[k] = v
		}

		// Set labels for tracking
		if targetSecret.Labels == nil {
			targetSecret.Labels = make(map[string]string)
		}
		targetSecret.Labels["llmwarden.io/managed-by"] = "llmwarden"
		targetSecret.Labels["llmwarden.io/provider"] = provider.Name
		targetSecret.Labels["llmwarden.io/access"] = access.Name
		targetSecret.Labels["llmwarden.io/auth-type"] = string(provider.Spec.Auth.Type)

		// Set type
		targetSecret.Type = corev1.SecretTypeOpaque

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to create/update secret: %w", err)
	}

	// Build metadata
	metadata := map[string]string{
		"provider":     provider.Name,
		"providerType": string(provider.Spec.Provider),
		"authType":     string(provider.Spec.Auth.Type),
		"sourceSecret": fmt.Sprintf("%s/%s", sourceKey.Namespace, sourceKey.Name),
		"targetSecret": fmt.Sprintf("%s/%s", access.Namespace, access.Spec.SecretName),
	}

	// Determine if rotation is needed
	needsRotation := false
	var expiresAt *time.Time

	if provider.Spec.Auth.APIKey.Rotation != nil && provider.Spec.Auth.APIKey.Rotation.Enabled {
		// Check if rotation interval has passed
		if targetSecret.CreationTimestamp.Time.Add(24 * time.Hour).Before(time.Now()) {
			needsRotation = true
		}
	}

	return &ProvisionResult{
		SecretName:      access.Spec.SecretName,
		SecretNamespace: access.Namespace,
		SecretKeys:      secretKeys,
		ExpiresAt:       expiresAt,
		NeedsRotation:   needsRotation,
		ProvisionedAt:   time.Now(),
		Metadata:        metadata,
	}, nil
}

// Cleanup removes the secret created for the LLMAccess.
// The secret will be automatically deleted via owner references when the LLMAccess is deleted,
// but this method provides explicit cleanup if needed.
func (p *ApiKeyProvisioner) Cleanup(ctx context.Context, provider *llmwardenv1alpha1.LLMProvider, access *llmwardenv1alpha1.LLMAccess) error {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      access.Spec.SecretName,
			Namespace: access.Namespace,
		},
	}

	err := p.client.Delete(ctx, secret)
	if err != nil {
		if apierrors.IsNotFound(err) {
			// Secret already deleted - this is fine
			return nil
		}
		return fmt.Errorf("failed to delete secret: %w", err)
	}

	return nil
}

// HealthCheck validates that the provisioned secret exists and contains valid data.
func (p *ApiKeyProvisioner) HealthCheck(ctx context.Context, provider *llmwardenv1alpha1.LLMProvider, access *llmwardenv1alpha1.LLMAccess) (*HealthCheckResult, error) {
	result := &HealthCheckResult{
		LastChecked: time.Now(),
		Metadata:    make(map[string]string),
	}

	// Check if target secret exists
	targetSecret := &corev1.Secret{}
	err := p.client.Get(ctx, types.NamespacedName{
		Name:      access.Spec.SecretName,
		Namespace: access.Namespace,
	}, targetSecret)

	if err != nil {
		if apierrors.IsNotFound(err) {
			result.Healthy = false
			result.Message = "Secret not found"
			return result, nil
		}
		return nil, fmt.Errorf("failed to get secret: %w", err)
	}

	// Verify apiKey exists in secret
	if _, exists := targetSecret.Data["apiKey"]; !exists {
		result.Healthy = false
		result.Message = "API key not found in secret"
		return result, nil
	}

	// Check if source secret still exists
	if provider.Spec.Auth.APIKey != nil {
		sourceSecret := &corev1.Secret{}
		sourceKey := types.NamespacedName{
			Name:      provider.Spec.Auth.APIKey.SecretRef.Name,
			Namespace: provider.Spec.Auth.APIKey.SecretRef.Namespace,
		}
		err := p.client.Get(ctx, sourceKey, sourceSecret)
		if err != nil {
			result.Warnings = append(result.Warnings, fmt.Sprintf("Source secret %s/%s not accessible", sourceKey.Namespace, sourceKey.Name))
		}
	}

	// Check age and rotation needs
	age := time.Since(targetSecret.CreationTimestamp.Time)
	result.Metadata["secretAge"] = age.String()

	if provider.Spec.Auth.APIKey != nil && provider.Spec.Auth.APIKey.Rotation != nil && provider.Spec.Auth.APIKey.Rotation.Enabled {
		// Warn if secret is getting old
		if age > 25*24*time.Hour { // 25 days if rotation is 30d
			result.Warnings = append(result.Warnings, "Secret is nearing rotation interval")
		}
	}

	result.Healthy = true
	result.Message = "Secret exists and contains valid API key"
	return result, nil
}
