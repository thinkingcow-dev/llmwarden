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

package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// LLMAccessSpec defines the desired state of LLMAccess
type LLMAccessSpec struct {
	// ProviderRef references the cluster-scoped LLMProvider resource
	// +kubebuilder:validation:Required
	ProviderRef ProviderReference `json:"providerRef"`

	// Models is a list of model names/IDs that this access requires.
	// Must be a subset of the provider's allowedModels.
	// +kubebuilder:validation:MinItems=1
	// +optional
	Models []string `json:"models,omitempty"`

	// SecretName is the name of the Kubernetes Secret to create in this namespace
	// containing the credentials
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	SecretName string `json:"secretName"`

	// WorkloadSelector determines which pods receive credential injection via webhook
	// +optional
	WorkloadSelector *metav1.LabelSelector `json:"workloadSelector,omitempty"`

	// Injection defines how credentials are injected into matching pods
	// +kubebuilder:validation:Required
	Injection InjectionConfig `json:"injection"`

	// Rotation allows overriding the provider's rotation schedule
	// The interval must be less than or equal to the provider's interval
	// +optional
	Rotation *AccessRotationConfig `json:"rotation,omitempty"`
}

// ProviderReference references a cluster-scoped LLMProvider
type ProviderReference struct {
	// Name of the LLMProvider resource
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`
}

// InjectionConfig defines how credentials are injected into pods
type InjectionConfig struct {
	// Env defines environment variable injection
	// +optional
	Env []EnvVarMapping `json:"env,omitempty"`

	// Volume defines volume mount injection
	// +optional
	Volume *VolumeInjection `json:"volume,omitempty"`
}

// EnvVarMapping defines mapping from secret key to environment variable
type EnvVarMapping struct {
	// Name is the environment variable name to set in the pod
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// SecretKey is the key in the generated secret to map from
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	SecretKey string `json:"secretKey"`
}

// VolumeInjection defines volume mount configuration for credential injection
type VolumeInjection struct {
	// MountPath is where to mount the secret volume in the pod
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	MountPath string `json:"mountPath"`

	// ReadOnly determines if the volume should be mounted read-only
	// +kubebuilder:default=true
	// +optional
	ReadOnly bool `json:"readOnly,omitempty"`
}

// AccessRotationConfig defines rotation configuration for this LLMAccess
type AccessRotationConfig struct {
	// Interval is the duration between credential rotations (e.g., "7d", "24h")
	// Must be less than or equal to the provider's rotation interval
	// +kubebuilder:validation:Pattern=`^\d+[dhm]$`
	// +optional
	Interval string `json:"interval,omitempty"`
}

// LLMAccessStatus defines the observed state of LLMAccess
type LLMAccessStatus struct {
	// Conditions represent the current state of the LLMAccess resource
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// SecretRef references the created Secret containing credentials
	// +optional
	SecretRef *corev1.ObjectReference `json:"secretRef,omitempty"`

	// LastRotation is the timestamp of the last credential rotation
	// +optional
	LastRotation *metav1.Time `json:"lastRotation,omitempty"`

	// NextRotation is the timestamp of the next scheduled rotation
	// +optional
	NextRotation *metav1.Time `json:"nextRotation,omitempty"`

	// ProvisionedModels is the list of models that have been successfully provisioned
	// +optional
	ProvisionedModels []string `json:"provisionedModels,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=llma
// +kubebuilder:printcolumn:name="Provider",type=string,JSONPath=`.spec.providerRef.name`
// +kubebuilder:printcolumn:name="Secret",type=string,JSONPath=`.spec.secretName`
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="Last Rotation",type=date,JSONPath=`.status.lastRotation`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// LLMAccess is the Schema for the llmaccesses API.
// It requests access to an LLM provider for a workload in a namespace.
type LLMAccess struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// spec defines the desired state of LLMAccess
	// +required
	Spec LLMAccessSpec `json:"spec"`

	// status defines the observed state of LLMAccess
	// +optional
	Status LLMAccessStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// LLMAccessList contains a list of LLMAccess
type LLMAccessList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []LLMAccess `json:"items"`
}

func init() {
	SchemeBuilder.Register(&LLMAccess{}, &LLMAccessList{})
}
