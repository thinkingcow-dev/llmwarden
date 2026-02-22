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
	"fmt"

	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	llmwardenv1alpha1 "github.com/thinkingcow-dev/llmwarden/api/v1alpha1"
)

// nolint:unused
// log is for logging in this package.
var llmaccesslog = logf.Log.WithName("llmaccess-resource")

// SetupLLMAccessWebhookWithManager registers the webhook for LLMAccess in the manager.
func SetupLLMAccessWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr, &llmwardenv1alpha1.LLMAccess{}).
		WithValidator(&LLMAccessCustomValidator{}).
		WithDefaulter(&LLMAccessCustomDefaulter{}).
		Complete()
}

// SetupPodInjectorWebhookWithManager registers the pod injector webhook with the manager.
func SetupPodInjectorWebhookWithManager(mgr ctrl.Manager) error {
	decoder := admission.NewDecoder(mgr.GetScheme())

	podInjector := &PodInjector{
		Client:  mgr.GetClient(),
		decoder: decoder,
	}

	mgr.GetWebhookServer().Register("/mutate-v1-pod", &admission.Webhook{
		Handler: podInjector,
	})

	return nil
}

// TODO(user): EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!

// +kubebuilder:webhook:path=/mutate-llmwarden-io-v1alpha1-llmaccess,mutating=true,failurePolicy=fail,sideEffects=None,groups=llmwarden.io,resources=llmaccesses,verbs=create;update,versions=v1alpha1,name=mllmaccess-v1alpha1.kb.io,admissionReviewVersions=v1

// LLMAccessCustomDefaulter struct is responsible for setting default values on the custom resource of the
// Kind LLMAccess when those are created or updated.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as it is used only for temporary operations and does not need to be deeply copied.
type LLMAccessCustomDefaulter struct {
	// TODO(user): Add more fields as needed for defaulting
}

// Default implements webhook.CustomDefaulter so a webhook will be registered for the Kind LLMAccess.
func (d *LLMAccessCustomDefaulter) Default(_ context.Context, obj *llmwardenv1alpha1.LLMAccess) error {
	llmaccesslog.Info("Defaulting for LLMAccess", "name", obj.GetName())

	// TODO(user): fill in your defaulting logic.

	return nil
}

// TODO(user): change verbs to "verbs=create;update;delete" if you want to enable deletion validation.
// NOTE: If you want to customise the 'path', use the flags '--defaulting-path' or '--validation-path'.
// +kubebuilder:webhook:path=/validate-llmwarden-io-v1alpha1-llmaccess,mutating=false,failurePolicy=fail,sideEffects=None,groups=llmwarden.io,resources=llmaccesses,verbs=create;update,versions=v1alpha1,name=vllmaccess-v1alpha1.kb.io,admissionReviewVersions=v1

// LLMAccessCustomValidator struct is responsible for validating the LLMAccess resource
// when it is created, updated, or deleted.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as this struct is used only for temporary operations and does not need to be deeply copied.
type LLMAccessCustomValidator struct {
	// TODO(user): Add more fields as needed for validation
}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type LLMAccess.
func (v *LLMAccessCustomValidator) ValidateCreate(_ context.Context, obj *llmwardenv1alpha1.LLMAccess) (admission.Warnings, error) {
	llmaccesslog.Info("Validation for LLMAccess upon creation", "name", obj.GetName())

	var warnings admission.Warnings

	// Validate provider reference is not empty
	if obj.Spec.ProviderRef.Name == "" {
		return nil, fmt.Errorf("spec.providerRef.name cannot be empty")
	}

	// Validate secret name follows K8s naming conventions
	if obj.Spec.SecretName == "" {
		return nil, fmt.Errorf("spec.secretName cannot be empty")
	}

	// Validate injection configuration - must have at least env or volume
	if len(obj.Spec.Injection.Env) == 0 && obj.Spec.Injection.Volume == nil {
		return nil, fmt.Errorf("spec.injection must define at least one of: env or volume")
	}

	// Validate env var names don't conflict with common K8s env vars
	reservedEnvVars := map[string]bool{
		"KUBERNETES_SERVICE_HOST": true,
		"KUBERNETES_SERVICE_PORT": true,
		"HOSTNAME":                true,
		"HOME":                    true,
	}

	for _, envMapping := range obj.Spec.Injection.Env {
		if reservedEnvVars[envMapping.Name] {
			warnings = append(warnings, fmt.Sprintf("env var '%s' overrides reserved Kubernetes variable", envMapping.Name))
		}
		// Validate env var name format
		if !isValidEnvVarName(envMapping.Name) {
			return warnings, fmt.Errorf("invalid env var name: %s (must match [A-Z_][A-Z0-9_]*)", envMapping.Name)
		}
	}

	// Validate volume mount path is absolute
	if obj.Spec.Injection.Volume != nil {
		if obj.Spec.Injection.Volume.MountPath == "" {
			return warnings, fmt.Errorf("spec.injection.volume.mountPath cannot be empty")
		}
		if obj.Spec.Injection.Volume.MountPath[0] != '/' {
			return warnings, fmt.Errorf("spec.injection.volume.mountPath must be an absolute path")
		}
	}

	return warnings, nil
}

// isValidEnvVarName validates environment variable names according to POSIX standard
func isValidEnvVarName(name string) bool {
	if len(name) == 0 {
		return false
	}
	// First character must be A-Z or underscore
	if (name[0] < 'A' || name[0] > 'Z') && name[0] != '_' {
		return false
	}
	// Rest must be A-Z, 0-9, or underscore
	for i := 1; i < len(name); i++ {
		c := name[i]
		if (c < 'A' || c > 'Z') && (c < '0' || c > '9') && c != '_' {
			return false
		}
	}
	return true
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type LLMAccess.
func (v *LLMAccessCustomValidator) ValidateUpdate(_ context.Context, oldObj, newObj *llmwardenv1alpha1.LLMAccess) (admission.Warnings, error) {
	llmaccesslog.Info("Validation for LLMAccess upon update", "name", newObj.GetName())

	// TODO(user): fill in your validation logic upon object update.

	return nil, nil
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type LLMAccess.
func (v *LLMAccessCustomValidator) ValidateDelete(_ context.Context, obj *llmwardenv1alpha1.LLMAccess) (admission.Warnings, error) {
	llmaccesslog.Info("Validation for LLMAccess upon deletion", "name", obj.GetName())

	// TODO(user): fill in your validation logic upon object deletion.

	return nil, nil
}
