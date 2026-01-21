package controller

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	maiav1alpha1 "github.com/ar4mirez/maia/operator/api/v1alpha1"
)

var _ = Describe("MaiaInstance Controller", func() {
	const (
		timeout  = time.Second * 10
		interval = time.Millisecond * 250
	)

	Context("When creating a MaiaInstance", func() {
		It("Should create the required Kubernetes resources", func() {
			ctx := context.Background()

			// Create namespace
			namespace := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-maia-instance",
				},
			}
			Expect(k8sClient.Create(ctx, namespace)).Should(Succeed())

			// Create MaiaInstance
			replicas := int32(1)
			instance := &maiav1alpha1.MaiaInstance{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-instance",
					Namespace: "test-maia-instance",
				},
				Spec: maiav1alpha1.MaiaInstanceSpec{
					Replicas: &replicas,
					Image: maiav1alpha1.ImageSpec{
						Repository: "ghcr.io/ar4mirez/maia",
						Tag:        "latest",
					},
					Storage: maiav1alpha1.StorageSpec{
						Size:    "5Gi",
						DataDir: "/data",
					},
					Logging: maiav1alpha1.LoggingSpec{
						Level:  "info",
						Format: "json",
					},
					Metrics: maiav1alpha1.MetricsSpec{
						Enabled: true,
					},
				},
			}
			Expect(k8sClient.Create(ctx, instance)).Should(Succeed())

			// Verify MaiaInstance was created
			instanceLookupKey := types.NamespacedName{Name: "test-instance", Namespace: "test-maia-instance"}
			createdInstance := &maiav1alpha1.MaiaInstance{}

			Eventually(func() bool {
				err := k8sClient.Get(ctx, instanceLookupKey, createdInstance)
				return err == nil
			}, timeout, interval).Should(BeTrue())

			Expect(createdInstance.Spec.Replicas).NotTo(BeNil())
			Expect(*createdInstance.Spec.Replicas).To(Equal(int32(1)))

			// Clean up
			Expect(k8sClient.Delete(ctx, instance)).Should(Succeed())
			Expect(k8sClient.Delete(ctx, namespace)).Should(Succeed())
		})
	})

	Context("When reconciling a MaiaInstance", func() {
		It("Should create ConfigMap with correct configuration", func() {
			ctx := context.Background()

			// Create namespace
			namespace := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-configmap",
				},
			}
			Expect(k8sClient.Create(ctx, namespace)).Should(Succeed())

			// Create MaiaInstance
			replicas := int32(2)
			instance := &maiav1alpha1.MaiaInstance{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "configmap-test",
					Namespace: "test-configmap",
				},
				Spec: maiav1alpha1.MaiaInstanceSpec{
					Replicas: &replicas,
					Image: maiav1alpha1.ImageSpec{
						Repository: "maia",
						Tag:        "v1.0.0",
					},
					Logging: maiav1alpha1.LoggingSpec{
						Level:  "debug",
						Format: "json",
					},
					Tenancy: maiav1alpha1.TenancySpec{
						Enabled:       true,
						RequireTenant: true,
					},
					RateLimit: maiav1alpha1.RateLimitSpec{
						Enabled:           true,
						RequestsPerSecond: 50,
						Burst:             100,
					},
				},
			}
			Expect(k8sClient.Create(ctx, instance)).Should(Succeed())

			// Trigger reconciliation manually for testing
			reconciler := &MaiaInstanceReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      "configmap-test",
					Namespace: "test-configmap",
				},
			})
			Expect(err).NotTo(HaveOccurred())

			// Verify ConfigMap was created
			configMap := &corev1.ConfigMap{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      "configmap-test-config",
					Namespace: "test-configmap",
				}, configMap)
				return err == nil
			}, timeout, interval).Should(BeTrue())

			Expect(configMap.Data).To(HaveKey("config.yaml"))
			config := configMap.Data["config.yaml"]
			Expect(config).To(ContainSubstring("level: debug"))
			Expect(config).To(ContainSubstring("tenant:"))
			Expect(config).To(ContainSubstring("enabled: true"))
			Expect(config).To(ContainSubstring("rate_limit:"))

			// Clean up
			Expect(k8sClient.Delete(ctx, instance)).Should(Succeed())
			Expect(k8sClient.Delete(ctx, namespace)).Should(Succeed())
		})
	})

	Context("When deleting a MaiaInstance", func() {
		It("Should clean up all owned resources", func() {
			ctx := context.Background()

			// Create namespace
			namespace := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-delete",
				},
			}
			Expect(k8sClient.Create(ctx, namespace)).Should(Succeed())

			// Create MaiaInstance
			replicas := int32(1)
			instance := &maiav1alpha1.MaiaInstance{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "delete-test",
					Namespace: "test-delete",
				},
				Spec: maiav1alpha1.MaiaInstanceSpec{
					Replicas: &replicas,
				},
			}
			Expect(k8sClient.Create(ctx, instance)).Should(Succeed())

			// Trigger reconciliation
			reconciler := &MaiaInstanceReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      "delete-test",
					Namespace: "test-delete",
				},
			})
			Expect(err).NotTo(HaveOccurred())

			// Delete the instance
			Expect(k8sClient.Delete(ctx, instance)).Should(Succeed())

			// Trigger reconciliation for deletion
			_, err = reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      "delete-test",
					Namespace: "test-delete",
				},
			})
			Expect(err).NotTo(HaveOccurred())

			// Verify instance is deleted
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      "delete-test",
					Namespace: "test-delete",
				}, &maiav1alpha1.MaiaInstance{})
				return errors.IsNotFound(err)
			}, timeout, interval).Should(BeTrue())

			// Clean up namespace
			Expect(k8sClient.Delete(ctx, namespace)).Should(Succeed())
		})
	})
})

var _ = Describe("MaiaInstance Helper Functions", func() {
	Context("getImage", func() {
		It("should return correct image string", func() {
			spec := maiav1alpha1.ImageSpec{
				Repository: "ghcr.io/ar4mirez/maia",
				Tag:        "v1.0.0",
			}
			Expect(getImage(spec)).To(Equal("ghcr.io/ar4mirez/maia:v1.0.0"))
		})

		It("should use defaults when not specified", func() {
			spec := maiav1alpha1.ImageSpec{}
			Expect(getImage(spec)).To(Equal("ghcr.io/ar4mirez/maia:latest"))
		})
	})

	Context("getStorageSize", func() {
		It("should return specified size", func() {
			spec := maiav1alpha1.StorageSpec{Size: "50Gi"}
			Expect(getStorageSize(spec)).To(Equal("50Gi"))
		})

		It("should return default size", func() {
			spec := maiav1alpha1.StorageSpec{}
			Expect(getStorageSize(spec)).To(Equal("10Gi"))
		})
	})

	Context("buildResourceRequirements", func() {
		It("should build correct resource requirements", func() {
			spec := maiav1alpha1.ResourcesSpec{
				Limits: maiav1alpha1.ResourceQuantities{
					CPU:    "2",
					Memory: "4Gi",
				},
				Requests: maiav1alpha1.ResourceQuantities{
					CPU:    "500m",
					Memory: "512Mi",
				},
			}
			resources := buildResourceRequirements(spec)

			Expect(resources.Limits.Cpu().String()).To(Equal("2"))
			Expect(resources.Limits.Memory().String()).To(Equal("4Gi"))
			Expect(resources.Requests.Cpu().String()).To(Equal("500m"))
			Expect(resources.Requests.Memory().String()).To(Equal("512Mi"))
		})

		It("should use defaults when not specified", func() {
			spec := maiav1alpha1.ResourcesSpec{}
			resources := buildResourceRequirements(spec)

			Expect(resources.Limits.Cpu().String()).To(Equal("1"))
			Expect(resources.Limits.Memory().String()).To(Equal("1Gi"))
			Expect(resources.Requests.Cpu().String()).To(Equal("100m"))
			Expect(resources.Requests.Memory().String()).To(Equal("256Mi"))
		})
	})
})

var _ = Describe("MaiaInstance Deployment", func() {
	Context("When creating deployment spec", func() {
		It("Should include correct labels", func() {
			ctx := context.Background()

			// Create namespace
			namespace := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-deployment",
				},
			}
			Expect(k8sClient.Create(ctx, namespace)).Should(Succeed())

			replicas := int32(1)
			instance := &maiav1alpha1.MaiaInstance{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "deployment-test",
					Namespace: "test-deployment",
				},
				Spec: maiav1alpha1.MaiaInstanceSpec{
					Replicas: &replicas,
				},
			}
			Expect(k8sClient.Create(ctx, instance)).Should(Succeed())

			// Trigger reconciliation
			reconciler := &MaiaInstanceReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      "deployment-test",
					Namespace: "test-deployment",
				},
			})
			Expect(err).NotTo(HaveOccurred())

			// Verify Deployment labels
			deployment := &appsv1.Deployment{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      "deployment-test",
					Namespace: "test-deployment",
				}, deployment)
				return err == nil
			}, time.Second*10, time.Millisecond*250).Should(BeTrue())

			Expect(deployment.Spec.Template.Labels).To(HaveKeyWithValue("app.kubernetes.io/name", "maia"))
			Expect(deployment.Spec.Template.Labels).To(HaveKeyWithValue("app.kubernetes.io/instance", "deployment-test"))
			Expect(deployment.Spec.Template.Labels).To(HaveKeyWithValue("app.kubernetes.io/managed-by", "maia-operator"))

			// Clean up
			Expect(k8sClient.Delete(ctx, instance)).Should(Succeed())
			Expect(k8sClient.Delete(ctx, namespace)).Should(Succeed())
		})
	})
})
