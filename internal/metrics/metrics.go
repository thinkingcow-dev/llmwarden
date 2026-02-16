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

package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

var (
	// LLMAccessTotal tracks the total number of LLMAccess resources by provider, namespace, and status
	LLMAccessTotal = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "llmwarden_llmaccess_total",
			Help: "Total number of LLMAccess resources by provider, namespace, and status",
		},
		[]string{"provider", "namespace", "status"},
	)

	// CredentialRotationsTotal counts the total number of credential rotations
	CredentialRotationsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "llmwarden_credential_rotations_total",
			Help: "Total number of credential rotations performed",
		},
		[]string{"provider", "namespace"},
	)

	// CredentialRotationErrors counts the total number of credential rotation errors
	CredentialRotationErrors = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "llmwarden_credential_rotation_errors_total",
			Help: "Total number of credential rotation errors",
		},
		[]string{"provider", "namespace", "error_type"},
	)

	// CredentialAge tracks the age of the current credential in seconds
	CredentialAge = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "llmwarden_credential_age_seconds",
			Help: "Age of the current credential in seconds",
		},
		[]string{"provider", "namespace", "name"},
	)

	// CredentialNextRotation tracks the time until next rotation in seconds
	CredentialNextRotation = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "llmwarden_credential_next_rotation_seconds",
			Help: "Time until next credential rotation in seconds",
		},
		[]string{"provider", "namespace", "name"},
	)

	// ProviderHealth tracks the health status of LLM providers
	ProviderHealth = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "llmwarden_provider_health",
			Help: "Health status of LLM providers (1 = healthy, 0 = unhealthy)",
		},
		[]string{"provider", "status"},
	)

	// WebhookInjectionsTotal counts the total number of webhook injections
	WebhookInjectionsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "llmwarden_webhook_injections_total",
			Help: "Total number of credential injections performed by the webhook",
		},
		[]string{"namespace", "provider"},
	)

	// ReconciliationDuration tracks the duration of reconciliation loops
	ReconciliationDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "llmwarden_reconciliation_duration_seconds",
			Help:    "Duration of reconciliation loops in seconds",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"controller", "result"},
	)

	// SecretProvisioningTotal counts the total number of secrets provisioned
	SecretProvisioningTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "llmwarden_secret_provisioning_total",
			Help: "Total number of secrets provisioned",
		},
		[]string{"provider", "namespace", "result"},
	)
)

func init() {
	// Register custom metrics with the controller-runtime metrics registry
	metrics.Registry.MustRegister(
		LLMAccessTotal,
		CredentialRotationsTotal,
		CredentialRotationErrors,
		CredentialAge,
		CredentialNextRotation,
		ProviderHealth,
		WebhookInjectionsTotal,
		ReconciliationDuration,
		SecretProvisioningTotal,
	)
}
