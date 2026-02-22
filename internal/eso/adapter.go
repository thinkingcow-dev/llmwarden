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

// Package eso provides an abstraction layer for External Secrets Operator (ESO) integration.
// The Adapter interface is the single point of change when migrating between ESO API versions.
// All provisioner logic operates against our internal types; only the adapter translates
// to/from the concrete ESO API resource structure.
package eso

import (
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// SecretCreationPolicy controls how ESO manages the target Kubernetes Secret.
type SecretCreationPolicy string

const (
	// SecretCreationPolicyOwner creates the Secret and sets the ExternalSecret as owner.
	// The Secret is deleted when the ExternalSecret is deleted.
	SecretCreationPolicyOwner SecretCreationPolicy = "Owner"

	// SecretCreationPolicyOrphan creates the Secret but does not set an owner reference.
	SecretCreationPolicyOrphan SecretCreationPolicy = "Orphan"

	// SecretCreationPolicyMerge merges data into an existing Secret instead of creating one.
	SecretCreationPolicyMerge SecretCreationPolicy = "Merge"

	// SecretCreationPolicyNone does not create a Secret; data is exposed via the ExternalSecret status.
	SecretCreationPolicyNone SecretCreationPolicy = "None"
)

// ExternalSecretSpec is our internal, version-agnostic representation of an ESO ExternalSecret spec.
// Keeping this type stable means provisioner code never needs to change when ESO API versions evolve —
// only the Adapter implementation needs updating.
type ExternalSecretSpec struct {
	// RefreshInterval is how often ESO polls the external store (e.g., "1h", "5m", "30s").
	RefreshInterval string

	// StoreRef references the SecretStore or ClusterSecretStore that backs this secret.
	StoreRef StoreRef

	// Target defines the resulting Kubernetes Secret.
	Target ExternalSecretTarget

	// Data maps individual keys from the external store to local secret keys.
	Data []ExternalSecretData
}

// StoreRef references a SecretStore or ClusterSecretStore resource.
type StoreRef struct {
	// Name of the SecretStore/ClusterSecretStore resource.
	Name string

	// Kind is "SecretStore" for namespace-scoped or "ClusterSecretStore" for cluster-scoped.
	Kind string
}

// ExternalSecretTarget defines the Kubernetes Secret that ESO will create/manage.
type ExternalSecretTarget struct {
	// Name of the Kubernetes Secret to create/update.
	Name string

	// CreationPolicy controls Secret lifecycle relative to the ExternalSecret.
	CreationPolicy SecretCreationPolicy
}

// ExternalSecretData maps a single remote secret reference to a local secret key.
type ExternalSecretData struct {
	// SecretKey is the key name in the resulting Kubernetes Secret.
	SecretKey string

	// RemoteRef locates the value in the external store.
	RemoteRef RemoteRef
}

// RemoteRef defines how to look up a value in the external store.
type RemoteRef struct {
	// Key is the path/name of the secret in the external store.
	Key string

	// Property is an optional field/property within a multi-value secret.
	// Leave empty to use the entire secret value.
	Property string

	// Version is an optional version/tag of the secret. Leave empty for the latest.
	Version string
}

// SyncStatus represents the current synchronization status of an ExternalSecret.
type SyncStatus struct {
	// Ready indicates whether ESO has successfully synced the secret.
	Ready bool

	// Message provides human-readable details about the current sync state.
	Message string
}

// Adapter converts our internal ExternalSecretSpec into versioned ESO API objects.
// Implement a new Adapter (e.g., V1Adapter) to target a different ESO API version
// without touching any provisioner logic.
type Adapter interface {
	// GVK returns the GroupVersionKind for the ExternalSecret resource this adapter targets.
	GVK() schema.GroupVersionKind

	// Build constructs an unstructured ExternalSecret object from our internal spec.
	// The caller is responsible for setting owner references after Build().
	Build(namespace, name string, labels map[string]string, spec ExternalSecretSpec) *unstructured.Unstructured

	// ParseSyncStatus extracts synchronization status from an existing ExternalSecret object.
	// Returns a best-effort status; never returns nil.
	ParseSyncStatus(obj *unstructured.Unstructured) *SyncStatus
}
