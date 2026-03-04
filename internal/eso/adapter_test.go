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

package eso

import (
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// adapterTestCase is a shared test case run against both adapters to prevent drift.
type adapterTestCase struct {
	name          string
	namespace     string
	esName        string
	labels        map[string]string
	spec          ExternalSecretSpec
	wantVersion   string // expected API version in object metadata
	wantNamespace string
	wantName      string
	// spec field assertions
	wantRefreshInterval string
	wantStoreRefName    string
	wantStoreRefKind    string
	wantTargetName      string
	wantCreationPolicy  string
	wantDataKey         string
	wantRemoteKey       string
	wantRemoteProperty  string // "" means property must be absent
	wantRemoteVersion   string // "" means version must be absent
}

func adapterCases() []adapterTestCase {
	return []adapterTestCase{
		{
			name:      "full spec with property and version",
			namespace: "prod-ns",
			esName:    "openai-es",
			labels: map[string]string{
				"llmwarden.io/managed-by": "llmwarden",
				"llmwarden.io/provider":   "openai-prod",
			},
			spec: ExternalSecretSpec{
				RefreshInterval: "1h",
				StoreRef:        StoreRef{Name: "vault-backend", Kind: "ClusterSecretStore"},
				Target:          ExternalSecretTarget{Name: "openai-creds", CreationPolicy: SecretCreationPolicyOwner},
				Data: []ExternalSecretData{
					{
						SecretKey: "apiKey",
						RemoteRef: RemoteRef{Key: "secret/data/openai", Property: "api-key", Version: "v2"},
					},
				},
			},
			wantNamespace:       "prod-ns",
			wantName:            "openai-es",
			wantRefreshInterval: "1h",
			wantStoreRefName:    "vault-backend",
			wantStoreRefKind:    "ClusterSecretStore",
			wantTargetName:      "openai-creds",
			wantCreationPolicy:  "Owner",
			wantDataKey:         "apiKey",
			wantRemoteKey:       "secret/data/openai",
			wantRemoteProperty:  "api-key",
			wantRemoteVersion:   "v2",
		},
		{
			name:      "minimal spec without property or version",
			namespace: "dev-ns",
			esName:    "anthropic-es",
			labels:    map[string]string{"llmwarden.io/managed-by": "llmwarden"},
			spec: ExternalSecretSpec{
				RefreshInterval: "30m",
				StoreRef:        StoreRef{Name: "aws-sm", Kind: "SecretStore"},
				Target:          ExternalSecretTarget{Name: "anthropic-creds", CreationPolicy: SecretCreationPolicyOrphan},
				Data: []ExternalSecretData{
					{
						SecretKey: "key",
						RemoteRef: RemoteRef{Key: "prod/anthropic/key"},
					},
				},
			},
			wantNamespace:       "dev-ns",
			wantName:            "anthropic-es",
			wantRefreshInterval: "30m",
			wantStoreRefName:    "aws-sm",
			wantStoreRefKind:    "SecretStore",
			wantTargetName:      "anthropic-creds",
			wantCreationPolicy:  "Orphan",
			wantDataKey:         "key",
			wantRemoteKey:       "prod/anthropic/key",
			wantRemoteProperty:  "", // must be absent
			wantRemoteVersion:   "", // must be absent
		},
		{
			name:      "labels are propagated",
			namespace: "ns",
			esName:    "es",
			labels: map[string]string{
				"foo": "bar",
				"baz": "qux",
			},
			spec: ExternalSecretSpec{
				RefreshInterval: "5m",
				StoreRef:        StoreRef{Name: "store", Kind: "SecretStore"},
				Target:          ExternalSecretTarget{Name: "secret", CreationPolicy: SecretCreationPolicyOwner},
				Data:            []ExternalSecretData{{SecretKey: "k", RemoteRef: RemoteRef{Key: "r"}}},
			},
			wantNamespace: "ns",
			wantName:      "es",
		},
	}
}

// runAdapterTests runs all shared cases against the given adapter.
func runAdapterTests(t *testing.T, adapter Adapter, expectedVersion string) {
	t.Helper()
	for _, tc := range adapterCases() {
		t.Run(tc.name, func(t *testing.T) {
			obj := adapter.Build(tc.namespace, tc.esName, tc.labels, tc.spec)
			if obj == nil {
				t.Fatal("Build() returned nil")
			}

			// Metadata
			if obj.GetNamespace() != tc.wantNamespace {
				t.Errorf("namespace = %q, want %q", obj.GetNamespace(), tc.wantNamespace)
			}
			if obj.GetName() != tc.wantName {
				t.Errorf("name = %q, want %q", obj.GetName(), tc.wantName)
			}
			if obj.GetAPIVersion() != "external-secrets.io/"+expectedVersion {
				t.Errorf("apiVersion = %q, want %q", obj.GetAPIVersion(), "external-secrets.io/"+expectedVersion)
			}
			if obj.GetKind() != "ExternalSecret" {
				t.Errorf("kind = %q, want ExternalSecret", obj.GetKind())
			}

			// Labels
			for k, wantV := range tc.labels {
				if gotV := obj.GetLabels()[k]; gotV != wantV {
					t.Errorf("label[%s] = %q, want %q", k, gotV, wantV)
				}
			}

			// refreshInterval
			if tc.wantRefreshInterval != "" {
				got, _, _ := unstructured.NestedString(obj.Object, "spec", "refreshInterval")
				if got != tc.wantRefreshInterval {
					t.Errorf("spec.refreshInterval = %q, want %q", got, tc.wantRefreshInterval)
				}
			}

			// secretStoreRef
			if tc.wantStoreRefName != "" {
				got, _, _ := unstructured.NestedString(obj.Object, "spec", "secretStoreRef", "name")
				if got != tc.wantStoreRefName {
					t.Errorf("spec.secretStoreRef.name = %q, want %q", got, tc.wantStoreRefName)
				}
			}
			if tc.wantStoreRefKind != "" {
				got, _, _ := unstructured.NestedString(obj.Object, "spec", "secretStoreRef", "kind")
				if got != tc.wantStoreRefKind {
					t.Errorf("spec.secretStoreRef.kind = %q, want %q", got, tc.wantStoreRefKind)
				}
			}

			// target
			if tc.wantTargetName != "" {
				got, _, _ := unstructured.NestedString(obj.Object, "spec", "target", "name")
				if got != tc.wantTargetName {
					t.Errorf("spec.target.name = %q, want %q", got, tc.wantTargetName)
				}
			}
			if tc.wantCreationPolicy != "" {
				got, _, _ := unstructured.NestedString(obj.Object, "spec", "target", "creationPolicy")
				if got != tc.wantCreationPolicy {
					t.Errorf("spec.target.creationPolicy = %q, want %q", got, tc.wantCreationPolicy)
				}
			}

			// data[0]
			if tc.wantRemoteKey != "" {
				dataSlice, _, _ := unstructured.NestedSlice(obj.Object, "spec", "data")
				if len(dataSlice) == 0 {
					t.Fatal("spec.data is empty")
				}
				d, ok := dataSlice[0].(map[string]any)
				if !ok {
					t.Fatal("spec.data[0] is not a map")
				}
				if tc.wantDataKey != "" {
					if got, _ := d["secretKey"].(string); got != tc.wantDataKey {
						t.Errorf("spec.data[0].secretKey = %q, want %q", got, tc.wantDataKey)
					}
				}
				rr, ok := d["remoteRef"].(map[string]any)
				if !ok {
					t.Fatal("spec.data[0].remoteRef is not a map")
				}
				if got, _ := rr["key"].(string); got != tc.wantRemoteKey {
					t.Errorf("spec.data[0].remoteRef.key = %q, want %q", got, tc.wantRemoteKey)
				}
				// property must be set only when expected
				gotProp, _ := rr["property"].(string)
				if tc.wantRemoteProperty != "" && gotProp != tc.wantRemoteProperty {
					t.Errorf("spec.data[0].remoteRef.property = %q, want %q", gotProp, tc.wantRemoteProperty)
				}
				if tc.wantRemoteProperty == "" && gotProp != "" {
					t.Errorf("spec.data[0].remoteRef.property = %q, want absent", gotProp)
				}
				// version must be set only when expected
				gotVer, _ := rr["version"].(string)
				if tc.wantRemoteVersion != "" && gotVer != tc.wantRemoteVersion {
					t.Errorf("spec.data[0].remoteRef.version = %q, want %q", gotVer, tc.wantRemoteVersion)
				}
				if tc.wantRemoteVersion == "" && gotVer != "" {
					t.Errorf("spec.data[0].remoteRef.version = %q, want absent", gotVer)
				}
			}
		})
	}
}

// TestV1Beta1Adapter_Build runs the shared build assertions against the v1beta1 adapter.
func TestV1Beta1Adapter_Build(t *testing.T) {
	runAdapterTests(t, NewV1Beta1Adapter(), "v1beta1")
}

// TestV1Adapter_Build runs the same assertions against the v1 adapter, preventing drift.
func TestV1Adapter_Build(t *testing.T) {
	runAdapterTests(t, NewV1Adapter(), "v1")
}

// TestAdapters_GVK verifies each adapter reports the correct GVK.
func TestAdapters_GVK(t *testing.T) {
	cases := []struct {
		name        string
		adapter     Adapter
		wantGroup   string
		wantVersion string
		wantKind    string
	}{
		{"v1beta1", NewV1Beta1Adapter(), "external-secrets.io", "v1beta1", "ExternalSecret"},
		{"v1", NewV1Adapter(), "external-secrets.io", "v1", "ExternalSecret"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gvk := tc.adapter.GVK()
			if gvk.Group != tc.wantGroup {
				t.Errorf("GVK().Group = %q, want %q", gvk.Group, tc.wantGroup)
			}
			if gvk.Version != tc.wantVersion {
				t.Errorf("GVK().Version = %q, want %q", gvk.Version, tc.wantVersion)
			}
			if gvk.Kind != tc.wantKind {
				t.Errorf("GVK().Kind = %q, want %q", gvk.Kind, tc.wantKind)
			}
		})
	}
}

// TestAdapters_ParseSyncStatus verifies status parsing is consistent across adapters.
func TestAdapters_ParseSyncStatus(t *testing.T) {
	adapters := []struct {
		name    string
		adapter Adapter
	}{
		{"v1beta1", NewV1Beta1Adapter()},
		{"v1", NewV1Adapter()},
	}

	makeES := func(adapter Adapter, condStatus, message string) *unstructured.Unstructured {
		obj := &unstructured.Unstructured{}
		obj.SetGroupVersionKind(adapter.GVK())
		obj.Object["status"] = map[string]any{
			"conditions": []any{
				map[string]any{
					"type":    "Ready",
					"status":  condStatus,
					"message": message,
				},
			},
		}
		return obj
	}

	statusCases := []struct {
		name        string
		obj         func(adapter Adapter) *unstructured.Unstructured
		wantReady   bool
		wantMessage string
	}{
		{
			name:        "ready when condition True",
			obj:         func(a Adapter) *unstructured.Unstructured { return makeES(a, "True", "synced") },
			wantReady:   true,
			wantMessage: "synced",
		},
		{
			name:        "not ready when condition False",
			obj:         func(a Adapter) *unstructured.Unstructured { return makeES(a, "False", "store not found") },
			wantReady:   false,
			wantMessage: "store not found",
		},
		{
			name: "not ready when no conditions",
			obj: func(a Adapter) *unstructured.Unstructured {
				obj := &unstructured.Unstructured{}
				obj.SetGroupVersionKind(a.GVK())
				return obj
			},
			wantReady: false,
		},
		{
			name:      "not ready when nil input",
			obj:       func(_ Adapter) *unstructured.Unstructured { return nil },
			wantReady: false,
		},
	}

	for _, adapterCase := range adapters {
		for _, sc := range statusCases {
			t.Run(adapterCase.name+"/"+sc.name, func(t *testing.T) {
				status := adapterCase.adapter.ParseSyncStatus(sc.obj(adapterCase.adapter))
				if status == nil {
					t.Fatal("ParseSyncStatus() returned nil")
				}
				if status.Ready != sc.wantReady {
					t.Errorf("Ready = %v, want %v", status.Ready, sc.wantReady)
				}
				if sc.wantMessage != "" && status.Message != sc.wantMessage {
					t.Errorf("Message = %q, want %q", status.Message, sc.wantMessage)
				}
			})
		}
	}
}
