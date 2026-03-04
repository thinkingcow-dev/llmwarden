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
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	llmwardenv1alpha1 "github.com/llmwarden/llmwarden/api/v1alpha1"
	"github.com/llmwarden/llmwarden/internal/metrics"
	"github.com/llmwarden/llmwarden/internal/provisioner"
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
	Scheme                    *runtime.Scheme
	Recorder                  record.EventRecorder
	ApiKeyProvisioner         *provisioner.ApiKeyProvisioner
	ExternalSecretProvisioner *provisioner.ExternalSecretProvisioner
}

// +kubebuilder:rbac:groups=llmwarden.io,resources=llmaccesses,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=llmwarden.io,resources=llmaccesses/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=llmwarden.io,resources=llmaccesses/finalizers,verbs=update
// +kubebuilder:rbac:groups=llmwarden.io,resources=llmproviders,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch
// +kubebuilder:rbac:groups="",resources=namespaces,verbs=get;list;watch
// +kubebuilder:rbac:groups=external-secrets.io,resources=externalsecrets,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *LLMAccessReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	startTime := time.Now()

	// Fetch the LLMAccess instance
	llmAccess := &llmwardenv1alpha1.LLMAccess{}
	if err := r.Get(ctx, req.NamespacedName, llmAccess); err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("LLMAccess resource not found, ignoring since object must be deleted")
			metrics.ReconciliationDuration.WithLabelValues("llmaccess", "success").Observe(time.Since(startTime).Seconds())
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Failed to get LLMAccess")
		metrics.ReconciliationDuration.WithLabelValues("llmaccess", "error").Observe(time.Since(startTime).Seconds())
		return ctrl.Result{}, err
	}

	// Handle deletion
	if !llmAccess.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(llmAccess, llmAccessFinalizer) {
			// Fetch the provider to determine which provisioner to call for cleanup.
			// The provider may already be deleted; if so, skip cleanup (owner references
			// on the owned Secret/ExternalSecret will GC them via Kubernetes).
			provider := &llmwardenv1alpha1.LLMProvider{}
			if err := r.Get(ctx, types.NamespacedName{Name: llmAccess.Spec.ProviderRef.Name}, provider); err == nil {
				if prov, err := r.selectProvisioner(provider.Spec.Auth.Type); err == nil {
					if cleanupErr := prov.Cleanup(ctx, provider, llmAccess); cleanupErr != nil {
						logger.Error(cleanupErr, "Failed to cleanup provisioner resources during deletion")
						// Don't block deletion on cleanup failures for the ESO path;
						// log and proceed so the finalizer can be removed.
					}
				}
			}
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
			logger.Error(err, "Referenced LLMProvider not found", "provider", llmAccess.Spec.ProviderRef.Name)
			r.Recorder.Event(llmAccess, corev1.EventTypeWarning, ReasonProviderNotFound,
				fmt.Sprintf("LLMProvider %s not found", llmAccess.Spec.ProviderRef.Name))
			setCondition(&llmAccess.Status.Conditions, llmAccess.Generation, ConditionTypeReady, metav1.ConditionFalse, ReasonProviderNotFound,
				fmt.Sprintf("LLMProvider %s not found", llmAccess.Spec.ProviderRef.Name))
			if err := r.Status().Update(ctx, llmAccess); err != nil {
				return ctrl.Result{}, fmt.Errorf("failed to update status: %w", err)
			}
			return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
		}
		return ctrl.Result{}, fmt.Errorf("failed to get LLMProvider: %w", err)
	}

	// Validate namespace is allowed
	if !r.isNamespaceAllowed(ctx, llmAccess.Namespace, provider) {
		logger.Info("Namespace not allowed by provider", "namespace", llmAccess.Namespace, "provider", provider.Name)
		r.Recorder.Event(llmAccess, corev1.EventTypeWarning, ReasonNamespaceNotAllowed,
			fmt.Sprintf("Namespace %s is not allowed by LLMProvider %s", llmAccess.Namespace, provider.Name))
		setCondition(&llmAccess.Status.Conditions, llmAccess.Generation, ConditionTypeReady, metav1.ConditionFalse, ReasonNamespaceNotAllowed,
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
		logger.Error(err, "Model validation failed")
		r.Recorder.Event(llmAccess, corev1.EventTypeWarning, ReasonModelNotAllowed, err.Error())
		setCondition(&llmAccess.Status.Conditions, llmAccess.Generation, ConditionTypeReady, metav1.ConditionFalse, ReasonModelNotAllowed, err.Error())
		if err := r.Status().Update(ctx, llmAccess); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to update status: %w", err)
		}
		// Don't requeue - this is a permanent error until user fixes the spec
		return ctrl.Result{}, nil
	}

	// Select the provisioner based on the provider's auth type.
	prov, err := r.selectProvisioner(provider.Spec.Auth.Type)
	if err != nil {
		logger.Info("Auth type not supported", "authType", provider.Spec.Auth.Type)
		setCondition(&llmAccess.Status.Conditions, llmAccess.Generation, ConditionTypeReady, metav1.ConditionFalse, ReasonAuthTypeNotSupported, err.Error())
		if statusErr := r.Status().Update(ctx, llmAccess); statusErr != nil {
			return ctrl.Result{}, fmt.Errorf("failed to update status: %w", statusErr)
		}
		// Permanent error — don't requeue until the spec changes.
		return ctrl.Result{}, nil
	}

	// Provision credentials via the selected provisioner.
	if _, err := prov.Provision(ctx, provider, llmAccess); err != nil {
		logger.Error(err, "Failed to provision secret")
		r.Recorder.Event(llmAccess, corev1.EventTypeWarning, ReasonSecretUpdateFailed,
			fmt.Sprintf("Failed to provision credentials: %v", err))
		setCondition(&llmAccess.Status.Conditions, llmAccess.Generation, ConditionTypeReady, metav1.ConditionFalse, ReasonReconciliationError,
			fmt.Sprintf("Failed to provision credentials: %v", err))
		setCondition(&llmAccess.Status.Conditions, llmAccess.Generation, ConditionTypeCredentialProvisioned, metav1.ConditionFalse, ReasonSecretUpdateFailed, err.Error())
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

	setCondition(&llmAccess.Status.Conditions, llmAccess.Generation, ConditionTypeCredentialProvisioned, metav1.ConditionTrue, ReasonSecretCreated,
		"Secret created/updated successfully")
	setCondition(&llmAccess.Status.Conditions, llmAccess.Generation, ConditionTypeReady, metav1.ConditionTrue, ReasonCredentialProvisioned,
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
	logger.Info("Successfully reconciled LLMAccess", "namespace", llmAccess.Namespace, "name", llmAccess.Name)

	// Requeue before next rotation
	if rotationInterval > 0 {
		return ctrl.Result{RequeueAfter: rotationInterval}, nil
	}

	return ctrl.Result{}, nil
}

// selectProvisioner returns the Provisioner implementation for the given auth type.
func (r *LLMAccessReconciler) selectProvisioner(authType llmwardenv1alpha1.AuthType) (provisioner.Provisioner, error) {
	switch authType {
	case llmwardenv1alpha1.AuthTypeAPIKey:
		if r.ApiKeyProvisioner == nil {
			return nil, fmt.Errorf("auth type %s: provisioner not configured", authType)
		}
		return r.ApiKeyProvisioner, nil
	case llmwardenv1alpha1.AuthTypeExternalSecret:
		if r.ExternalSecretProvisioner == nil {
			return nil, fmt.Errorf("auth type %s: provisioner not configured", authType)
		}
		return r.ExternalSecretProvisioner, nil
	default:
		return nil, fmt.Errorf("auth type %s is not supported", authType)
	}
}

// isNamespaceAllowed checks if the namespace is allowed by the provider's namespace selector
func (r *LLMAccessReconciler) isNamespaceAllowed(ctx context.Context, namespace string, provider *llmwardenv1alpha1.LLMProvider) bool {
	// If no selector is defined, all namespaces are allowed
	if provider.Spec.NamespaceSelector == nil {
		return true
	}

	// Get the namespace object to check its labels
	ns := &corev1.Namespace{}
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
// Maximum allowed: 365 days to prevent DoS via excessive durations
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
			// Prevent integer overflow and reject non-positive intervals ("0d" is ambiguous).
			if value <= 0 || value > 365 {
				return 0, fmt.Errorf("duration value out of range (1-365): %d", value)
			}
			unit = s[i:]
			break
		}
	}

	if unit == "" {
		return 0, fmt.Errorf("missing duration unit in: %s", s)
	}

	var duration time.Duration
	switch unit {
	case "d":
		duration = time.Duration(value) * 24 * time.Hour
	case "h":
		duration = time.Duration(value) * time.Hour
	case "m":
		duration = time.Duration(value) * time.Minute
	default:
		return 0, fmt.Errorf("unsupported duration unit: %s", unit)
	}

	// Additional safety check: max 365 days
	if duration > 365*24*time.Hour {
		return 0, fmt.Errorf("duration exceeds maximum allowed (365 days): %s", s)
	}

	return duration, nil
}

// providerRefNameField is the field index key for LLMAccess.spec.providerRef.name.
const providerRefNameField = ".spec.providerRef.name"

// SetupWithManager sets up the controller with the Manager.
func (r *LLMAccessReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// Register a field index on spec.providerRef.name so that mapProviderToAccesses can
	// use a targeted List (client.MatchingFields) instead of listing all LLMAccess resources
	// cluster-wide. Without this index, every LLMProvider change triggers an O(N) scan of all
	// LLMAccess objects across all namespaces, which does not scale.
	if err := mgr.GetFieldIndexer().IndexField(
		context.Background(),
		&llmwardenv1alpha1.LLMAccess{},
		providerRefNameField,
		func(obj client.Object) []string {
			access, ok := obj.(*llmwardenv1alpha1.LLMAccess)
			if !ok {
				return nil
			}
			return []string{access.Spec.ProviderRef.Name}
		},
	); err != nil {
		return fmt.Errorf("setting up providerRef.name field index: %w", err)
	}

	// Watch LLMProvider changes and enqueue only LLMAccess resources that reference the changed
	// provider. The field index makes this lookup O(matches) rather than O(total LLMAccess).
	mapProviderToAccesses := func(ctx context.Context, obj client.Object) []reconcile.Request {
		llmAccessList := &llmwardenv1alpha1.LLMAccessList{}
		if err := mgr.GetClient().List(ctx, llmAccessList,
			client.MatchingFields{providerRefNameField: obj.GetName()},
		); err != nil {
			return nil
		}
		reqs := make([]reconcile.Request, 0, len(llmAccessList.Items))
		for _, access := range llmAccessList.Items {
			reqs = append(reqs, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      access.Name,
					Namespace: access.Namespace,
				},
			})
		}
		return reqs
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&llmwardenv1alpha1.LLMAccess{}).
		Owns(&corev1.Secret{}).
		Watches(&llmwardenv1alpha1.LLMProvider{}, handler.EnqueueRequestsFromMapFunc(mapProviderToAccesses)).
		Named("llmaccess").
		Complete(r)
}
