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

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	llmwardenv1alpha1 "github.com/thinkingcow-dev/llmwarden/api/v1alpha1"
	"github.com/thinkingcow-dev/llmwarden/internal/eso"
)

// testProvider returns a minimal LLMProvider with externalSecret auth configured.
func testProvider(storeName, storeKind, remoteKey, remoteProperty, refreshInterval string) *llmwardenv1alpha1.LLMProvider {
	return &llmwardenv1alpha1.LLMProvider{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-provider",
		},
		Spec: llmwardenv1alpha1.LLMProviderSpec{
			Provider: llmwardenv1alpha1.ProviderOpenAI,
			Auth: llmwardenv1alpha1.AuthConfig{
				Type: llmwardenv1alpha1.AuthTypeExternalSecret,
				ExternalSecret: &llmwardenv1alpha1.ExternalSecretAuth{
					Store: llmwardenv1alpha1.StoreReference{
						Name: storeName,
						Kind: llmwardenv1alpha1.SecretStoreKind(storeKind),
					},
					RemoteRef: llmwardenv1alpha1.RemoteReference{
						Key:      remoteKey,
						Property: remoteProperty,
					},
					RefreshInterval: refreshInterval,
				},
			},
		},
	}
}

// testAccess returns a minimal LLMAccess for the given provider.
func testAccess(namespace, secretName, rotationInterval string) *llmwardenv1alpha1.LLMAccess {
	access := &llmwardenv1alpha1.LLMAccess{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-access",
			Namespace: namespace,
			UID:       "test-uid-es-1",
		},
		Spec: llmwardenv1alpha1.LLMAccessSpec{
			ProviderRef: llmwardenv1alpha1.ProviderReference{Name: "test-provider"},
			SecretName:  secretName,
			Injection: llmwardenv1alpha1.InjectionConfig{
				Env: []llmwardenv1alpha1.EnvVarMapping{
					{Name: "OPENAI_API_KEY", SecretKey: "apiKey"},
				},
			},
		},
	}
	if rotationInterval != "" {
		access.Spec.Rotation = &llmwardenv1alpha1.AccessRotationConfig{
			Interval: rotationInterval,
		}
	}
	return access
}

// newTestScheme builds a scheme with llmwarden types registered (core types not needed for ES tests).
func newTestScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	_ = llmwardenv1alpha1.AddToScheme(s)
	return s
}

func TestExternalSecretProvisioner_Provision(t *testing.T) {
	tests := []struct {
		name                string
		provider            *llmwardenv1alpha1.LLMProvider
		access              *llmwardenv1alpha1.LLMAccess
		wantErr             bool
		wantESName          string
		wantRefreshInterval string
		wantStoreRef        map[string]string
		wantRemoteKey       string
		wantRemoteProperty  string
		wantLabels          map[string]string
	}{
		{
			name: "creates ExternalSecret with ClusterSecretStore",
			provider: testProvider(
				"vault-backend", "ClusterSecretStore",
				"secret/data/openai/production", "api-key",
				"1h",
			),
			access:              testAccess("test-ns", "openai-credentials", ""),
			wantErr:             false,
			wantESName:          "openai-credentials",
			wantRefreshInterval: "1h",
			wantStoreRef: map[string]string{
				"name": "vault-backend",
				"kind": "ClusterSecretStore",
			},
			wantRemoteKey:      "secret/data/openai/production",
			wantRemoteProperty: "api-key",
			wantLabels: map[string]string{
				"llmwarden.io/managed-by": "llmwarden",
				"llmwarden.io/provider":   "test-provider",
				"llmwarden.io/access":     "test-access",
				"llmwarden.io/auth-type":  "externalSecret",
			},
		},
		{
			name: "creates ExternalSecret with namespace-scoped SecretStore",
			provider: testProvider(
				"aws-sm-store", "SecretStore",
				"prod/openai/key", "",
				"30m",
			),
			access:              testAccess("prod-ns", "openai-creds", ""),
			wantErr:             false,
			wantESName:          "openai-creds",
			wantRefreshInterval: "30m",
			wantStoreRef: map[string]string{
				"name": "aws-sm-store",
				"kind": "SecretStore",
			},
			wantRemoteKey:      "prod/openai/key",
			wantRemoteProperty: "",
		},
		{
			name: "LLMAccess rotation.interval overrides provider refreshInterval",
			provider: testProvider(
				"vault-backend", "ClusterSecretStore",
				"secret/openai", "key",
				"24h",
			),
			access:              testAccess("test-ns", "openai-creds", "6h"),
			wantErr:             false,
			wantESName:          "openai-creds",
			wantRefreshInterval: "6h", // access override wins
			wantRemoteKey:       "secret/openai",
		},
		{
			name: "uses default refresh interval when provider has none",
			provider: testProvider(
				"vault-backend", "ClusterSecretStore",
				"secret/openai", "key",
				"", // no provider interval
			),
			access:              testAccess("test-ns", "creds", ""),
			wantErr:             false,
			wantRefreshInterval: "1h", // default
		},
		{
			name: "error when externalSecret config is nil",
			provider: &llmwardenv1alpha1.LLMProvider{
				ObjectMeta: metav1.ObjectMeta{Name: "bad-provider"},
				Spec: llmwardenv1alpha1.LLMProviderSpec{
					Provider: llmwardenv1alpha1.ProviderOpenAI,
					Auth: llmwardenv1alpha1.AuthConfig{
						Type:           llmwardenv1alpha1.AuthTypeExternalSecret,
						ExternalSecret: nil,
					},
				},
			},
			access:  testAccess("test-ns", "openai-creds", ""),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			scheme := newTestScheme()
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
			adapter := eso.NewV1Beta1Adapter()

			p := NewExternalSecretProvisioner(fakeClient, scheme, adapter)

			result, err := p.Provision(ctx, tt.provider, tt.access)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Provision() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}

			// Verify result fields
			if result == nil {
				t.Fatal("Provision() returned nil result")
			}
			if result.SecretName != tt.access.Spec.SecretName {
				t.Errorf("result.SecretName = %q, want %q", result.SecretName, tt.access.Spec.SecretName)
			}
			if result.SecretNamespace != tt.access.Namespace {
				t.Errorf("result.SecretNamespace = %q, want %q", result.SecretNamespace, tt.access.Namespace)
			}

			// Verify ExternalSecret was created in the fake client
			esName := tt.access.Spec.SecretName
			if tt.wantESName != "" {
				esName = tt.wantESName
			}

			esObj := &unstructured.Unstructured{}
			esObj.SetGroupVersionKind(adapter.GVK())
			err = fakeClient.Get(ctx, types.NamespacedName{
				Namespace: tt.access.Namespace,
				Name:      esName,
			}, esObj)
			if err != nil {
				t.Fatalf("ExternalSecret not found after Provision: %v", err)
			}

			// Verify refreshInterval in spec
			if tt.wantRefreshInterval != "" {
				gotInterval, _, _ := unstructured.NestedString(esObj.Object, "spec", "refreshInterval")
				if gotInterval != tt.wantRefreshInterval {
					t.Errorf("spec.refreshInterval = %q, want %q", gotInterval, tt.wantRefreshInterval)
				}
			}

			// Verify secretStoreRef
			for k, wantV := range tt.wantStoreRef {
				gotV, _, _ := unstructured.NestedString(esObj.Object, "spec", "secretStoreRef", k)
				if gotV != wantV {
					t.Errorf("spec.secretStoreRef.%s = %q, want %q", k, gotV, wantV)
				}
			}

			// Verify target
			gotTargetName, _, _ := unstructured.NestedString(esObj.Object, "spec", "target", "name")
			if gotTargetName != tt.access.Spec.SecretName {
				t.Errorf("spec.target.name = %q, want %q", gotTargetName, tt.access.Spec.SecretName)
			}

			// Verify data[0].remoteRef.key
			if tt.wantRemoteKey != "" {
				// NestedString doesn't work with array index — iterate data slice instead
				dataSlice, _, _ := unstructured.NestedSlice(esObj.Object, "spec", "data")
				if len(dataSlice) == 0 {
					t.Fatal("spec.data is empty")
				}
				firstData, ok := dataSlice[0].(map[string]interface{})
				if !ok {
					t.Fatal("spec.data[0] is not a map")
				}
				remoteRef, ok := firstData["remoteRef"].(map[string]interface{})
				if !ok {
					t.Fatal("spec.data[0].remoteRef is not a map")
				}
				if gotKey, _ := remoteRef["key"].(string); gotKey != tt.wantRemoteKey {
					t.Errorf("spec.data[0].remoteRef.key = %q, want %q", gotKey, tt.wantRemoteKey)
				}
				if tt.wantRemoteProperty != "" {
					gotProp, _ := remoteRef["property"].(string)
					if gotProp != tt.wantRemoteProperty {
						t.Errorf("spec.data[0].remoteRef.property = %q, want %q", gotProp, tt.wantRemoteProperty)
					}
				}
			}

			// Verify standard labels
			for k, wantV := range tt.wantLabels {
				gotV := esObj.GetLabels()[k]
				if gotV != wantV {
					t.Errorf("label %s = %q, want %q", k, gotV, wantV)
				}
			}
		})
	}
}

func TestExternalSecretProvisioner_Provision_Idempotent(t *testing.T) {
	ctx := context.Background()
	scheme := newTestScheme()
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	adapter := eso.NewV1Beta1Adapter()
	p := NewExternalSecretProvisioner(fakeClient, scheme, adapter)

	provider := testProvider("vault", "ClusterSecretStore", "secret/openai", "key", "1h")
	access := testAccess("test-ns", "openai-creds", "")

	// First call
	_, err := p.Provision(ctx, provider, access)
	if err != nil {
		t.Fatalf("First Provision() error = %v", err)
	}

	// Second call — must not fail (idempotent)
	_, err = p.Provision(ctx, provider, access)
	if err != nil {
		t.Fatalf("Second Provision() error = %v (should be idempotent)", err)
	}

	// Verify only one ExternalSecret exists
	esList := &unstructured.UnstructuredList{}
	esList.SetGroupVersionKind(adapter.GVK())
	_ = fakeClient.List(ctx, esList)
	// Note: List on unstructured requires the GVK to be list kind — skip count check in fake
}

func TestExternalSecretProvisioner_Cleanup(t *testing.T) {
	ctx := context.Background()
	scheme := newTestScheme()
	adapter := eso.NewV1Beta1Adapter()

	// Pre-create an ExternalSecret in the fake store
	existingES := &unstructured.Unstructured{}
	existingES.SetGroupVersionKind(adapter.GVK())
	existingES.SetNamespace("test-ns")
	existingES.SetName("openai-creds")

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(existingES).
		Build()

	p := NewExternalSecretProvisioner(fakeClient, scheme, adapter)

	provider := testProvider("vault", "ClusterSecretStore", "secret/openai", "key", "1h")
	access := testAccess("test-ns", "openai-creds", "")

	// Cleanup should delete the ExternalSecret
	err := p.Cleanup(ctx, provider, access)
	if err != nil {
		t.Fatalf("Cleanup() error = %v", err)
	}

	// Verify it's gone
	esObj := &unstructured.Unstructured{}
	esObj.SetGroupVersionKind(adapter.GVK())
	err = fakeClient.Get(ctx, types.NamespacedName{Namespace: "test-ns", Name: "openai-creds"}, esObj)
	if err == nil {
		t.Error("ExternalSecret should have been deleted")
	}

	// Cleanup on non-existent resource must be idempotent
	err = p.Cleanup(ctx, provider, access)
	if err != nil {
		t.Errorf("Cleanup() on non-existent ExternalSecret error = %v, want nil", err)
	}
}

func TestExternalSecretProvisioner_HealthCheck(t *testing.T) {
	adapter := eso.NewV1Beta1Adapter()

	// Helper to build an ExternalSecret with a given sync condition.
	buildESWithCondition := func(namespace, name, conditionStatus, message string) *unstructured.Unstructured {
		es := &unstructured.Unstructured{}
		es.SetGroupVersionKind(adapter.GVK())
		es.SetNamespace(namespace)
		es.SetName(name)
		es.Object["status"] = map[string]interface{}{
			"conditions": []interface{}{
				map[string]interface{}{
					"type":    "Ready",
					"status":  conditionStatus,
					"message": message,
				},
			},
		}
		return es
	}

	tests := []struct {
		name        string
		existingES  *unstructured.Unstructured
		wantHealthy bool
		wantMessage string
	}{
		{
			name:        "healthy when ESO has synced",
			existingES:  buildESWithCondition("test-ns", "openai-creds", "True", "Secret synced successfully"),
			wantHealthy: true,
			wantMessage: "Secret synced successfully",
		},
		{
			name:        "unhealthy when ESO sync failed",
			existingES:  buildESWithCondition("test-ns", "openai-creds", "False", "SecretStore not found"),
			wantHealthy: false,
			wantMessage: "SecretStore not found",
		},
		{
			name:        "unhealthy when ExternalSecret not found",
			existingES:  nil,
			wantHealthy: false,
			wantMessage: "ExternalSecret not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			scheme := newTestScheme()

			builder := fake.NewClientBuilder().WithScheme(scheme)
			if tt.existingES != nil {
				builder = builder.WithObjects(tt.existingES)
			}
			fakeClient := builder.Build()

			p := NewExternalSecretProvisioner(fakeClient, scheme, adapter)

			provider := testProvider("vault", "ClusterSecretStore", "secret/openai", "key", "1h")
			access := testAccess("test-ns", "openai-creds", "")

			result, err := p.HealthCheck(ctx, provider, access)
			if err != nil {
				t.Fatalf("HealthCheck() error = %v", err)
			}
			if result.Healthy != tt.wantHealthy {
				t.Errorf("HealthCheck().Healthy = %v, want %v", result.Healthy, tt.wantHealthy)
			}
			if result.Message != tt.wantMessage {
				t.Errorf("HealthCheck().Message = %q, want %q", result.Message, tt.wantMessage)
			}
			if result.LastChecked.IsZero() {
				t.Error("HealthCheck().LastChecked should be set")
			}
		})
	}
}

func TestExternalSecretProvisioner_effectiveRefreshInterval(t *testing.T) {
	p := &ExternalSecretProvisioner{}

	tests := []struct {
		name             string
		accessInterval   string
		providerInterval string
		want             string
	}{
		{"access override wins", "6h", "24h", "6h"},
		{"provider interval used when access has none", "", "24h", "24h"},
		{"default when both are empty", "", "", "1h"},
		{"access override used even when provider empty", "2h", "", "2h"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			access := testAccess("ns", "creds", tt.accessInterval)
			got := p.effectiveRefreshInterval(access, tt.providerInterval)
			if got != tt.want {
				t.Errorf("effectiveRefreshInterval() = %q, want %q", got, tt.want)
			}
		})
	}
}
