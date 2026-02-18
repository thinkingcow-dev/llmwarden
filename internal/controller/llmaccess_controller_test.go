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
	"math/rand"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	llmwardenv1alpha1 "github.com/thinkingcow-dev/llmwarden/api/v1alpha1"
)

var _ = Describe("LLMAccess Controller", func() {
	const (
		timeout  = time.Second * 10
		interval = time.Millisecond * 250
	)

	Context("When reconciling an LLMAccess with apiKey auth", func() {
		var (
			ctx                  context.Context
			namespace            *corev1.Namespace
			providerNamespace    *corev1.Namespace
			provider             *llmwardenv1alpha1.LLMProvider
			providerSecret       *corev1.Secret
			llmAccess            *llmwardenv1alpha1.LLMAccess
			controllerReconciler *LLMAccessReconciler
		)

		BeforeEach(func() {
			ctx = context.Background()
			controllerReconciler = &LLMAccessReconciler{
				Client:   k8sClient,
				Scheme:   k8sClient.Scheme(),
				Recorder: record.NewFakeRecorder(100),
			}

			// Create provider namespace
			providerNamespace = &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "llmwarden-system-" + randString(5),
				},
			}
			Expect(k8sClient.Create(ctx, providerNamespace)).To(Succeed())

			// Create test namespace with labels
			namespace = &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-ns-" + randString(5),
					Labels: map[string]string{
						"ai-tier": "production",
					},
				},
			}
			Expect(k8sClient.Create(ctx, namespace)).To(Succeed())

			// Create provider secret
			providerSecret = &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "openai-key",
					Namespace: providerNamespace.Name,
				},
				Data: map[string][]byte{
					"api-key": []byte("sk-test-key-1234567890"),
				},
			}
			Expect(k8sClient.Create(ctx, providerSecret)).To(Succeed())

			// Create LLMProvider
			provider = &llmwardenv1alpha1.LLMProvider{
				ObjectMeta: metav1.ObjectMeta{
					Name: "openai-prod-" + randString(5),
				},
				Spec: llmwardenv1alpha1.LLMProviderSpec{
					Provider: llmwardenv1alpha1.ProviderOpenAI,
					Auth: llmwardenv1alpha1.AuthConfig{
						Type: llmwardenv1alpha1.AuthTypeAPIKey,
						APIKey: &llmwardenv1alpha1.APIKeyAuth{
							SecretRef: llmwardenv1alpha1.SecretReference{
								Name:      providerSecret.Name,
								Namespace: providerSecret.Namespace,
								Key:       "api-key",
							},
							Rotation: &llmwardenv1alpha1.RotationConfig{
								Enabled:  true,
								Interval: "7d",
								Strategy: llmwardenv1alpha1.RotationStrategyProviderAPI,
							},
						},
					},
					AllowedModels: []string{"gpt-4o", "gpt-4o-mini"},
					NamespaceSelector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"ai-tier": "production",
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, provider)).To(Succeed())
		})

		AfterEach(func() {
			// Cleanup in reverse order
			if llmAccess != nil {
				_ = k8sClient.Delete(ctx, llmAccess)
			}
			if provider != nil {
				_ = k8sClient.Delete(ctx, provider)
			}
			if providerSecret != nil {
				_ = k8sClient.Delete(ctx, providerSecret)
			}
			if namespace != nil {
				_ = k8sClient.Delete(ctx, namespace)
			}
			if providerNamespace != nil {
				_ = k8sClient.Delete(ctx, providerNamespace)
			}
		})

		It("should successfully provision credentials", func() {
			llmAccess = &llmwardenv1alpha1.LLMAccess{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "chatbot-access",
					Namespace: namespace.Name,
				},
				Spec: llmwardenv1alpha1.LLMAccessSpec{
					ProviderRef: llmwardenv1alpha1.ProviderReference{
						Name: provider.Name,
					},
					Models:     []string{"gpt-4o"},
					SecretName: "openai-credentials",
					Injection: llmwardenv1alpha1.InjectionConfig{
						Env: []llmwardenv1alpha1.EnvVarMapping{
							{
								Name:      "OPENAI_API_KEY",
								SecretKey: "apiKey",
							},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, llmAccess)).To(Succeed())

			// First reconcile - adds finalizer
			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      llmAccess.Name,
					Namespace: llmAccess.Namespace,
				},
			})
			Expect(err).NotTo(HaveOccurred())

			// Second reconcile - does actual provisioning
			_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      llmAccess.Name,
					Namespace: llmAccess.Namespace,
				},
			})
			Expect(err).NotTo(HaveOccurred())

			// Verify secret was created
			Eventually(func() error {
				secret := &corev1.Secret{}
				return k8sClient.Get(ctx, types.NamespacedName{
					Name:      "openai-credentials",
					Namespace: namespace.Name,
				}, secret)
			}, timeout, interval).Should(Succeed())

			// Verify secret contents
			secret := &corev1.Secret{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      "openai-credentials",
				Namespace: namespace.Name,
			}, secret)).To(Succeed())
			Expect(secret.Data).To(HaveKey("apiKey"))
			Expect(secret.Data["apiKey"]).To(Equal([]byte("sk-test-key-1234567890")))
			Expect(secret.Labels).To(HaveKeyWithValue("llmwarden.io/managed-by", "llmwarden"))
			Expect(secret.Labels).To(HaveKeyWithValue("llmwarden.io/provider", provider.Name))

			// Verify status conditions
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      llmAccess.Name,
					Namespace: llmAccess.Namespace,
				}, llmAccess)
				if err != nil {
					return false
				}

				for _, cond := range llmAccess.Status.Conditions {
					if cond.Type == ConditionTypeReady && cond.Status == metav1.ConditionTrue {
						return true
					}
				}
				return false
			}, timeout, interval).Should(BeTrue())

			// Verify status fields
			Expect(llmAccess.Status.SecretRef).NotTo(BeNil())
			Expect(llmAccess.Status.SecretRef.Name).To(Equal("openai-credentials"))
			Expect(llmAccess.Status.ProvisionedModels).To(Equal([]string{"gpt-4o"}))
			Expect(llmAccess.Status.LastRotation).NotTo(BeNil())
			Expect(llmAccess.Status.NextRotation).NotTo(BeNil())
		})

		It("should reject LLMAccess when namespace is not allowed", func() {
			// Create namespace without required label
			restrictedNs := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "restricted-ns-" + randString(5),
					Labels: map[string]string{
						"ai-tier": "development", // different label
					},
				},
			}
			Expect(k8sClient.Create(ctx, restrictedNs)).To(Succeed())
			defer func() {
				_ = k8sClient.Delete(ctx, restrictedNs)
			}()

			llmAccess = &llmwardenv1alpha1.LLMAccess{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "unauthorized-access",
					Namespace: restrictedNs.Name,
				},
				Spec: llmwardenv1alpha1.LLMAccessSpec{
					ProviderRef: llmwardenv1alpha1.ProviderReference{
						Name: provider.Name,
					},
					Models:     []string{"gpt-4o"},
					SecretName: "openai-credentials",
					Injection: llmwardenv1alpha1.InjectionConfig{
						Env: []llmwardenv1alpha1.EnvVarMapping{
							{Name: "OPENAI_API_KEY", SecretKey: "apiKey"},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, llmAccess)).To(Succeed())

			// First reconcile - adds finalizer
			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      llmAccess.Name,
					Namespace: llmAccess.Namespace,
				},
			})
			Expect(err).NotTo(HaveOccurred())

			// Second reconcile - does actual provisioning
			_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      llmAccess.Name,
					Namespace: llmAccess.Namespace,
				},
			})
			Expect(err).NotTo(HaveOccurred())

			// Verify Ready condition is False with NamespaceNotAllowed reason
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      llmAccess.Name,
					Namespace: llmAccess.Namespace,
				}, llmAccess)
				if err != nil {
					return false
				}

				for _, cond := range llmAccess.Status.Conditions {
					if cond.Type == ConditionTypeReady &&
						cond.Status == metav1.ConditionFalse &&
						cond.Reason == ReasonNamespaceNotAllowed {
						return true
					}
				}
				return false
			}, timeout, interval).Should(BeTrue())

			// Verify secret was NOT created
			secret := &corev1.Secret{}
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      "openai-credentials",
				Namespace: restrictedNs.Name,
			}, secret)
			Expect(apierrors.IsNotFound(err)).To(BeTrue())
		})

		It("should reject LLMAccess when model is not allowed", func() {
			llmAccess = &llmwardenv1alpha1.LLMAccess{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "unauthorized-model",
					Namespace: namespace.Name,
				},
				Spec: llmwardenv1alpha1.LLMAccessSpec{
					ProviderRef: llmwardenv1alpha1.ProviderReference{
						Name: provider.Name,
					},
					Models:     []string{"gpt-4-turbo"}, // not in allowedModels
					SecretName: "openai-credentials",
					Injection: llmwardenv1alpha1.InjectionConfig{
						Env: []llmwardenv1alpha1.EnvVarMapping{
							{Name: "OPENAI_API_KEY", SecretKey: "apiKey"},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, llmAccess)).To(Succeed())

			// First reconcile - adds finalizer
			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      llmAccess.Name,
					Namespace: llmAccess.Namespace,
				},
			})
			Expect(err).NotTo(HaveOccurred())

			// Second reconcile - does actual provisioning
			_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      llmAccess.Name,
					Namespace: llmAccess.Namespace,
				},
			})
			Expect(err).NotTo(HaveOccurred())

			// Verify Ready condition is False with ModelNotAllowed reason
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      llmAccess.Name,
					Namespace: llmAccess.Namespace,
				}, llmAccess)
				if err != nil {
					return false
				}

				for _, cond := range llmAccess.Status.Conditions {
					if cond.Type == ConditionTypeReady &&
						cond.Status == metav1.ConditionFalse &&
						cond.Reason == ReasonModelNotAllowed {
						return true
					}
				}
				return false
			}, timeout, interval).Should(BeTrue())
		})

		It("should handle provider not found gracefully", func() {
			llmAccess = &llmwardenv1alpha1.LLMAccess{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "missing-provider",
					Namespace: namespace.Name,
				},
				Spec: llmwardenv1alpha1.LLMAccessSpec{
					ProviderRef: llmwardenv1alpha1.ProviderReference{
						Name: "nonexistent-provider",
					},
					Models:     []string{"gpt-4o"},
					SecretName: "openai-credentials",
					Injection: llmwardenv1alpha1.InjectionConfig{
						Env: []llmwardenv1alpha1.EnvVarMapping{
							{Name: "OPENAI_API_KEY", SecretKey: "apiKey"},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, llmAccess)).To(Succeed())

			// First reconcile - adds finalizer
			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      llmAccess.Name,
					Namespace: llmAccess.Namespace,
				},
			})
			Expect(err).NotTo(HaveOccurred())

			// Second reconcile - tries to fetch provider and fails
			result, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      llmAccess.Name,
					Namespace: llmAccess.Namespace,
				},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(Equal(30 * time.Second))

			// Verify status reflects provider not found
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      llmAccess.Name,
					Namespace: llmAccess.Namespace,
				}, llmAccess)
				if err != nil {
					return false
				}

				for _, cond := range llmAccess.Status.Conditions {
					if cond.Type == ConditionTypeReady &&
						cond.Status == metav1.ConditionFalse &&
						cond.Reason == ReasonProviderNotFound {
						return true
					}
				}
				return false
			}, timeout, interval).Should(BeTrue())
		})

		It("should update secret when provider secret changes", func() {
			llmAccess = &llmwardenv1alpha1.LLMAccess{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "update-test",
					Namespace: namespace.Name,
				},
				Spec: llmwardenv1alpha1.LLMAccessSpec{
					ProviderRef: llmwardenv1alpha1.ProviderReference{
						Name: provider.Name,
					},
					Models:     []string{"gpt-4o"},
					SecretName: "openai-credentials",
					Injection: llmwardenv1alpha1.InjectionConfig{
						Env: []llmwardenv1alpha1.EnvVarMapping{
							{Name: "OPENAI_API_KEY", SecretKey: "apiKey"},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, llmAccess)).To(Succeed())

			// First reconcile
			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      llmAccess.Name,
					Namespace: llmAccess.Namespace,
				},
			})
			Expect(err).NotTo(HaveOccurred())

			// Update provider secret
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      providerSecret.Name,
				Namespace: providerSecret.Namespace,
			}, providerSecret)).To(Succeed())
			providerSecret.Data["api-key"] = []byte("sk-new-key-0987654321")
			Expect(k8sClient.Update(ctx, providerSecret)).To(Succeed())

			// Second reconcile
			_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      llmAccess.Name,
					Namespace: llmAccess.Namespace,
				},
			})
			Expect(err).NotTo(HaveOccurred())

			// Verify target secret was updated
			Eventually(func() []byte {
				secret := &corev1.Secret{}
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      "openai-credentials",
					Namespace: namespace.Name,
				}, secret)
				if err != nil {
					return nil
				}
				return secret.Data["apiKey"]
			}, timeout, interval).Should(Equal([]byte("sk-new-key-0987654321")))
		})
	})

	Context("Helper functions", func() {
		It("should parse duration strings correctly", func() {
			d, err := parseDuration("7d")
			Expect(err).NotTo(HaveOccurred())
			Expect(d).To(Equal(7 * 24 * time.Hour))

			d, err = parseDuration("24h")
			Expect(err).NotTo(HaveOccurred())
			Expect(d).To(Equal(24 * time.Hour))

			d, err = parseDuration("30m")
			Expect(err).NotTo(HaveOccurred())
			Expect(d).To(Equal(30 * time.Minute))

			_, err = parseDuration("invalid")
			Expect(err).To(HaveOccurred())

			_, err = parseDuration("7x")
			Expect(err).To(HaveOccurred())
		})
	})
})

// randString generates a random string of length 5
func randString(_ int) string {
	const letterBytes = "abcdefghijklmnopqrstuvwxyz0123456789"
	const length = 5
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	b := make([]byte, length)
	for i := range b {
		b[i] = letterBytes[rng.Intn(len(letterBytes))]
	}
	return string(b)
}
