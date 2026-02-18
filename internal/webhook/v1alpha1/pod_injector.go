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
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	llmwardenv1alpha1 "github.com/thinkingcow-dev/llmwarden/api/v1alpha1"
	"github.com/thinkingcow-dev/llmwarden/internal/metrics"
)

const (
	// InjectedProvidersAnnotation is the annotation key for tracking injected providers
	InjectedProvidersAnnotation = "llmwarden.io/injected-providers"

	// InjectionStatusAnnotation indicates injection status
	InjectionStatusAnnotation = "llmwarden.io/injection-status"
)

// log is for logging in this package.
var podinjectorlog = logf.Log.WithName("pod-injector")

// +kubebuilder:webhook:path=/mutate-v1-pod,mutating=true,failurePolicy=ignore,sideEffects=None,groups="",resources=pods,verbs=create,versions=v1,name=mpod.llmwarden.io,admissionReviewVersions=v1

// PodInjector injects LLM credentials into pods based on LLMAccess workload selectors.
type PodInjector struct {
	Client  client.Client
	decoder admission.Decoder
}

// Handle processes incoming pod creation requests and injects credentials.
func (i *PodInjector) Handle(ctx context.Context, req admission.Request) admission.Response {
	pod := &corev1.Pod{}

	err := i.decoder.Decode(req, pod)
	if err != nil {
		return admission.Errored(http.StatusBadRequest, fmt.Errorf("failed to decode pod: %w", err))
	}

	podinjectorlog.Info("Processing pod", "name", pod.Name, "namespace", pod.Namespace)

	// List all LLMAccess resources in the pod's namespace
	llmAccessList := &llmwardenv1alpha1.LLMAccessList{}
	if err := i.Client.List(ctx, llmAccessList, client.InNamespace(req.Namespace)); err != nil {
		podinjectorlog.Error(err, "Failed to list LLMAccess resources", "namespace", req.Namespace)
		// Use failurePolicy=ignore so we don't block pod creation if there's an error
		return admission.Allowed("failed to list LLMAccess resources, allowing pod creation")
	}

	if len(llmAccessList.Items) == 0 {
		// No LLMAccess resources in this namespace, nothing to inject
		return admission.Allowed("no LLMAccess resources in namespace")
	}

	// Track which providers we inject
	var injectedProviders []string
	modified := false

	// Check each LLMAccess to see if it matches this pod
	for _, llmAccess := range llmAccessList.Items {
		if i.shouldInject(pod, &llmAccess) {
			podinjectorlog.Info("Injecting credentials",
				"pod", pod.Name,
				"llmaccess", llmAccess.Name,
				"provider", llmAccess.Spec.ProviderRef.Name)

			if err := i.injectCredentials(pod, &llmAccess); err != nil {
				podinjectorlog.Error(err, "Failed to inject credentials",
					"pod", pod.Name,
					"llmaccess", llmAccess.Name)
				continue
			}

			injectedProviders = append(injectedProviders, llmAccess.Spec.ProviderRef.Name)
			// Track successful injection in metrics
			metrics.WebhookInjectionsTotal.WithLabelValues(req.Namespace, llmAccess.Spec.ProviderRef.Name).Inc()
			modified = true
		}
	}

	if !modified {
		// No matching LLMAccess resources for this pod
		return admission.Allowed("no matching LLMAccess resources")
	}

	// Add annotations to track injection
	if pod.Annotations == nil {
		pod.Annotations = make(map[string]string)
	}
	pod.Annotations[InjectedProvidersAnnotation] = strings.Join(injectedProviders, ",")
	pod.Annotations[InjectionStatusAnnotation] = "injected"

	// Marshal the modified pod
	marshaledPod, err := json.Marshal(pod)
	if err != nil {
		return admission.Errored(http.StatusInternalServerError, fmt.Errorf("failed to marshal pod: %w", err))
	}

	podinjectorlog.Info("Successfully injected credentials",
		"pod", pod.Name,
		"providers", strings.Join(injectedProviders, ","))

	return admission.PatchResponseFromRaw(req.Object.Raw, marshaledPod)
}

// shouldInject determines if credentials should be injected into the pod based on the workload selector.
func (i *PodInjector) shouldInject(pod *corev1.Pod, llmAccess *llmwardenv1alpha1.LLMAccess) bool {
	// If no workload selector is defined, don't inject
	if llmAccess.Spec.WorkloadSelector == nil {
		return false
	}

	// Convert label selector to labels.Selector
	selector, err := metav1.LabelSelectorAsSelector(llmAccess.Spec.WorkloadSelector)
	if err != nil {
		podinjectorlog.Error(err, "Failed to parse workload selector",
			"llmaccess", llmAccess.Name)
		return false
	}

	// Check if pod labels match the selector
	return selector.Matches(labels.Set(pod.Labels))
}

// injectCredentials injects environment variables and/or volumes into the pod.
func (i *PodInjector) injectCredentials(pod *corev1.Pod, llmAccess *llmwardenv1alpha1.LLMAccess) error {
	// Inject environment variables if configured
	if len(llmAccess.Spec.Injection.Env) > 0 {
		if err := i.injectEnvVars(pod, llmAccess); err != nil {
			return fmt.Errorf("failed to inject env vars: %w", err)
		}
	}

	// Inject volume if configured
	if llmAccess.Spec.Injection.Volume != nil {
		if err := i.injectVolume(pod, llmAccess); err != nil {
			return fmt.Errorf("failed to inject volume: %w", err)
		}
	}

	return nil
}

// injectEnvVars injects environment variables into all containers in the pod.
func (i *PodInjector) injectEnvVars(pod *corev1.Pod, llmAccess *llmwardenv1alpha1.LLMAccess) error {
	secretName := llmAccess.Spec.SecretName

	// Create env vars from the mapping
	envVars := make([]corev1.EnvVar, 0, len(llmAccess.Spec.Injection.Env))
	for _, mapping := range llmAccess.Spec.Injection.Env {
		envVar := corev1.EnvVar{
			Name: mapping.Name,
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: secretName,
					},
					Key: mapping.SecretKey,
				},
			},
		}
		envVars = append(envVars, envVar)
	}

	// Inject into all containers
	for i := range pod.Spec.Containers {
		pod.Spec.Containers[i].Env = append(pod.Spec.Containers[i].Env, envVars...)
	}

	// Inject into all init containers
	for i := range pod.Spec.InitContainers {
		pod.Spec.InitContainers[i].Env = append(pod.Spec.InitContainers[i].Env, envVars...)
	}

	return nil
}

// injectVolume injects a volume mount into all containers in the pod.
func (i *PodInjector) injectVolume(pod *corev1.Pod, llmAccess *llmwardenv1alpha1.LLMAccess) error {
	volumeConfig := llmAccess.Spec.Injection.Volume
	secretName := llmAccess.Spec.SecretName

	// Create a unique volume name
	volumeName := fmt.Sprintf("llmwarden-%s", llmAccess.Name)

	// Add volume to pod spec
	volume := corev1.Volume{
		Name: volumeName,
		VolumeSource: corev1.VolumeSource{
			Secret: &corev1.SecretVolumeSource{
				SecretName: secretName,
			},
		},
	}
	pod.Spec.Volumes = append(pod.Spec.Volumes, volume)

	// Create volume mount
	volumeMount := corev1.VolumeMount{
		Name:      volumeName,
		MountPath: volumeConfig.MountPath,
		ReadOnly:  volumeConfig.ReadOnly,
	}

	// Add volume mount to all containers
	for i := range pod.Spec.Containers {
		pod.Spec.Containers[i].VolumeMounts = append(pod.Spec.Containers[i].VolumeMounts, volumeMount)
	}

	// Add volume mount to all init containers
	for i := range pod.Spec.InitContainers {
		pod.Spec.InitContainers[i].VolumeMounts = append(pod.Spec.InitContainers[i].VolumeMounts, volumeMount)
	}

	return nil
}

// InjectDecoder injects the decoder.
func (i *PodInjector) InjectDecoder(d admission.Decoder) error {
	i.decoder = d
	return nil
}
