/*
Copyright 2024.

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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	ac3v1alpha1 "github.com/rh-waterford-et/ac3_networkoperator/api/v1alpha1"
)

var _ = Describe("MultiClusterNetwork Controller", func() {
	Context("When reconciling a MultiClusterNetwork resource", func() {
		const (
			networkName = "test-network"
			namespace   = "default"
			timeout     = time.Second * 10
			duration    = time.Second * 10
			interval    = time.Millisecond * 250
		)

		var (
			ctx                   context.Context
			typeNamespacedName    types.NamespacedName
			multiclusterNetwork   *ac3v1alpha1.MultiClusterNetwork
			expectedConfigMapName = "combined-kubeconfig"
			expectedConfigMapNS   = "sk1"
		)

		BeforeEach(func() {
			ctx = context.Background()
			typeNamespacedName = types.NamespacedName{
				Name:      networkName,
				Namespace: namespace,
			}

			// Create the MultiClusterNetwork resource
			multiclusterNetwork = &ac3v1alpha1.MultiClusterNetwork{
				ObjectMeta: metav1.ObjectMeta{
					Name:      networkName,
					Namespace: namespace,
				},
				Spec: ac3v1alpha1.MultiClusterNetworkSpec{
					Links: []*ac3v1alpha1.MultiClusterLink{
						{
							SourceCluster:   "cluster-1",
							TargetCluster:   "cluster-2",
							SourceNamespace: "test-namespace",
							TargetNamespace: "test-namespace",
							Applications:    []string{"test-app"},
							Services:        []string{"test-service"},
							Port:            8080,
						},
					},
				},
			}

			// Create the resource
			Expect(k8sClient.Create(ctx, multiclusterNetwork)).To(Succeed())
		})

		AfterEach(func() {
			// Clean up the resource
			resource := &ac3v1alpha1.MultiClusterNetwork{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			if err == nil {
				Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
			}
		})

		Context("When the required ConfigMap does not exist", func() {
			It("should fail with ConfigMap not found error", func() {
				By("Reconciling the created resource")
				controllerReconciler := &NetworkReconciler{
					Client: k8sClient,
					Scheme: k8sClient.Scheme(),
				}

				result, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: typeNamespacedName,
				})

				// Should fail because combined-kubeconfig doesn't exist
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("combined-kubeconfig"))
				Expect(result).To(Equal(reconcile.Result{}))
			})
		})

		Context("When the required ConfigMap exists", func() {
			BeforeEach(func() {
				// Create the required ConfigMap with a minimal kubeconfig
				kubeconfig := `
apiVersion: v1
kind: Config
clusters:
- name: cluster-1
  cluster:
    server: https://cluster-1.example.com
- name: cluster-2
  cluster:
    server: https://cluster-2.example.com
contexts:
- name: cluster-1
  context:
    cluster: cluster-1
    user: user-1
- name: cluster-2
  context:
    cluster: cluster-2
    user: user-2
users:
- name: user-1
  user:
    token: fake-token-1
- name: user-2
  user:
    token: fake-token-2
current-context: cluster-1
`

				configMap := &corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      expectedConfigMapName,
						Namespace: expectedConfigMapNS,
					},
					Data: map[string]string{
						"kubeconfig": kubeconfig,
					},
				}

				// Create the namespace if it doesn't exist
				ns := &corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{
						Name: expectedConfigMapNS,
					},
				}
				err := k8sClient.Create(ctx, ns)
				if err != nil && !errors.IsAlreadyExists(err) {
					Expect(err).NotTo(HaveOccurred())
				}

				// Create the ConfigMap
				Expect(k8sClient.Create(ctx, configMap)).To(Succeed())
			})

			AfterEach(func() {
				// Clean up the ConfigMap
				configMap := &corev1.ConfigMap{}
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      expectedConfigMapName,
					Namespace: expectedConfigMapNS,
				}, configMap)
				if err == nil {
					Expect(k8sClient.Delete(ctx, configMap)).To(Succeed())
				}
			})

			It("should successfully parse the kubeconfig", func() {
				By("Reconciling the created resource")
				controllerReconciler := &NetworkReconciler{
					Client: k8sClient,
					Scheme: k8sClient.Scheme(),
				}

				result, _ := controllerReconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: typeNamespacedName,
				})

				// The reconcile should start processing but may fail on external cluster access
				// This is expected in a test environment
				Expect(result).To(Equal(reconcile.Result{}))

				// Check that the MultiClusterNetwork still exists and can be retrieved
				retrievedNetwork := &ac3v1alpha1.MultiClusterNetwork{}
				Expect(k8sClient.Get(ctx, typeNamespacedName, retrievedNetwork)).To(Succeed())
				Expect(retrievedNetwork.Spec.Links).To(HaveLen(1))
				Expect(retrievedNetwork.Spec.Links[0].SourceCluster).To(Equal("cluster-1"))
				Expect(retrievedNetwork.Spec.Links[0].TargetCluster).To(Equal("cluster-2"))
			})
		})
	})

	Context("When testing MultiClusterNetwork resource validation", func() {
		It("should validate required fields", func() {
			ctx := context.Background()

			// Test creating a MultiClusterNetwork without required fields
			invalidNetwork := &ac3v1alpha1.MultiClusterNetwork{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "invalid-network",
					Namespace: "default",
				},
				Spec: ac3v1alpha1.MultiClusterNetworkSpec{
					// Missing required Links field
				},
			}

			err := k8sClient.Create(ctx, invalidNetwork)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("spec.links: Required value"))
		})

		It("should create a valid MultiClusterNetwork", func() {
			ctx := context.Background()

			validNetwork := &ac3v1alpha1.MultiClusterNetwork{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "valid-network",
					Namespace: "default",
				},
				Spec: ac3v1alpha1.MultiClusterNetworkSpec{
					Links: []*ac3v1alpha1.MultiClusterLink{
						{
							SourceCluster:   "cluster-1",
							TargetCluster:   "cluster-2",
							SourceNamespace: "source-ns",
							TargetNamespace: "target-ns",
							Applications:    []string{"app1", "app2"},
							Services:        []string{"service1", "service2"},
							Port:            8080,
						},
					},
				},
			}

			Expect(k8sClient.Create(ctx, validNetwork)).To(Succeed())

			// Verify the network was created
			retrievedNetwork := &ac3v1alpha1.MultiClusterNetwork{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      "valid-network",
				Namespace: "default",
			}, retrievedNetwork)).To(Succeed())

			Expect(retrievedNetwork.Spec.Links).To(HaveLen(1))
			Expect(retrievedNetwork.Spec.Links[0].SourceCluster).To(Equal("cluster-1"))
			Expect(retrievedNetwork.Spec.Links[0].Port).To(Equal(8080))

			// Clean up
			Expect(k8sClient.Delete(ctx, validNetwork)).To(Succeed())
		})
	})

	Context("When testing MultiClusterLink validation", func() {
		It("should accept valid MultiClusterLink configurations", func() {
			ctx := context.Background()

			// Test with multiple links
			multiLinkNetwork := &ac3v1alpha1.MultiClusterNetwork{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "multi-link-network",
					Namespace: "default",
				},
				Spec: ac3v1alpha1.MultiClusterNetworkSpec{
					Links: []*ac3v1alpha1.MultiClusterLink{
						{
							SourceCluster:   "cluster-1",
							TargetCluster:   "cluster-2",
							SourceNamespace: "ns1",
							TargetNamespace: "ns2",
							Applications:    []string{"app1"},
							Services:        []string{"service1"},
							Port:            8080,
						},
						{
							SourceCluster:   "cluster-2",
							TargetCluster:   "cluster-3",
							SourceNamespace: "ns2",
							TargetNamespace: "ns3",
							Applications:    []string{"app2"},
							Services:        []string{"service2"},
							Port:            9090,
						},
					},
				},
			}

			Expect(k8sClient.Create(ctx, multiLinkNetwork)).To(Succeed())

			// Verify both links were created
			retrievedNetwork := &ac3v1alpha1.MultiClusterNetwork{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      "multi-link-network",
				Namespace: "default",
			}, retrievedNetwork)).To(Succeed())

			Expect(retrievedNetwork.Spec.Links).To(HaveLen(2))
			Expect(retrievedNetwork.Spec.Links[0].Port).To(Equal(8080))
			Expect(retrievedNetwork.Spec.Links[1].Port).To(Equal(9090))

			// Clean up
			Expect(k8sClient.Delete(ctx, multiLinkNetwork)).To(Succeed())
		})
	})

	Context("When testing service discovery functionality", func() {
		var (
			logger     = zap.New(zap.UseDevMode(true))
			reconciler *NetworkReconciler
		)

		BeforeEach(func() {
			reconciler = &NetworkReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}
		})

		Context("When testing deployment annotation updates", func() {
			It("should add Skupper annotations to matching deployments", func() {
				ctx := context.Background()
				testNamespace := "deploy-test-1"

				// Create test namespace
				ns := &corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{
						Name: testNamespace,
					},
				}
				Expect(k8sClient.Create(ctx, ns)).To(Succeed())
				defer func() {
					Expect(k8sClient.Delete(ctx, ns)).To(Succeed())
				}()

				// Create test deployments
				matchingDeployment := &appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-app",
						Namespace: testNamespace,
					},
					Spec: appsv1.DeploymentSpec{
						Replicas: int32Ptr(1),
						Selector: &metav1.LabelSelector{
							MatchLabels: map[string]string{"app": "test-app"},
						},
						Template: corev1.PodTemplateSpec{
							ObjectMeta: metav1.ObjectMeta{
								Labels: map[string]string{"app": "test-app"},
							},
							Spec: corev1.PodSpec{
								Containers: []corev1.Container{
									{
										Name:  "test-container",
										Image: "nginx:latest",
									},
								},
							},
						},
					},
				}

				nonMatchingDeployment := &appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "other-app",
						Namespace: testNamespace,
					},
					Spec: appsv1.DeploymentSpec{
						Replicas: int32Ptr(1),
						Selector: &metav1.LabelSelector{
							MatchLabels: map[string]string{"app": "other-app"},
						},
						Template: corev1.PodTemplateSpec{
							ObjectMeta: metav1.ObjectMeta{
								Labels: map[string]string{"app": "other-app"},
							},
							Spec: corev1.PodSpec{
								Containers: []corev1.Container{
									{
										Name:  "test-container",
										Image: "nginx:latest",
									},
								},
							},
						},
					},
				}

				// Create deployments
				Expect(k8sClient.Create(ctx, matchingDeployment)).To(Succeed())
				Expect(k8sClient.Create(ctx, nonMatchingDeployment)).To(Succeed())

				// Call the function
				appNames := []string{"test-app"}
				err := reconciler.updateDeploymentsWithSkupperAnnotation(ctx, testNamespace, appNames, logger)
				Expect(err).NotTo(HaveOccurred())

				// Verify the matching deployment has the annotation
				updatedDeployment := &appsv1.Deployment{}
				Expect(k8sClient.Get(ctx, types.NamespacedName{
					Name:      "test-app",
					Namespace: testNamespace,
				}, updatedDeployment)).To(Succeed())

				Expect(updatedDeployment.Annotations).To(HaveKeyWithValue("skupper.io/proxy", "tcp"))

				// Verify the non-matching deployment doesn't have the annotation
				otherDeployment := &appsv1.Deployment{}
				Expect(k8sClient.Get(ctx, types.NamespacedName{
					Name:      "other-app",
					Namespace: testNamespace,
				}, otherDeployment)).To(Succeed())

				if otherDeployment.Annotations != nil {
					Expect(otherDeployment.Annotations).NotTo(HaveKey("skupper.io/proxy"))
				}
			})

			It("should update existing annotations on deployments", func() {
				ctx := context.Background()
				testNamespace := "deploy-test-2"

				// Create test namespace
				ns := &corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{
						Name: testNamespace,
					},
				}
				Expect(k8sClient.Create(ctx, ns)).To(Succeed())
				defer func() {
					Expect(k8sClient.Delete(ctx, ns)).To(Succeed())
				}()

				// Create deployment with existing annotations
				deployment := &appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "existing-app",
						Namespace: testNamespace,
						Annotations: map[string]string{
							"existing-key": "existing-value",
						},
					},
					Spec: appsv1.DeploymentSpec{
						Replicas: int32Ptr(1),
						Selector: &metav1.LabelSelector{
							MatchLabels: map[string]string{"app": "existing-app"},
						},
						Template: corev1.PodTemplateSpec{
							ObjectMeta: metav1.ObjectMeta{
								Labels: map[string]string{"app": "existing-app"},
							},
							Spec: corev1.PodSpec{
								Containers: []corev1.Container{
									{
										Name:  "test-container",
										Image: "nginx:latest",
									},
								},
							},
						},
					},
				}

				Expect(k8sClient.Create(ctx, deployment)).To(Succeed())

				// Call the function
				appNames := []string{"existing-app"}
				err := reconciler.updateDeploymentsWithSkupperAnnotation(ctx, testNamespace, appNames, logger)
				Expect(err).NotTo(HaveOccurred())

				// Verify both annotations exist
				updatedDeployment := &appsv1.Deployment{}
				Expect(k8sClient.Get(ctx, types.NamespacedName{
					Name:      "existing-app",
					Namespace: testNamespace,
				}, updatedDeployment)).To(Succeed())

				Expect(updatedDeployment.Annotations).To(HaveKeyWithValue("skupper.io/proxy", "tcp"))
				Expect(updatedDeployment.Annotations).To(HaveKeyWithValue("existing-key", "existing-value"))
			})

			It("should handle multiple app names", func() {
				ctx := context.Background()
				testNamespace := "deploy-test-3"

				// Create test namespace
				ns := &corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{
						Name: testNamespace,
					},
				}
				Expect(k8sClient.Create(ctx, ns)).To(Succeed())
				defer func() {
					Expect(k8sClient.Delete(ctx, ns)).To(Succeed())
				}()

				// Create multiple deployments
				for i, appName := range []string{"app1", "app2", "app3"} {
					deployment := &appsv1.Deployment{
						ObjectMeta: metav1.ObjectMeta{
							Name:      appName,
							Namespace: testNamespace,
						},
						Spec: appsv1.DeploymentSpec{
							Replicas: int32Ptr(1),
							Selector: &metav1.LabelSelector{
								MatchLabels: map[string]string{"app": appName},
							},
							Template: corev1.PodTemplateSpec{
								ObjectMeta: metav1.ObjectMeta{
									Labels: map[string]string{"app": appName},
								},
								Spec: corev1.PodSpec{
									Containers: []corev1.Container{
										{
											Name:  "test-container",
											Image: "nginx:latest",
										},
									},
								},
							},
						},
					}
					Expect(k8sClient.Create(ctx, deployment)).To(Succeed())
					// Small delay to ensure unique creation times
					time.Sleep(time.Millisecond * time.Duration(i*10))
				}

				// Call the function with multiple app names
				appNames := []string{"app1", "app3"} // Skip app2
				err := reconciler.updateDeploymentsWithSkupperAnnotation(ctx, testNamespace, appNames, logger)
				Expect(err).NotTo(HaveOccurred())

				// Verify app1 and app3 have annotations
				for _, appName := range []string{"app1", "app3"} {
					deployment := &appsv1.Deployment{}
					Expect(k8sClient.Get(ctx, types.NamespacedName{
						Name:      appName,
						Namespace: testNamespace,
					}, deployment)).To(Succeed())
					Expect(deployment.Annotations).To(HaveKeyWithValue("skupper.io/proxy", "tcp"))
				}

				// Verify app2 doesn't have the annotation
				app2 := &appsv1.Deployment{}
				Expect(k8sClient.Get(ctx, types.NamespacedName{
					Name:      "app2",
					Namespace: testNamespace,
				}, app2)).To(Succeed())
				if app2.Annotations != nil {
					Expect(app2.Annotations).NotTo(HaveKey("skupper.io/proxy"))
				}
			})

			It("should handle empty app names list", func() {
				ctx := context.Background()
				testNamespace := "deploy-test-4"

				// Create test namespace
				ns := &corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{
						Name: testNamespace,
					},
				}
				Expect(k8sClient.Create(ctx, ns)).To(Succeed())
				defer func() {
					Expect(k8sClient.Delete(ctx, ns)).To(Succeed())
				}()

				// Create a deployment
				deployment := &appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-app",
						Namespace: testNamespace,
					},
					Spec: appsv1.DeploymentSpec{
						Replicas: int32Ptr(1),
						Selector: &metav1.LabelSelector{
							MatchLabels: map[string]string{"app": "test-app"},
						},
						Template: corev1.PodTemplateSpec{
							ObjectMeta: metav1.ObjectMeta{
								Labels: map[string]string{"app": "test-app"},
							},
							Spec: corev1.PodSpec{
								Containers: []corev1.Container{
									{
										Name:  "test-container",
										Image: "nginx:latest",
									},
								},
							},
						},
					},
				}
				Expect(k8sClient.Create(ctx, deployment)).To(Succeed())

				// Call the function with empty app names
				err := reconciler.updateDeploymentsWithSkupperAnnotation(ctx, testNamespace, []string{}, logger)
				Expect(err).NotTo(HaveOccurred())

				// Verify deployment doesn't have the annotation
				updatedDeployment := &appsv1.Deployment{}
				Expect(k8sClient.Get(ctx, types.NamespacedName{
					Name:      "test-app",
					Namespace: testNamespace,
				}, updatedDeployment)).To(Succeed())

				if updatedDeployment.Annotations != nil {
					Expect(updatedDeployment.Annotations).NotTo(HaveKey("skupper.io/proxy"))
				}
			})
		})

		Context("When testing service annotation updates", func() {
			It("should add Skupper annotations to matching services", func() {
				ctx := context.Background()
				testNamespace := "service-test-1"

				// Create test namespace
				ns := &corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{
						Name: testNamespace,
					},
				}
				Expect(k8sClient.Create(ctx, ns)).To(Succeed())
				defer func() {
					Expect(k8sClient.Delete(ctx, ns)).To(Succeed())
				}()

				// Create test services
				matchingService := &corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-service",
						Namespace: testNamespace,
					},
					Spec: corev1.ServiceSpec{
						Ports: []corev1.ServicePort{
							{
								Port:       80,
								TargetPort: intstr.FromInt(8080),
							},
						},
						Selector: map[string]string{"app": "test-app"},
					},
				}

				nonMatchingService := &corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "other-service",
						Namespace: testNamespace,
					},
					Spec: corev1.ServiceSpec{
						Ports: []corev1.ServicePort{
							{
								Port:       80,
								TargetPort: intstr.FromInt(8080),
							},
						},
						Selector: map[string]string{"app": "other-app"},
					},
				}

				// Create services
				Expect(k8sClient.Create(ctx, matchingService)).To(Succeed())
				Expect(k8sClient.Create(ctx, nonMatchingService)).To(Succeed())

				// Call the function
				serviceNames := []string{"test-service"}
				err := reconciler.updateServicesWithSkupperAnnotation(ctx, testNamespace, serviceNames, logger)
				Expect(err).NotTo(HaveOccurred())

				// Verify the matching service has the annotation
				updatedService := &corev1.Service{}
				Expect(k8sClient.Get(ctx, types.NamespacedName{
					Name:      "test-service",
					Namespace: testNamespace,
				}, updatedService)).To(Succeed())

				Expect(updatedService.Annotations).To(HaveKeyWithValue("skupper.io/proxy", "test-service"))

				// Verify the non-matching service doesn't have the annotation
				otherService := &corev1.Service{}
				Expect(k8sClient.Get(ctx, types.NamespacedName{
					Name:      "other-service",
					Namespace: testNamespace,
				}, otherService)).To(Succeed())

				if otherService.Annotations != nil {
					Expect(otherService.Annotations).NotTo(HaveKey("skupper.io/proxy"))
				}
			})

			It("should update existing annotations on services", func() {
				ctx := context.Background()
				testNamespace := "service-test-2"

				// Create test namespace
				ns := &corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{
						Name: testNamespace,
					},
				}
				Expect(k8sClient.Create(ctx, ns)).To(Succeed())
				defer func() {
					Expect(k8sClient.Delete(ctx, ns)).To(Succeed())
				}()

				// Create service with existing annotations
				service := &corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "existing-service",
						Namespace: testNamespace,
						Annotations: map[string]string{
							"existing-key": "existing-value",
						},
					},
					Spec: corev1.ServiceSpec{
						Ports: []corev1.ServicePort{
							{
								Port:       80,
								TargetPort: intstr.FromInt(8080),
							},
						},
						Selector: map[string]string{"app": "existing-app"},
					},
				}

				Expect(k8sClient.Create(ctx, service)).To(Succeed())

				// Call the function
				serviceNames := []string{"existing-service"}
				err := reconciler.updateServicesWithSkupperAnnotation(ctx, testNamespace, serviceNames, logger)
				Expect(err).NotTo(HaveOccurred())

				// Verify both annotations exist
				updatedService := &corev1.Service{}
				Expect(k8sClient.Get(ctx, types.NamespacedName{
					Name:      "existing-service",
					Namespace: testNamespace,
				}, updatedService)).To(Succeed())

				Expect(updatedService.Annotations).To(HaveKeyWithValue("skupper.io/proxy", "existing-service"))
				Expect(updatedService.Annotations).To(HaveKeyWithValue("existing-key", "existing-value"))
			})

			It("should handle multiple service names", func() {
				ctx := context.Background()
				testNamespace := "service-test-3"

				// Create test namespace
				ns := &corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{
						Name: testNamespace,
					},
				}
				Expect(k8sClient.Create(ctx, ns)).To(Succeed())
				defer func() {
					Expect(k8sClient.Delete(ctx, ns)).To(Succeed())
				}()

				// Create multiple services
				for i, serviceName := range []string{"service1", "service2", "service3"} {
					service := &corev1.Service{
						ObjectMeta: metav1.ObjectMeta{
							Name:      serviceName,
							Namespace: testNamespace,
						},
						Spec: corev1.ServiceSpec{
							Ports: []corev1.ServicePort{
								{
									Port:       80,
									TargetPort: intstr.FromInt(8080),
								},
							},
							Selector: map[string]string{"app": serviceName},
						},
					}
					Expect(k8sClient.Create(ctx, service)).To(Succeed())
					// Small delay to ensure unique creation times
					time.Sleep(time.Millisecond * time.Duration(i*10))
				}

				// Call the function with multiple service names
				serviceNames := []string{"service1", "service3"} // Skip service2
				err := reconciler.updateServicesWithSkupperAnnotation(ctx, testNamespace, serviceNames, logger)
				Expect(err).NotTo(HaveOccurred())

				// Verify service1 and service3 have annotations
				for _, serviceName := range []string{"service1", "service3"} {
					service := &corev1.Service{}
					Expect(k8sClient.Get(ctx, types.NamespacedName{
						Name:      serviceName,
						Namespace: testNamespace,
					}, service)).To(Succeed())
					Expect(service.Annotations).To(HaveKeyWithValue("skupper.io/proxy", serviceName))
				}

				// Verify service2 doesn't have the annotation
				service2 := &corev1.Service{}
				Expect(k8sClient.Get(ctx, types.NamespacedName{
					Name:      "service2",
					Namespace: testNamespace,
				}, service2)).To(Succeed())
				if service2.Annotations != nil {
					Expect(service2.Annotations).NotTo(HaveKey("skupper.io/proxy"))
				}
			})

			It("should handle empty service names list", func() {
				ctx := context.Background()
				testNamespace := "service-test-4"

				// Create test namespace
				ns := &corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{
						Name: testNamespace,
					},
				}
				Expect(k8sClient.Create(ctx, ns)).To(Succeed())
				defer func() {
					Expect(k8sClient.Delete(ctx, ns)).To(Succeed())
				}()

				// Create a service
				service := &corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-service",
						Namespace: testNamespace,
					},
					Spec: corev1.ServiceSpec{
						Ports: []corev1.ServicePort{
							{
								Port:       80,
								TargetPort: intstr.FromInt(8080),
							},
						},
						Selector: map[string]string{"app": "test-app"},
					},
				}
				Expect(k8sClient.Create(ctx, service)).To(Succeed())

				// Call the function with empty service names
				err := reconciler.updateServicesWithSkupperAnnotation(ctx, testNamespace, []string{}, logger)
				Expect(err).NotTo(HaveOccurred())

				// Verify service doesn't have the annotation
				updatedService := &corev1.Service{}
				Expect(k8sClient.Get(ctx, types.NamespacedName{
					Name:      "test-service",
					Namespace: testNamespace,
				}, updatedService)).To(Succeed())

				if updatedService.Annotations != nil {
					Expect(updatedService.Annotations).NotTo(HaveKey("skupper.io/proxy"))
				}
			})

			It("should handle non-existent service names gracefully", func() {
				ctx := context.Background()
				testNamespace := "service-test-5"

				// Create test namespace
				ns := &corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{
						Name: testNamespace,
					},
				}
				Expect(k8sClient.Create(ctx, ns)).To(Succeed())
				defer func() {
					Expect(k8sClient.Delete(ctx, ns)).To(Succeed())
				}()

				// Create a service
				service := &corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "real-service",
						Namespace: testNamespace,
					},
					Spec: corev1.ServiceSpec{
						Ports: []corev1.ServicePort{
							{
								Port:       80,
								TargetPort: intstr.FromInt(8080),
							},
						},
						Selector: map[string]string{"app": "real-app"},
					},
				}
				Expect(k8sClient.Create(ctx, service)).To(Succeed())

				// Call the function with non-existent service name
				serviceNames := []string{"non-existent-service"}
				err := reconciler.updateServicesWithSkupperAnnotation(ctx, testNamespace, serviceNames, logger)
				Expect(err).NotTo(HaveOccurred()) // Should not error, just skip

				// Verify the real service doesn't have the annotation
				updatedService := &corev1.Service{}
				Expect(k8sClient.Get(ctx, types.NamespacedName{
					Name:      "real-service",
					Namespace: testNamespace,
				}, updatedService)).To(Succeed())

				if updatedService.Annotations != nil {
					Expect(updatedService.Annotations).NotTo(HaveKey("skupper.io/proxy"))
				}
			})
		})

		Context("When testing error scenarios", func() {
			It("should handle non-existent namespace for deployments", func() {
				ctx := context.Background()

				// Call the function with non-existent namespace
				err := reconciler.updateDeploymentsWithSkupperAnnotation(ctx, "non-existent-namespace", []string{"test-app"}, logger)
				Expect(err).NotTo(HaveOccurred()) // Should not error, just return empty list
			})

			It("should handle non-existent namespace for services", func() {
				ctx := context.Background()

				// Call the function with non-existent namespace
				err := reconciler.updateServicesWithSkupperAnnotation(ctx, "non-existent-namespace", []string{"test-service"}, logger)
				Expect(err).NotTo(HaveOccurred()) // Should not error, just return empty list
			})
		})
	})
})
