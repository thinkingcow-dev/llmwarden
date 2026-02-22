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
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// V1Beta1GVK is the GroupVersionKind for the ESO v1beta1 ExternalSecret resource.
// Update this constant (and the field mapping in V1Beta1Adapter) to target a different ESO version.
var V1Beta1GVK = schema.GroupVersionKind{
	Group:   "external-secrets.io",
	Version: "v1beta1",
	Kind:    "ExternalSecret",
}

// V1Beta1Adapter implements Adapter for ESO API version v1beta1.
// It uses unstructured.Unstructured to avoid a direct Go module dependency on the
// external-secrets/external-secrets package, making version migration straightforward.
type V1Beta1Adapter struct{}

// NewV1Beta1Adapter creates an Adapter targeting ESO v1beta1.
func NewV1Beta1Adapter() *V1Beta1Adapter {
	return &V1Beta1Adapter{}
}

// GVK returns the ExternalSecret GroupVersionKind for ESO v1beta1.
func (a *V1Beta1Adapter) GVK() schema.GroupVersionKind {
	return V1Beta1GVK
}

// Build constructs an unstructured ExternalSecret object for ESO v1beta1.
// See: https://external-secrets.io/latest/api/externalsecret/
func (a *V1Beta1Adapter) Build(namespace, name string, labels map[string]string, spec ExternalSecretSpec) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(a.GVK())
	obj.SetNamespace(namespace)
	obj.SetName(name)
	obj.SetLabels(labels)

	obj.Object["spec"] = a.buildSpec(spec)

	return obj
}

// buildSpec converts our internal ExternalSecretSpec to the ESO v1beta1 spec map.
// Field names match the ESO v1beta1 API exactly. Updating this method is the only
// change needed when ESO alters field names or structure in a future version.
func (a *V1Beta1Adapter) buildSpec(spec ExternalSecretSpec) map[string]any {
	// SecretStore reference
	secretStoreRef := map[string]any{
		"name": spec.StoreRef.Name,
		"kind": spec.StoreRef.Kind,
	}

	// Target secret configuration
	target := map[string]any{
		"name":           spec.Target.Name,
		"creationPolicy": string(spec.Target.CreationPolicy),
	}

	// Data entries: remote → local secret key mappings
	data := make([]any, 0, len(spec.Data))
	for _, d := range spec.Data {
		remoteRef := map[string]any{
			"key": d.RemoteRef.Key,
		}
		if d.RemoteRef.Property != "" {
			remoteRef["property"] = d.RemoteRef.Property
		}
		if d.RemoteRef.Version != "" {
			remoteRef["version"] = d.RemoteRef.Version
		}
		data = append(data, map[string]any{
			"secretKey": d.SecretKey,
			"remoteRef": remoteRef,
		})
	}

	return map[string]any{
		"refreshInterval": spec.RefreshInterval,
		"secretStoreRef":  secretStoreRef,
		"target":          target,
		"data":            data,
	}
}

// ParseSyncStatus reads the sync status from an ESO v1beta1 ExternalSecret object.
// The ESO v1beta1 status schema:
//
//	status:
//	  conditions:
//	    - type: Ready
//	      status: "True" | "False"
//	      reason: SecretSynced | ...
//	      message: "..."
func (a *V1Beta1Adapter) ParseSyncStatus(obj *unstructured.Unstructured) *SyncStatus {
	if obj == nil {
		return &SyncStatus{Ready: false, Message: "ExternalSecret is nil"}
	}

	conditions, found, err := unstructured.NestedSlice(obj.Object, "status", "conditions")
	if err != nil || !found {
		return &SyncStatus{Ready: false, Message: "no status conditions yet; ESO may still be syncing"}
	}

	for _, c := range conditions {
		condMap, ok := c.(map[string]any)
		if !ok {
			continue
		}
		condType, _ := condMap["type"].(string)
		if condType != "Ready" {
			continue
		}
		condStatus, _ := condMap["status"].(string)
		message, _ := condMap["message"].(string)
		return &SyncStatus{
			Ready:   condStatus == "True",
			Message: message,
		}
	}

	return &SyncStatus{Ready: false, Message: "Ready condition not found in ExternalSecret status"}
}
