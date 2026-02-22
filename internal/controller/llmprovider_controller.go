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

package controller

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	llmwardenv1alpha1 "github.com/thinkingcow-dev/llmwarden/api/v1alpha1"
	"github.com/thinkingcow-dev/llmwarden/internal/metrics"
)

// LLMProviderReconciler reconciles a LLMProvider object
type LLMProviderReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
}

// +kubebuilder:rbac:groups=llmwarden.io,resources=llmproviders,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=llmwarden.io,resources=llmproviders/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=llmwarden.io,resources=llmproviders/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch

const (
	providerRequeueInterval = 5 * time.Minute
	reasonInvalidConfig     = "InvalidConfig"
)

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *LLMProviderReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)
	startTime := time.Now()

	// Fetch the LLMProvider instance
	provider := &llmwardenv1alpha1.LLMProvider{}
	if err := r.Get(ctx, req.NamespacedName, provider); err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("LLMProvider resource not found, ignoring since object must be deleted")
			metrics.ReconciliationDuration.WithLabelValues("llmprovider", "success").Observe(time.Since(startTime).Seconds())
			return ctrl.Result{}, nil
		}
		log.Error(err, "Failed to get LLMProvider")
		metrics.ReconciliationDuration.WithLabelValues("llmprovider", "error").Observe(time.Since(startTime).Seconds())
		return ctrl.Result{}, err
	}

	// Validate provider config and set Ready condition
	condStatus, reason, message := r.validateProviderConfig(ctx, provider)
	r.setCondition(provider, "Ready", condStatus, reason, message)

	// Update LastCredentialCheck timestamp
	now := metav1.Now()
	provider.Status.LastCredentialCheck = &now

	// Count LLMAccess resources referencing this provider
	llmAccessList := &llmwardenv1alpha1.LLMAccessList{}
	if err := r.List(ctx, llmAccessList); err != nil {
		log.Error(err, "Failed to list LLMAccess resources")
	} else {
		accessCount := int32(0)
		for _, access := range llmAccessList.Items {
			if access.Spec.ProviderRef.Name == provider.Name {
				accessCount++
			}
		}
		provider.Status.AccessCount = accessCount
	}

	if err := r.Status().Update(ctx, provider); err != nil {
		log.Error(err, "Failed to update provider status")
		metrics.ReconciliationDuration.WithLabelValues("llmprovider", "error").Observe(time.Since(startTime).Seconds())
		return ctrl.Result{}, fmt.Errorf("failed to update provider status: %w", err)
	}

	// Update health metrics and emit event
	if condStatus == metav1.ConditionTrue {
		metrics.ProviderHealth.WithLabelValues(provider.Name, "healthy").Set(1)
		metrics.ProviderHealth.WithLabelValues(provider.Name, "unhealthy").Set(0)
		r.Recorder.Event(provider, corev1.EventTypeNormal, "ProviderHealthy",
			"LLM provider is healthy and ready")
	} else {
		metrics.ProviderHealth.WithLabelValues(provider.Name, "healthy").Set(0)
		metrics.ProviderHealth.WithLabelValues(provider.Name, "unhealthy").Set(1)
		r.Recorder.Event(provider, corev1.EventTypeWarning, "ProviderUnhealthy",
			fmt.Sprintf("LLM provider health check failed: %s", message))
	}

	metrics.ReconciliationDuration.WithLabelValues("llmprovider", "success").Observe(time.Since(startTime).Seconds())
	log.V(1).Info("Successfully reconciled LLMProvider", "name", provider.Name, "ready", condStatus)

	// Requeue periodically for health checks
	return ctrl.Result{RequeueAfter: providerRequeueInterval}, nil
}

// validateProviderConfig validates the provider's auth configuration and returns
// the condition status, reason, and message.
func (r *LLMProviderReconciler) validateProviderConfig(ctx context.Context, provider *llmwardenv1alpha1.LLMProvider) (metav1.ConditionStatus, string, string) {
	switch provider.Spec.Auth.Type {
	case llmwardenv1alpha1.AuthTypeAPIKey:
		return r.validateAPIKeyConfig(ctx, provider)
	case llmwardenv1alpha1.AuthTypeExternalSecret:
		return r.validateExternalSecretConfig(provider)
	case llmwardenv1alpha1.AuthTypeWorkloadIdentity:
		// Workload identity is Phase 3 — config is accepted but not validated
		return metav1.ConditionTrue, "WorkloadIdentityNotValidated",
			"WorkloadIdentity auth type accepted (validation implemented in Phase 3)"
	default:
		return metav1.ConditionFalse, "UnknownAuthType",
			fmt.Sprintf("Unknown auth type: %s", provider.Spec.Auth.Type)
	}
}

// validateAPIKeyConfig checks that the referenced secret exists and contains the expected key.
func (r *LLMProviderReconciler) validateAPIKeyConfig(ctx context.Context, provider *llmwardenv1alpha1.LLMProvider) (metav1.ConditionStatus, string, string) {
	if provider.Spec.Auth.APIKey == nil {
		return metav1.ConditionFalse, reasonInvalidConfig,
			"spec.auth.apiKey is required when spec.auth.type is apiKey"
	}

	ref := provider.Spec.Auth.APIKey.SecretRef
	secret := &corev1.Secret{}
	if err := r.Get(ctx, types.NamespacedName{Name: ref.Name, Namespace: ref.Namespace}, secret); err != nil {
		if apierrors.IsNotFound(err) {
			return metav1.ConditionFalse, "SecretNotFound",
				fmt.Sprintf("Provider secret %s/%s not found", ref.Namespace, ref.Name)
		}
		return metav1.ConditionFalse, "SecretGetError",
			fmt.Sprintf("Failed to get provider secret %s/%s: %v", ref.Namespace, ref.Name, err)
	}

	if _, exists := secret.Data[ref.Key]; !exists {
		return metav1.ConditionFalse, "SecretKeyMissing",
			fmt.Sprintf("Key %q not found in secret %s/%s", ref.Key, ref.Namespace, ref.Name)
	}

	return metav1.ConditionTrue, "SecretFound",
		fmt.Sprintf("Provider secret %s/%s exists and contains key %q", ref.Namespace, ref.Name, ref.Key)
}

// validateExternalSecretConfig validates that the externalSecret auth config is well-formed.
// It does not attempt to contact ESO — ESO may not be installed yet when the provider is created.
func (r *LLMProviderReconciler) validateExternalSecretConfig(provider *llmwardenv1alpha1.LLMProvider) (metav1.ConditionStatus, string, string) {
	cfg := provider.Spec.Auth.ExternalSecret
	if cfg == nil {
		return metav1.ConditionFalse, reasonInvalidConfig,
			"spec.auth.externalSecret is required when spec.auth.type is externalSecret"
	}

	if cfg.Store.Name == "" {
		return metav1.ConditionFalse, reasonInvalidConfig,
			"spec.auth.externalSecret.store.name must not be empty"
	}

	if cfg.Store.Kind != "SecretStore" && cfg.Store.Kind != "ClusterSecretStore" {
		return metav1.ConditionFalse, reasonInvalidConfig,
			fmt.Sprintf("spec.auth.externalSecret.store.kind must be SecretStore or ClusterSecretStore, got %q", cfg.Store.Kind)
	}

	if cfg.RemoteRef.Key == "" {
		return metav1.ConditionFalse, reasonInvalidConfig,
			"spec.auth.externalSecret.remoteRef.key must not be empty"
	}

	return metav1.ConditionTrue, "ExternalSecretConfigured",
		fmt.Sprintf("ExternalSecret configured: %s/%s → %s", cfg.Store.Kind, cfg.Store.Name, cfg.RemoteRef.Key)
}

// setCondition sets or updates a condition on the provider status.
func (r *LLMProviderReconciler) setCondition(provider *llmwardenv1alpha1.LLMProvider, conditionType string, status metav1.ConditionStatus, reason, message string) {
	now := metav1.Now()
	for i, cond := range provider.Status.Conditions {
		if cond.Type == conditionType {
			if cond.Status != status {
				provider.Status.Conditions[i].LastTransitionTime = now
			}
			provider.Status.Conditions[i].Status = status
			provider.Status.Conditions[i].Reason = reason
			provider.Status.Conditions[i].Message = message
			provider.Status.Conditions[i].ObservedGeneration = provider.Generation
			return
		}
	}
	provider.Status.Conditions = append(provider.Status.Conditions, metav1.Condition{
		Type:               conditionType,
		Status:             status,
		LastTransitionTime: now,
		Reason:             reason,
		Message:            message,
		ObservedGeneration: provider.Generation,
	})
}

// SetupWithManager sets up the controller with the Manager.
func (r *LLMProviderReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&llmwardenv1alpha1.LLMProvider{}).
		Named("llmprovider").
		Complete(r)
}
