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
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	llmwardenv1alpha1 "github.com/tpbansal/llmwarden/api/v1alpha1"
)

func TestApiKeyProvisioner_Provision(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = llmwardenv1alpha1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	tests := []struct {
		name          string
		provider      *llmwardenv1alpha1.LLMProvider
		access        *llmwardenv1alpha1.LLMAccess
		sourceSecret  *corev1.Secret
		wantErr       bool
		wantSecretKey string
		checkLabels   bool
	}{
		{
			name: "successful provision with basic config",
			provider: &llmwardenv1alpha1.LLMProvider{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-provider",
				},
				Spec: llmwardenv1alpha1.LLMProviderSpec{
					Provider: llmwardenv1alpha1.ProviderOpenAI,
					Auth: llmwardenv1alpha1.AuthConfig{
						Type: llmwardenv1alpha1.AuthTypeAPIKey,
						APIKey: &llmwardenv1alpha1.APIKeyAuth{
							SecretRef: llmwardenv1alpha1.SecretReference{
								Name:      "source-secret",
								Namespace: "provider-ns",
								Key:       "api-key",
							},
						},
					},
				},
			},
			access: &llmwardenv1alpha1.LLMAccess{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-access",
					Namespace: "test-ns",
					UID:       "test-uid-123",
				},
				Spec: llmwardenv1alpha1.LLMAccessSpec{
					SecretName: "target-secret",
					ProviderRef: llmwardenv1alpha1.ProviderReference{
						Name: "test-provider",
					},
					Injection: llmwardenv1alpha1.InjectionConfig{
						Env: []llmwardenv1alpha1.EnvVarMapping{
							{Name: "OPENAI_API_KEY", SecretKey: "apiKey"},
						},
					},
				},
			},
			sourceSecret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "source-secret",
					Namespace: "provider-ns",
				},
				Data: map[string][]byte{
					"api-key": []byte("sk-test-key-123"),
				},
			},
			wantErr:       false,
			wantSecretKey: "apiKey",
			checkLabels:   true,
		},
		{
			name: "successful provision with endpoint config",
			provider: &llmwardenv1alpha1.LLMProvider{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-provider-endpoint",
				},
				Spec: llmwardenv1alpha1.LLMProviderSpec{
					Provider: llmwardenv1alpha1.ProviderOpenAI,
					Auth: llmwardenv1alpha1.AuthConfig{
						Type: llmwardenv1alpha1.AuthTypeAPIKey,
						APIKey: &llmwardenv1alpha1.APIKeyAuth{
							SecretRef: llmwardenv1alpha1.SecretReference{
								Name:      "source-secret",
								Namespace: "provider-ns",
								Key:       "api-key",
							},
						},
					},
					Endpoint: &llmwardenv1alpha1.EndpointConfig{
						BaseURL: "https://custom-endpoint.example.com",
					},
				},
			},
			access: &llmwardenv1alpha1.LLMAccess{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-access-endpoint",
					Namespace: "test-ns",
					UID:       "test-uid-456",
				},
				Spec: llmwardenv1alpha1.LLMAccessSpec{
					SecretName: "target-secret-endpoint",
					ProviderRef: llmwardenv1alpha1.ProviderReference{
						Name: "test-provider-endpoint",
					},
					Injection: llmwardenv1alpha1.InjectionConfig{
						Env: []llmwardenv1alpha1.EnvVarMapping{
							{Name: "OPENAI_API_KEY", SecretKey: "apiKey"},
						},
					},
				},
			},
			sourceSecret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "source-secret",
					Namespace: "provider-ns",
				},
				Data: map[string][]byte{
					"api-key": []byte("sk-test-key-456"),
				},
			},
			wantErr:       false,
			wantSecretKey: "apiKey",
			checkLabels:   true,
		},
		{
			name: "error when source secret not found",
			provider: &llmwardenv1alpha1.LLMProvider{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-provider-missing",
				},
				Spec: llmwardenv1alpha1.LLMProviderSpec{
					Provider: llmwardenv1alpha1.ProviderOpenAI,
					Auth: llmwardenv1alpha1.AuthConfig{
						Type: llmwardenv1alpha1.AuthTypeAPIKey,
						APIKey: &llmwardenv1alpha1.APIKeyAuth{
							SecretRef: llmwardenv1alpha1.SecretReference{
								Name:      "missing-secret",
								Namespace: "provider-ns",
								Key:       "api-key",
							},
						},
					},
				},
			},
			access: &llmwardenv1alpha1.LLMAccess{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-access-missing",
					Namespace: "test-ns",
					UID:       "test-uid-789",
				},
				Spec: llmwardenv1alpha1.LLMAccessSpec{
					SecretName: "target-secret-missing",
					ProviderRef: llmwardenv1alpha1.ProviderReference{
						Name: "test-provider-missing",
					},
					Injection: llmwardenv1alpha1.InjectionConfig{
						Env: []llmwardenv1alpha1.EnvVarMapping{
							{Name: "OPENAI_API_KEY", SecretKey: "apiKey"},
						},
					},
				},
			},
			sourceSecret: nil, // No secret created
			wantErr:      true,
		},
		{
			name: "error when key not found in source secret",
			provider: &llmwardenv1alpha1.LLMProvider{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-provider-badkey",
				},
				Spec: llmwardenv1alpha1.LLMProviderSpec{
					Provider: llmwardenv1alpha1.ProviderOpenAI,
					Auth: llmwardenv1alpha1.AuthConfig{
						Type: llmwardenv1alpha1.AuthTypeAPIKey,
						APIKey: &llmwardenv1alpha1.APIKeyAuth{
							SecretRef: llmwardenv1alpha1.SecretReference{
								Name:      "source-secret-badkey",
								Namespace: "provider-ns",
								Key:       "wrong-key",
							},
						},
					},
				},
			},
			access: &llmwardenv1alpha1.LLMAccess{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-access-badkey",
					Namespace: "test-ns",
					UID:       "test-uid-abc",
				},
				Spec: llmwardenv1alpha1.LLMAccessSpec{
					SecretName: "target-secret-badkey",
					ProviderRef: llmwardenv1alpha1.ProviderReference{
						Name: "test-provider-badkey",
					},
					Injection: llmwardenv1alpha1.InjectionConfig{
						Env: []llmwardenv1alpha1.EnvVarMapping{
							{Name: "OPENAI_API_KEY", SecretKey: "apiKey"},
						},
					},
				},
			},
			sourceSecret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "source-secret-badkey",
					Namespace: "provider-ns",
				},
				Data: map[string][]byte{
					"api-key": []byte("sk-test-key-badkey"),
				},
			},
			wantErr: true,
		},
		{
			name: "error when provider has no apiKey config",
			provider: &llmwardenv1alpha1.LLMProvider{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-provider-noconfig",
				},
				Spec: llmwardenv1alpha1.LLMProviderSpec{
					Provider: llmwardenv1alpha1.ProviderOpenAI,
					Auth: llmwardenv1alpha1.AuthConfig{
						Type:   llmwardenv1alpha1.AuthTypeAPIKey,
						APIKey: nil, // No config
					},
				},
			},
			access: &llmwardenv1alpha1.LLMAccess{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-access-noconfig",
					Namespace: "test-ns",
					UID:       "test-uid-def",
				},
				Spec: llmwardenv1alpha1.LLMAccessSpec{
					SecretName: "target-secret-noconfig",
					ProviderRef: llmwardenv1alpha1.ProviderReference{
						Name: "test-provider-noconfig",
					},
					Injection: llmwardenv1alpha1.InjectionConfig{
						Env: []llmwardenv1alpha1.EnvVarMapping{
							{Name: "OPENAI_API_KEY", SecretKey: "apiKey"},
						},
					},
				},
			},
			sourceSecret: nil,
			wantErr:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()

			// Create fake client with initial objects
			objects := []runtime.Object{}
			if tt.sourceSecret != nil {
				objects = append(objects, tt.sourceSecret)
			}

			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithRuntimeObjects(objects...).
				Build()

			provisioner := NewApiKeyProvisioner(fakeClient, scheme)

			// Call Provision
			result, err := provisioner.Provision(ctx, tt.provider, tt.access)

			// Check error expectation
			if (err != nil) != tt.wantErr {
				t.Errorf("Provision() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErr {
				return // Expected error, test passes
			}

			// Verify result
			if result == nil {
				t.Fatal("Provision() returned nil result")
			}

			if result.SecretName != tt.access.Spec.SecretName {
				t.Errorf("Provision() result.SecretName = %v, want %v", result.SecretName, tt.access.Spec.SecretName)
			}

			if result.SecretNamespace != tt.access.Namespace {
				t.Errorf("Provision() result.SecretNamespace = %v, want %v", result.SecretNamespace, tt.access.Namespace)
			}

			// Verify secret was created
			targetSecret := &corev1.Secret{}
			err = fakeClient.Get(ctx, types.NamespacedName{
				Name:      tt.access.Spec.SecretName,
				Namespace: tt.access.Namespace,
			}, targetSecret)

			if err != nil {
				t.Fatalf("Failed to get target secret: %v", err)
			}

			// Verify secret contains apiKey
			if _, exists := targetSecret.Data[tt.wantSecretKey]; !exists {
				t.Errorf("Target secret missing key %s", tt.wantSecretKey)
			}

			// Verify API key value matches source
			if string(targetSecret.Data[tt.wantSecretKey]) != string(tt.sourceSecret.Data[tt.provider.Spec.Auth.APIKey.SecretRef.Key]) {
				t.Errorf("Target secret apiKey value mismatch")
			}

			// Check labels if requested
			if tt.checkLabels {
				expectedLabels := map[string]string{
					"llmwarden.io/managed-by": "llmwarden",
					"llmwarden.io/provider":   tt.provider.Name,
					"llmwarden.io/access":     tt.access.Name,
					"llmwarden.io/auth-type":  string(tt.provider.Spec.Auth.Type),
				}

				for k, v := range expectedLabels {
					if targetSecret.Labels[k] != v {
						t.Errorf("Label %s = %v, want %v", k, targetSecret.Labels[k], v)
					}
				}
			}

			// Verify endpoint is in stringData if configured
			if tt.provider.Spec.Endpoint != nil && tt.provider.Spec.Endpoint.BaseURL != "" {
				if targetSecret.StringData["baseUrl"] != tt.provider.Spec.Endpoint.BaseURL {
					t.Errorf("baseUrl = %v, want %v", targetSecret.StringData["baseUrl"], tt.provider.Spec.Endpoint.BaseURL)
				}
			}
		})
	}
}

func TestApiKeyProvisioner_Cleanup(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = llmwardenv1alpha1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	ctx := context.Background()

	// Create test secret
	existingSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cleanup-secret",
			Namespace: "test-ns",
		},
		Data: map[string][]byte{
			"apiKey": []byte("sk-cleanup-test"),
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithRuntimeObjects(existingSecret).
		Build()

	provisioner := NewApiKeyProvisioner(fakeClient, scheme)

	access := &llmwardenv1alpha1.LLMAccess{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-access",
			Namespace: "test-ns",
		},
		Spec: llmwardenv1alpha1.LLMAccessSpec{
			SecretName: "cleanup-secret",
			ProviderRef: llmwardenv1alpha1.ProviderReference{
				Name: "test-provider",
			},
			Injection: llmwardenv1alpha1.InjectionConfig{
				Env: []llmwardenv1alpha1.EnvVarMapping{
					{Name: "OPENAI_API_KEY", SecretKey: "apiKey"},
				},
			},
		},
	}

	provider := &llmwardenv1alpha1.LLMProvider{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-provider",
		},
		Spec: llmwardenv1alpha1.LLMProviderSpec{
			Provider: llmwardenv1alpha1.ProviderOpenAI,
			Auth: llmwardenv1alpha1.AuthConfig{
				Type: llmwardenv1alpha1.AuthTypeAPIKey,
			},
		},
	}

	// Call Cleanup
	err := provisioner.Cleanup(ctx, provider, access)
	if err != nil {
		t.Fatalf("Cleanup() error = %v", err)
	}

	// Verify secret was deleted
	targetSecret := &corev1.Secret{}
	err = fakeClient.Get(ctx, types.NamespacedName{
		Name:      "cleanup-secret",
		Namespace: "test-ns",
	}, targetSecret)

	if err == nil {
		t.Error("Secret should have been deleted")
	}

	// Test cleanup when secret doesn't exist (should not error)
	err = provisioner.Cleanup(ctx, provider, access)
	if err != nil {
		t.Errorf("Cleanup() on non-existent secret error = %v, want nil", err)
	}
}

func TestApiKeyProvisioner_HealthCheck(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = llmwardenv1alpha1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	tests := []struct {
		name         string
		targetSecret *corev1.Secret
		sourceSecret *corev1.Secret
		wantHealthy  bool
		wantMessage  string
	}{
		{
			name: "healthy when secret exists with apiKey",
			targetSecret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "health-secret",
					Namespace: "test-ns",
				},
				Data: map[string][]byte{
					"apiKey": []byte("sk-healthy-key"),
				},
			},
			sourceSecret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "source-secret",
					Namespace: "provider-ns",
				},
				Data: map[string][]byte{
					"api-key": []byte("sk-source-key"),
				},
			},
			wantHealthy: true,
			wantMessage: "Secret exists and contains valid API key",
		},
		{
			name:         "unhealthy when secret not found",
			targetSecret: nil,
			sourceSecret: nil,
			wantHealthy:  false,
			wantMessage:  "Secret not found",
		},
		{
			name: "unhealthy when apiKey missing",
			targetSecret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "health-secret-nokey",
					Namespace: "test-ns",
				},
				Data: map[string][]byte{
					"wrongKey": []byte("sk-wrong"),
				},
			},
			sourceSecret: nil,
			wantHealthy:  false,
			wantMessage:  "API key not found in secret",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()

			objects := []runtime.Object{}
			if tt.targetSecret != nil {
				objects = append(objects, tt.targetSecret)
			}
			if tt.sourceSecret != nil {
				objects = append(objects, tt.sourceSecret)
			}

			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithRuntimeObjects(objects...).
				Build()

			provisioner := NewApiKeyProvisioner(fakeClient, scheme)

			access := &llmwardenv1alpha1.LLMAccess{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-access",
					Namespace: "test-ns",
				},
				Spec: llmwardenv1alpha1.LLMAccessSpec{
					SecretName: "health-secret",
					ProviderRef: llmwardenv1alpha1.ProviderReference{
						Name: "test-provider",
					},
					Injection: llmwardenv1alpha1.InjectionConfig{
						Env: []llmwardenv1alpha1.EnvVarMapping{
							{Name: "OPENAI_API_KEY", SecretKey: "apiKey"},
						},
					},
				},
			}

			if tt.name == "unhealthy when apiKey missing" {
				access.Spec.SecretName = "health-secret-nokey"
			}

			provider := &llmwardenv1alpha1.LLMProvider{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-provider",
				},
				Spec: llmwardenv1alpha1.LLMProviderSpec{
					Provider: llmwardenv1alpha1.ProviderOpenAI,
					Auth: llmwardenv1alpha1.AuthConfig{
						Type: llmwardenv1alpha1.AuthTypeAPIKey,
						APIKey: &llmwardenv1alpha1.APIKeyAuth{
							SecretRef: llmwardenv1alpha1.SecretReference{
								Name:      "source-secret",
								Namespace: "provider-ns",
								Key:       "api-key",
							},
						},
					},
				},
			}

			result, err := provisioner.HealthCheck(ctx, provider, access)
			if err != nil {
				t.Fatalf("HealthCheck() error = %v", err)
			}

			if result.Healthy != tt.wantHealthy {
				t.Errorf("HealthCheck() Healthy = %v, want %v", result.Healthy, tt.wantHealthy)
			}

			if result.Message != tt.wantMessage {
				t.Errorf("HealthCheck() Message = %v, want %v", result.Message, tt.wantMessage)
			}

			if result.LastChecked.IsZero() {
				t.Error("HealthCheck() LastChecked should be set")
			}
		})
	}
}
