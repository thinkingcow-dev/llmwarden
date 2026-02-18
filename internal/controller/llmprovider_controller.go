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
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	llmwardenv1alpha1 "github.com/tpbansal/llmwarden/api/v1alpha1"
	"github.com/tpbansal/llmwarden/internal/metrics"
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

	// Determine provider health status based on conditions
	healthStatus := r.getProviderHealthStatus(provider)
	if healthStatus == "healthy" {
		metrics.ProviderHealth.WithLabelValues(provider.Name, "healthy").Set(1)
		metrics.ProviderHealth.WithLabelValues(provider.Name, "unhealthy").Set(0)
		r.Recorder.Event(provider, corev1.EventTypeNormal, "ProviderHealthy",
			"LLM provider is healthy and ready")
	} else {
		metrics.ProviderHealth.WithLabelValues(provider.Name, "healthy").Set(0)
		metrics.ProviderHealth.WithLabelValues(provider.Name, "unhealthy").Set(1)
		r.Recorder.Event(provider, corev1.EventTypeWarning, "ProviderUnhealthy",
			"LLM provider health check failed")
	}

	// Count LLMAccess resources that reference this provider
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
		// Update status with access count if needed
		if provider.Status.AccessCount != accessCount {
			provider.Status.AccessCount = accessCount
			if err := r.Status().Update(ctx, provider); err != nil {
				log.Error(err, "Failed to update provider status")
			}
		}
	}

	metrics.ReconciliationDuration.WithLabelValues("llmprovider", "success").Observe(time.Since(startTime).Seconds())
	log.V(1).Info("Successfully reconciled LLMProvider", "name", provider.Name)

	return ctrl.Result{}, nil
}

// getProviderHealthStatus determines the health status of a provider based on its conditions
func (r *LLMProviderReconciler) getProviderHealthStatus(provider *llmwardenv1alpha1.LLMProvider) string {
	// Check if Ready condition exists and is True
	for _, condition := range provider.Status.Conditions {
		if condition.Type == "Ready" && condition.Status == metav1.ConditionTrue {
			return "healthy"
		}
	}
	return "unhealthy"
}

// SetupWithManager sets up the controller with the Manager.
func (r *LLMProviderReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&llmwardenv1alpha1.LLMProvider{}).
		Named("llmprovider").
		Complete(r)
}
