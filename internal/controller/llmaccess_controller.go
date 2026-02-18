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
	"strconv"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	llmwardenv1alpha1 "github.com/thinkingcow-dev/llmwarden/api/v1alpha1"
	"github.com/thinkingcow-dev/llmwarden/internal/metrics"
)

const (
	// Condition types
	ConditionTypeReady                 = "Ready"
	ConditionTypeCredentialProvisioned = "CredentialProvisioned"

	// Condition reasons
	ReasonProviderNotFound      = "ProviderNotFound"
	ReasonNamespaceNotAllowed   = "NamespaceNotAllowed"
	ReasonModelNotAllowed       = "ModelNotAllowed"
	ReasonAuthTypeNotSupported  = "AuthTypeNotSupported"
	ReasonProviderSecretMissing = "ProviderSecretMissing"
	ReasonSecretCreated         = "SecretCreated"
	ReasonSecretUpdateFailed    = "SecretUpdateFailed"
	ReasonCredentialProvisioned = "CredentialProvisioned"
	ReasonReconciliationError   = "ReconciliationError"

	// Finalizer
	llmAccessFinalizer = "llmwarden.io/finalizer"
)

// LLMAccessReconciler reconciles a LLMAccess object
type LLMAccessReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
}

// +kubebuilder:rbac:groups=llmwarden.io,resources=llmaccesses,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=llmwarden.io,resources=llmaccesses/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=llmwarden.io,resources=llmaccesses/finalizers,verbs=update
// +kubebuilder:rbac:groups=llmwarden.io,resources=llmproviders,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch
// +kubebuilder:rbac:groups="",resources=namespaces,verbs=get;list;watch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *LLMAccessReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)
	startTime := time.Now()

	// Fetch the LLMAccess instance
	llmAccess := &llmwardenv1alpha1.LLMAccess{}
	if err := r.Get(ctx, req.NamespacedName, llmAccess); err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("LLMAccess resource not found, ignoring since object must be deleted")
			metrics.ReconciliationDuration.WithLabelValues("llmaccess", "success").Observe(time.Since(startTime).Seconds())
			return ctrl.Result{}, nil
		}
		log.Error(err, "Failed to get LLMAccess")
		metrics.ReconciliationDuration.WithLabelValues("llmaccess", "error").Observe(time.Since(startTime).Seconds())
		return ctrl.Result{}, err
	}

	// Handle deletion
	if !llmAccess.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(llmAccess, llmAccessFinalizer) {
			// Cleanup logic here if needed (e.g., revoke credentials)
			controllerutil.RemoveFinalizer(llmAccess, llmAccessFinalizer)
			if err := r.Update(ctx, llmAccess); err != nil {
				return ctrl.Result{}, fmt.Errorf("failed to remove finalizer: %w", err)
			}
		}
		return ctrl.Result{}, nil
	}

	// Add finalizer if not present
	if !controllerutil.ContainsFinalizer(llmAccess, llmAccessFinalizer) {
		controllerutil.AddFinalizer(llmAccess, llmAccessFinalizer)
		if err := r.Update(ctx, llmAccess); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to add finalizer: %w", err)
		}
		return ctrl.Result{Requeue: true}, nil
	}

	// Fetch referenced LLMProvider
	provider := &llmwardenv1alpha1.LLMProvider{}
	providerKey := types.NamespacedName{Name: llmAccess.Spec.ProviderRef.Name}
	if err := r.Get(ctx, providerKey, provider); err != nil {
		if apierrors.IsNotFound(err) {
			log.Error(err, "Referenced LLMProvider not found", "provider", llmAccess.Spec.ProviderRef.Name)
			r.Recorder.Event(llmAccess, corev1.EventTypeWarning, ReasonProviderNotFound,
				fmt.Sprintf("LLMProvider %s not found", llmAccess.Spec.ProviderRef.Name))
			r.setCondition(llmAccess, ConditionTypeReady, metav1.ConditionFalse, ReasonProviderNotFound,
				fmt.Sprintf("LLMProvider %s not found", llmAccess.Spec.ProviderRef.Name))
			if err := r.Status().Update(ctx, llmAccess); err != nil {
				return ctrl.Result{}, fmt.Errorf("failed to update status: %w", err)
			}
			return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
		}
		return ctrl.Result{}, fmt.Errorf("failed to get LLMProvider: %w", err)
	}

	// Validate namespace is allowed
	if !r.isNamespaceAllowed(llmAccess.Namespace, provider) {
		log.Info("Namespace not allowed by provider", "namespace", llmAccess.Namespace, "provider", provider.Name)
		r.Recorder.Event(llmAccess, corev1.EventTypeWarning, ReasonNamespaceNotAllowed,
			fmt.Sprintf("Namespace %s is not allowed by LLMProvider %s", llmAccess.Namespace, provider.Name))
		r.setCondition(llmAccess, ConditionTypeReady, metav1.ConditionFalse, ReasonNamespaceNotAllowed,
			fmt.Sprintf("Namespace %s is not allowed by LLMProvider %s", llmAccess.Namespace, provider.Name))
		if err := r.Status().Update(ctx, llmAccess); err != nil {
			metrics.ReconciliationDuration.WithLabelValues("llmaccess", "error").Observe(time.Since(startTime).Seconds())
			return ctrl.Result{}, fmt.Errorf("failed to update status: %w", err)
		}
		metrics.LLMAccessTotal.WithLabelValues(provider.Name, llmAccess.Namespace, "namespace_not_allowed").Set(1)
		metrics.ReconciliationDuration.WithLabelValues("llmaccess", "error").Observe(time.Since(startTime).Seconds())
		// Don't requeue - this is a permanent error until user fixes the provider or moves namespace
		return ctrl.Result{}, nil
	}

	// Validate requested models
	if err := r.validateModels(llmAccess.Spec.Models, provider); err != nil {
		log.Error(err, "Model validation failed")
		r.Recorder.Event(llmAccess, corev1.EventTypeWarning, ReasonModelNotAllowed, err.Error())
		r.setCondition(llmAccess, ConditionTypeReady, metav1.ConditionFalse, ReasonModelNotAllowed, err.Error())
		if err := r.Status().Update(ctx, llmAccess); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to update status: %w", err)
		}
		// Don't requeue - this is a permanent error until user fixes the spec
		return ctrl.Result{}, nil
	}

	// For MVP, only support apiKey auth type
	if provider.Spec.Auth.Type != llmwardenv1alpha1.AuthTypeAPIKey {
		log.Info("Auth type not supported in MVP", "authType", provider.Spec.Auth.Type)
		r.setCondition(llmAccess, ConditionTypeReady, metav1.ConditionFalse, ReasonAuthTypeNotSupported,
			fmt.Sprintf("Auth type %s not yet supported (MVP supports apiKey only)", provider.Spec.Auth.Type))
		if err := r.Status().Update(ctx, llmAccess); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to update status: %w", err)
		}
		return ctrl.Result{RequeueAfter: 5 * time.Minute}, nil
	}

	// Provision credentials (copy secret from provider namespace to access namespace)
	if err := r.provisionAPIKeySecret(ctx, llmAccess, provider); err != nil {
		log.Error(err, "Failed to provision secret")
		r.Recorder.Event(llmAccess, corev1.EventTypeWarning, ReasonSecretUpdateFailed,
			fmt.Sprintf("Failed to provision credentials: %v", err))
		r.setCondition(llmAccess, ConditionTypeReady, metav1.ConditionFalse, ReasonReconciliationError,
			fmt.Sprintf("Failed to provision credentials: %v", err))
		r.setCondition(llmAccess, ConditionTypeCredentialProvisioned, metav1.ConditionFalse, ReasonSecretUpdateFailed, err.Error())
		if err := r.Status().Update(ctx, llmAccess); err != nil {
			metrics.ReconciliationDuration.WithLabelValues("llmaccess", "error").Observe(time.Since(startTime).Seconds())
			return ctrl.Result{}, fmt.Errorf("failed to update status: %w", err)
		}
		metrics.SecretProvisioningTotal.WithLabelValues(provider.Name, llmAccess.Namespace, "error").Inc()
		metrics.LLMAccessTotal.WithLabelValues(provider.Name, llmAccess.Namespace, "error").Set(1)
		metrics.ReconciliationDuration.WithLabelValues("llmaccess", "error").Observe(time.Since(startTime).Seconds())
		return ctrl.Result{RequeueAfter: 30 * time.Second}, err
	}

	// Update status - credentials provisioned successfully
	now := metav1.Now()
	llmAccess.Status.SecretRef = &corev1.ObjectReference{
		Kind:      "Secret",
		Namespace: llmAccess.Namespace,
		Name:      llmAccess.Spec.SecretName,
	}
	llmAccess.Status.LastRotation = &now
	llmAccess.Status.ProvisionedModels = llmAccess.Spec.Models

	// Calculate next rotation time
	rotationInterval := r.getRotationInterval(llmAccess, provider)
	if rotationInterval > 0 {
		nextRotation := metav1.NewTime(now.Add(rotationInterval))
		llmAccess.Status.NextRotation = &nextRotation
	}

	r.setCondition(llmAccess, ConditionTypeCredentialProvisioned, metav1.ConditionTrue, ReasonSecretCreated,
		"Secret created/updated successfully")
	r.setCondition(llmAccess, ConditionTypeReady, metav1.ConditionTrue, ReasonCredentialProvisioned,
		"Credentials provisioned and ready")

	if err := r.Status().Update(ctx, llmAccess); err != nil {
		metrics.ReconciliationDuration.WithLabelValues("llmaccess", "error").Observe(time.Since(startTime).Seconds())
		return ctrl.Result{}, fmt.Errorf("failed to update status: %w", err)
	}

	// Emit success event
	r.Recorder.Event(llmAccess, corev1.EventTypeNormal, ReasonCredentialProvisioned,
		fmt.Sprintf("Successfully provisioned credentials for provider %s", provider.Name))

	// Update metrics for successful reconciliation
	metrics.SecretProvisioningTotal.WithLabelValues(provider.Name, llmAccess.Namespace, "success").Inc()
	metrics.LLMAccessTotal.WithLabelValues(provider.Name, llmAccess.Namespace, "ready").Set(1)

	// Track credential age
	if llmAccess.Status.LastRotation != nil {
		age := time.Since(llmAccess.Status.LastRotation.Time).Seconds()
		metrics.CredentialAge.WithLabelValues(provider.Name, llmAccess.Namespace, llmAccess.Name).Set(age)
	}

	// Track time until next rotation
	if llmAccess.Status.NextRotation != nil {
		nextRotationSeconds := time.Until(llmAccess.Status.NextRotation.Time).Seconds()
		metrics.CredentialNextRotation.WithLabelValues(provider.Name, llmAccess.Namespace, llmAccess.Name).Set(nextRotationSeconds)
	}

	metrics.ReconciliationDuration.WithLabelValues("llmaccess", "success").Observe(time.Since(startTime).Seconds())
	log.Info("Successfully reconciled LLMAccess", "namespace", llmAccess.Namespace, "name", llmAccess.Name)

	// Requeue before next rotation
	if rotationInterval > 0 {
		return ctrl.Result{RequeueAfter: rotationInterval}, nil
	}

	return ctrl.Result{}, nil
}

// isNamespaceAllowed checks if the namespace is allowed by the provider's namespace selector
func (r *LLMAccessReconciler) isNamespaceAllowed(namespace string, provider *llmwardenv1alpha1.LLMProvider) bool {
	// If no selector is defined, all namespaces are allowed
	if provider.Spec.NamespaceSelector == nil {
		return true
	}

	// Get the namespace object to check its labels
	ns := &corev1.Namespace{}
	ctx := context.Background()
	if err := r.Get(ctx, types.NamespacedName{Name: namespace}, ns); err != nil {
		return false
	}

	selector, err := metav1.LabelSelectorAsSelector(provider.Spec.NamespaceSelector)
	if err != nil {
		return false
	}

	return selector.Matches(labels.Set(ns.Labels))
}

// validateModels checks if requested models are allowed by the provider
func (r *LLMAccessReconciler) validateModels(requestedModels []string, provider *llmwardenv1alpha1.LLMProvider) error {
	// If no models are restricted (empty allowedModels), all models are allowed
	if len(provider.Spec.AllowedModels) == 0 {
		return nil
	}

	// Check each requested model is in the allowed list
	allowedMap := make(map[string]bool)
	for _, model := range provider.Spec.AllowedModels {
		allowedMap[model] = true
	}

	var notAllowed []string
	for _, model := range requestedModels {
		if !allowedMap[model] {
			notAllowed = append(notAllowed, model)
		}
	}

	if len(notAllowed) > 0 {
		return fmt.Errorf("models not allowed: %s (allowed models: %s)",
			strings.Join(notAllowed, ", "),
			strings.Join(provider.Spec.AllowedModels, ", "))
	}

	return nil
}

// provisionAPIKeySecret copies the secret from the provider namespace to the access namespace
func (r *LLMAccessReconciler) provisionAPIKeySecret(ctx context.Context, llmAccess *llmwardenv1alpha1.LLMAccess, provider *llmwardenv1alpha1.LLMProvider) error {
	log := log.FromContext(ctx)

	if provider.Spec.Auth.APIKey == nil {
		return fmt.Errorf("provider %s does not have apiKey configuration", provider.Name)
	}

	// Fetch the source secret from the provider's namespace
	sourceSecret := &corev1.Secret{}
	sourceKey := types.NamespacedName{
		Name:      provider.Spec.Auth.APIKey.SecretRef.Name,
		Namespace: provider.Spec.Auth.APIKey.SecretRef.Namespace,
	}
	if err := r.Get(ctx, sourceKey, sourceSecret); err != nil {
		if apierrors.IsNotFound(err) {
			return fmt.Errorf("provider secret %s/%s not found: %w", sourceKey.Namespace, sourceKey.Name, err)
		}
		return fmt.Errorf("failed to get provider secret: %w", err)
	}

	// Verify the key exists in the source secret
	secretKey := provider.Spec.Auth.APIKey.SecretRef.Key
	if _, exists := sourceSecret.Data[secretKey]; !exists {
		return fmt.Errorf("key %s not found in secret %s/%s", secretKey, sourceKey.Namespace, sourceKey.Name)
	}

	// Create or update the target secret in the LLMAccess namespace
	targetSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      llmAccess.Spec.SecretName,
			Namespace: llmAccess.Namespace,
		},
	}

	result, err := controllerutil.CreateOrUpdate(ctx, r.Client, targetSecret, func() error {
		// Set owner reference for garbage collection
		if err := controllerutil.SetControllerReference(llmAccess, targetSecret, r.Scheme); err != nil {
			return fmt.Errorf("failed to set owner reference: %w", err)
		}

		// Copy the secret data
		// Create a map with keys that the injection config expects
		if targetSecret.Data == nil {
			targetSecret.Data = make(map[string][]byte)
		}

		// Copy the API key with a standard key name
		targetSecret.Data["apiKey"] = sourceSecret.Data[secretKey]

		// Add additional metadata that might be useful
		if targetSecret.StringData == nil {
			targetSecret.StringData = make(map[string]string)
		}

		// Add base URL if configured
		if provider.Spec.Endpoint != nil && provider.Spec.Endpoint.BaseURL != "" {
			targetSecret.StringData["baseUrl"] = provider.Spec.Endpoint.BaseURL
		}

		// Add provider type for reference
		targetSecret.StringData["provider"] = string(provider.Spec.Provider)

		// Add labels for tracking
		if targetSecret.Labels == nil {
			targetSecret.Labels = make(map[string]string)
		}
		targetSecret.Labels["llmwarden.io/managed-by"] = "llmwarden"
		targetSecret.Labels["llmwarden.io/provider"] = provider.Name
		targetSecret.Labels["llmwarden.io/access"] = llmAccess.Name

		return nil
	})

	if err != nil {
		return fmt.Errorf("failed to create/update secret: %w", err)
	}

	log.Info("Secret reconciled", "result", result, "secret", targetSecret.Name)
	return nil
}

// getRotationInterval calculates the rotation interval for this LLMAccess
func (r *LLMAccessReconciler) getRotationInterval(llmAccess *llmwardenv1alpha1.LLMAccess, provider *llmwardenv1alpha1.LLMProvider) time.Duration {
	// Check if LLMAccess has a rotation override
	if llmAccess.Spec.Rotation != nil && llmAccess.Spec.Rotation.Interval != "" {
		if duration, err := parseDuration(llmAccess.Spec.Rotation.Interval); err == nil {
			return duration
		}
	}

	// Use provider's rotation interval if configured and enabled
	if provider.Spec.Auth.APIKey != nil &&
		provider.Spec.Auth.APIKey.Rotation != nil &&
		provider.Spec.Auth.APIKey.Rotation.Enabled &&
		provider.Spec.Auth.APIKey.Rotation.Interval != "" {
		if duration, err := parseDuration(provider.Spec.Auth.APIKey.Rotation.Interval); err == nil {
			return duration
		}
	}

	// Default: no rotation
	return 0
}

// parseDuration parses duration strings like "30d", "7d", "24h"
func parseDuration(s string) (time.Duration, error) {
	if s == "" {
		return 0, fmt.Errorf("empty duration string")
	}

	// Extract number and unit
	var value int
	var unit string

	for i, r := range s {
		if r < '0' || r > '9' {
			if i == 0 {
				return 0, fmt.Errorf("invalid duration format: %s", s)
			}
			var err error
			value, err = strconv.Atoi(s[:i])
			if err != nil {
				return 0, fmt.Errorf("invalid duration value: %w", err)
			}
			unit = s[i:]
			break
		}
	}

	if unit == "" {
		return 0, fmt.Errorf("missing duration unit in: %s", s)
	}

	switch unit {
	case "d":
		return time.Duration(value) * 24 * time.Hour, nil
	case "h":
		return time.Duration(value) * time.Hour, nil
	case "m":
		return time.Duration(value) * time.Minute, nil
	default:
		return 0, fmt.Errorf("unsupported duration unit: %s", unit)
	}
}

// setCondition sets a condition on the LLMAccess status
func (r *LLMAccessReconciler) setCondition(llmAccess *llmwardenv1alpha1.LLMAccess, conditionType string, status metav1.ConditionStatus, reason, message string) {
	now := metav1.Now()

	// Find existing condition
	for i, condition := range llmAccess.Status.Conditions {
		if condition.Type == conditionType {
			// Update existing condition only if status changed
			if condition.Status != status {
				llmAccess.Status.Conditions[i].Status = status
				llmAccess.Status.Conditions[i].LastTransitionTime = now
			}
			llmAccess.Status.Conditions[i].Reason = reason
			llmAccess.Status.Conditions[i].Message = message
			llmAccess.Status.Conditions[i].ObservedGeneration = llmAccess.Generation
			return
		}
	}

	// Add new condition
	llmAccess.Status.Conditions = append(llmAccess.Status.Conditions, metav1.Condition{
		Type:               conditionType,
		Status:             status,
		LastTransitionTime: now,
		Reason:             reason,
		Message:            message,
		ObservedGeneration: llmAccess.Generation,
	})
}

// SetupWithManager sets up the controller with the Manager.
func (r *LLMAccessReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&llmwardenv1alpha1.LLMAccess{}).
		Owns(&corev1.Secret{}).
		Named("llmaccess").
		Complete(r)
}
