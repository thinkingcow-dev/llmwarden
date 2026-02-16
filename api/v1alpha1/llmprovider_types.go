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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ProviderType defines the LLM provider type
// +kubebuilder:validation:Enum=openai;anthropic;aws-bedrock;azure-openai;gcp-vertexai;custom
type ProviderType string

const (
	ProviderOpenAI      ProviderType = "openai"
	ProviderAnthropic   ProviderType = "anthropic"
	ProviderAWSBedrock  ProviderType = "aws-bedrock"
	ProviderAzureOpenAI ProviderType = "azure-openai"
	ProviderGCPVertexAI ProviderType = "gcp-vertexai"
	ProviderCustom      ProviderType = "custom"
)

// AuthType defines the authentication strategy type
// +kubebuilder:validation:Enum=apiKey;externalSecret;workloadIdentity
type AuthType string

const (
	AuthTypeAPIKey           AuthType = "apiKey"
	AuthTypeExternalSecret   AuthType = "externalSecret"
	AuthTypeWorkloadIdentity AuthType = "workloadIdentity"
)

// RotationStrategy defines the credential rotation strategy
// +kubebuilder:validation:Enum=providerAPI;recreateSecret
type RotationStrategy string

const (
	RotationStrategyProviderAPI    RotationStrategy = "providerAPI"
	RotationStrategyRecreateSecret RotationStrategy = "recreateSecret"
)

// SecretStoreKind defines the kind of secret store
// +kubebuilder:validation:Enum=SecretStore;ClusterSecretStore
type SecretStoreKind string

const (
	SecretStoreKindSecretStore        SecretStoreKind = "SecretStore"
	SecretStoreKindClusterSecretStore SecretStoreKind = "ClusterSecretStore"
)

// LLMProviderSpec defines the desired state of LLMProvider
type LLMProviderSpec struct {
	// Provider specifies which LLM provider this configuration is for
	// +kubebuilder:validation:Required
	Provider ProviderType `json:"provider"`

	// Auth defines the authentication strategy for accessing the LLM provider
	// +kubebuilder:validation:Required
	Auth AuthConfig `json:"auth"`

	// AllowedModels is a list of model names/IDs that can be accessed through this provider.
	// Empty list means all models are allowed.
	// +optional
	AllowedModels []string `json:"allowedModels,omitempty"`

	// RateLimit defines rate limiting configuration (informational/enforced by webhook)
	// +optional
	RateLimit *RateLimitConfig `json:"rateLimit,omitempty"`

	// NamespaceSelector determines which namespaces can create LLMAccess resources
	// referencing this provider. Empty selector means all namespaces are allowed.
	// +optional
	NamespaceSelector *metav1.LabelSelector `json:"namespaceSelector,omitempty"`

	// Endpoint allows overriding the provider's default endpoint
	// (e.g., for proxies or private endpoints)
	// +optional
	Endpoint *EndpointConfig `json:"endpoint,omitempty"`
}

// AuthConfig defines the authentication configuration
type AuthConfig struct {
	// Type specifies the authentication strategy to use
	// +kubebuilder:validation:Required
	Type AuthType `json:"type"`

	// APIKey configuration for direct API key authentication
	// Required when type is "apiKey"
	// +optional
	APIKey *APIKeyAuth `json:"apiKey,omitempty"`

	// ExternalSecret configuration for External Secrets Operator integration
	// Required when type is "externalSecret"
	// +optional
	ExternalSecret *ExternalSecretAuth `json:"externalSecret,omitempty"`

	// WorkloadIdentity configuration for cloud-native secretless auth
	// Required when type is "workloadIdentity"
	// +optional
	WorkloadIdentity *WorkloadIdentityAuth `json:"workloadIdentity,omitempty"`
}

// APIKeyAuth defines API key authentication configuration
type APIKeyAuth struct {
	// SecretRef references an existing Kubernetes Secret containing the API key
	// +kubebuilder:validation:Required
	SecretRef SecretReference `json:"secretRef"`

	// Rotation defines credential rotation policy
	// +optional
	Rotation *RotationConfig `json:"rotation,omitempty"`
}

// SecretReference defines a reference to a Kubernetes Secret
type SecretReference struct {
	// Name of the secret
	// +kubebuilder:validation:Required
	Name string `json:"name"`

	// Namespace of the secret
	// +kubebuilder:validation:Required
	Namespace string `json:"namespace"`

	// Key within the secret that contains the API key
	// +kubebuilder:validation:Required
	Key string `json:"key"`
}

// RotationConfig defines credential rotation configuration
type RotationConfig struct {
	// Enabled determines whether automatic rotation is enabled
	// +kubebuilder:default=false
	Enabled bool `json:"enabled"`

	// Interval is the duration between credential rotations (e.g., "30d", "7d")
	// +kubebuilder:validation:Pattern=`^\d+[dhm]$`
	// +optional
	Interval string `json:"interval,omitempty"`

	// Strategy defines how rotation is performed
	// +kubebuilder:default=providerAPI
	// +optional
	Strategy RotationStrategy `json:"strategy,omitempty"`
}

// ExternalSecretAuth defines External Secrets Operator configuration
type ExternalSecretAuth struct {
	// Store references the SecretStore or ClusterSecretStore
	// +kubebuilder:validation:Required
	Store StoreReference `json:"store"`

	// RemoteRef defines the reference to the secret in the external store
	// +kubebuilder:validation:Required
	RemoteRef RemoteReference `json:"remoteRef"`

	// RefreshInterval is how often to check for secret updates
	// +kubebuilder:validation:Pattern=`^\d+[hms]$`
	// +kubebuilder:default="1h"
	// +optional
	RefreshInterval string `json:"refreshInterval,omitempty"`
}

// StoreReference references a SecretStore or ClusterSecretStore
type StoreReference struct {
	// Name of the SecretStore/ClusterSecretStore
	// +kubebuilder:validation:Required
	Name string `json:"name"`

	// Kind of the store (SecretStore or ClusterSecretStore)
	// +kubebuilder:validation:Required
	Kind SecretStoreKind `json:"kind"`
}

// RemoteReference defines how to find the secret in the external store
type RemoteReference struct {
	// Key is the key/path to the secret in the external store
	// +kubebuilder:validation:Required
	Key string `json:"key"`

	// Property is the property/field within the secret to use
	// +optional
	Property string `json:"property,omitempty"`
}

// WorkloadIdentityAuth defines cloud workload identity configuration
type WorkloadIdentityAuth struct {
	// AWS configuration for IRSA (IAM Roles for Service Accounts)
	// +optional
	AWS *AWSWorkloadIdentity `json:"aws,omitempty"`

	// Azure configuration for Azure Workload Identity
	// +optional
	Azure *AzureWorkloadIdentity `json:"azure,omitempty"`

	// GCP configuration for Workload Identity Federation
	// +optional
	GCP *GCPWorkloadIdentity `json:"gcp,omitempty"`
}

// AWSWorkloadIdentity defines AWS IRSA configuration
type AWSWorkloadIdentity struct {
	// RoleArn is the ARN of the IAM role to assume
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Pattern=`^arn:aws:iam::\d{12}:role/.*$`
	RoleArn string `json:"roleArn"`

	// Region is the AWS region
	// +kubebuilder:validation:Required
	Region string `json:"region"`
}

// AzureWorkloadIdentity defines Azure Workload Identity configuration
type AzureWorkloadIdentity struct {
	// ClientId is the Azure AD application client ID
	// +kubebuilder:validation:Required
	ClientId string `json:"clientId"`

	// TenantId is the Azure AD tenant ID
	// +kubebuilder:validation:Required
	TenantId string `json:"tenantId"`

	// ManagedIdentityResourceId is the resource ID of the managed identity (for user-assigned)
	// +optional
	ManagedIdentityResourceId string `json:"managedIdentityResourceId,omitempty"`
}

// GCPWorkloadIdentity defines GCP Workload Identity configuration
type GCPWorkloadIdentity struct {
	// ServiceAccountEmail is the GCP service account email
	// +kubebuilder:validation:Required
	ServiceAccountEmail string `json:"serviceAccountEmail"`

	// ProjectId is the GCP project ID
	// +kubebuilder:validation:Required
	ProjectId string `json:"projectId"`
}

// RateLimitConfig defines rate limiting configuration
type RateLimitConfig struct {
	// RequestsPerMinute is the max number of requests per minute
	// +kubebuilder:validation:Minimum=0
	// +optional
	RequestsPerMinute *int64 `json:"requestsPerMinute,omitempty"`

	// TokensPerMinute is the max number of tokens per minute
	// +kubebuilder:validation:Minimum=0
	// +optional
	TokensPerMinute *int64 `json:"tokensPerMinute,omitempty"`
}

// EndpointConfig defines endpoint configuration
type EndpointConfig struct {
	// BaseURL is the base URL for the provider API
	// Empty string means use provider default
	// +optional
	BaseURL string `json:"baseURL,omitempty"`
}

// LLMProviderStatus defines the observed state of LLMProvider
type LLMProviderStatus struct {
	// Conditions represent the current state of the LLMProvider resource
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// LastCredentialCheck is the timestamp of the last credential validation check
	// +optional
	LastCredentialCheck *metav1.Time `json:"lastCredentialCheck,omitempty"`

	// AccessCount is the number of LLMAccess resources referencing this provider
	// +optional
	AccessCount int32 `json:"accessCount,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster,shortName=llmp
// +kubebuilder:printcolumn:name="Provider",type=string,JSONPath=`.spec.provider`
// +kubebuilder:printcolumn:name="Auth Type",type=string,JSONPath=`.spec.auth.type`
// +kubebuilder:printcolumn:name="Access Count",type=integer,JSONPath=`.status.accessCount`
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// LLMProvider is the Schema for the llmproviders API.
// It declares an available LLM provider and its authentication configuration.
type LLMProvider struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// spec defines the desired state of LLMProvider
	// +required
	Spec LLMProviderSpec `json:"spec"`

	// status defines the observed state of LLMProvider
	// +optional
	Status LLMProviderStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// LLMProviderList contains a list of LLMProvider
type LLMProviderList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []LLMProvider `json:"items"`
}

func init() {
	SchemeBuilder.Register(&LLMProvider{}, &LLMProviderList{})
}
