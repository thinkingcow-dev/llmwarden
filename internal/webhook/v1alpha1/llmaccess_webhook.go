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

	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	llmwardenv1alpha1 "github.com/tpbansal/llmwarden/api/v1alpha1"
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

	// TODO(user): fill in your validation logic upon object creation.

	return nil, nil
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
