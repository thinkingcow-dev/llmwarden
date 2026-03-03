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

// V1GVK is the GroupVersionKind for the ESO v1 ExternalSecret resource.
// ESO v0.17+ serves v1 as the primary version; v1beta1 is deprecated.
var V1GVK = schema.GroupVersionKind{
	Group:   "external-secrets.io",
	Version: "v1",
	Kind:    "ExternalSecret",
}

// V1Adapter implements Adapter for ESO API version v1.
// The v1 field structure is identical to v1beta1; only the API version differs.
type V1Adapter struct{}

// NewV1Adapter creates an Adapter targeting ESO v1.
func NewV1Adapter() *V1Adapter {
	return &V1Adapter{}
}

// GVK returns the ExternalSecret GroupVersionKind for ESO v1.
func (a *V1Adapter) GVK() schema.GroupVersionKind {
	return V1GVK
}

// Build constructs an unstructured ExternalSecret object for ESO v1.
// The spec fields are identical to v1beta1.
func (a *V1Adapter) Build(namespace, name string, labels map[string]string, spec ExternalSecretSpec) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(a.GVK())
	obj.SetNamespace(namespace)
	obj.SetName(name)
	obj.SetLabels(labels)

	obj.Object["spec"] = a.buildSpec(spec)

	return obj
}

// buildSpec converts our internal ExternalSecretSpec to the ESO v1 spec map.
// Field names are identical to v1beta1.
func (a *V1Adapter) buildSpec(spec ExternalSecretSpec) map[string]any {
	secretStoreRef := map[string]any{
		"name": spec.StoreRef.Name,
		"kind": spec.StoreRef.Kind,
	}

	target := map[string]any{
		"name":           spec.Target.Name,
		"creationPolicy": string(spec.Target.CreationPolicy),
	}

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

// ParseSyncStatus reads the sync status from an ESO v1 ExternalSecret object.
// The status schema is identical to v1beta1.
func (a *V1Adapter) ParseSyncStatus(obj *unstructured.Unstructured) *SyncStatus {
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
