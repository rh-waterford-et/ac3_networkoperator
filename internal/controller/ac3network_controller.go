package controller

import (
	"context"
	"fmt"
	"os/exec"
	"time"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	// "k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	// "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"github.com/go-logr/logr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log"

	ac3v1alpha1 "github.com/raycarroll/ac3no/api/v1alpha1"
)

// AC3NetworkReconciler reconciles an AC3Network object
type AC3NetworkReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// SkupperRouter and related types
type SkupperRouter struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              SkupperRouterSpec `json:"spec,omitempty"`
}

type SkupperRouterSpec struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
}

func (in *SkupperRouter) DeepCopyObject() runtime.Object {
	out := SkupperRouter{
		TypeMeta:   in.TypeMeta,
		ObjectMeta: *in.ObjectMeta.DeepCopy(),
		Spec:       in.Spec,
	}
	return &out
}

func (in *SkupperRouter) GetObjectKind() schema.ObjectKind {
	return &in.TypeMeta
}

type SkupperRouterList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []SkupperRouter `json:"items"`
}

func (in *SkupperRouterList) DeepCopyObject() runtime.Object {
	out := SkupperRouterList{
		Items: make([]SkupperRouter, len(in.Items)),
	}
	copy(out.Items, in.Items)
	return &out
}

func (in *SkupperRouterList) GetObjectKind() schema.ObjectKind {
	return &in.TypeMeta
}

// +kubebuilder:rbac:groups=ac3.redhat.com,resources=ac3networks,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=ac3.redhat.com,resources=ac3networks/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=ac3.redhat.com,resources=ac3networks/finalizers,verbs=update
// +kubebuilder:rbac:groups=ac3.redhat.com,resources=skupperrouters,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=ac3.redhat.com,resources=skupperrouters/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=ac3.redhat.com,resources=skupperrouters/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;update;patch;delete

// Reconcile reads that state of the cluster for a AC3Network object and makes changes based on the state read
func (r *AC3NetworkReconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("Starting Reconcile loop", "request", req)
	ctx = logr.NewContext(ctx, logger)
	link := &ac3v1alpha1.AC3Network{}

    logger.Info("Fetching Link resource", "namespace", req.Namespace, "name", req.Name)

	if err := r.Client.Get(ctx, req.NamespacedName, link); err != nil {
		if errors.IsNotFound(err) {
            logger.Info(req.NamespacedName.String(), "Link here")
			logger.Error(err, "Link resource not found, possibly deleted")
			return reconcile.Result{}, nil
		}
		logger.Error(err, "Failed to get Link")
		return reconcile.Result{}, err
	}

	// logger.Info("Fetching Link resource", "namespace", link.Namespace, "name", link.Name)

    logger.Info("CR Detail", "SourceCluster", link.Spec.Link.SourceCluster)
    logger.Info("CR Detail", "SourceCluster", link.Spec)

	// 1. Fetch the ConfigMap with the combined kubeconfig
	configMap := &corev1.ConfigMap{}
	err := r.Get(ctx, client.ObjectKey{Name: "ac3-combined-kubeconfig", Namespace: "sk1"}, configMap)
	if err != nil {
		logger.Error(err, "Failed to get ConfigMap", "name", "ac3-combined-kubeconfig", "namespace", "sk1")
		return reconcile.Result{}, err
	}

	// 2. Extract kubeconfig content
	kubeconfigContent, ok := configMap.Data["kubeconfig"]
	if !ok {
		err := fmt.Errorf("kubeconfig not found")
		logger.Error(err, "ConfigMap does not contain kubeconfig", "name", "ac3-combined-kubeconfig")
		return reconcile.Result{}, err
	}

	// 3. Parse kubeconfig
	kubeconfig, err := clientcmd.Load([]byte(kubeconfigContent))
	if err != nil {
		logger.Error(err, "Failed to parse kubeconfig")
		return reconcile.Result{}, err
	}

	// 4. Iterate through contexts and interact with clusters
	for contextName, _ := range kubeconfig.Contexts {
		logger.Info("Switching context", "context", contextName)

		// Create a REST config for the cluster
		config, err := clientcmd.NewNonInteractiveClientConfig(*kubeconfig, contextName, nil, nil).ClientConfig()
		if err != nil {
			logger.Error(err, "Failed to create Kubernetes client config", "context", contextName)
			continue
		}

		clientset, err := kubernetes.NewForConfig(config)
		if err != nil {
			logger.Error(err, "Failed to create Kubernetes clientset", "context", contextName)
			continue
		}

		// Example: List all pods in the default namespace of the current context
		pods, err := clientset.CoreV1().Pods("default").List(ctx, metav1.ListOptions{})
		if err != nil {
			logger.Error(err, "Failed to list pods in default namespace", "context", contextName)
			continue
		}

		// Log pod names
		for _, pod := range pods.Items {
			logger.Info("Pod found", "podName", pod.Name, "context", contextName)
		}
	}

	// 5. Manage ConfigMaps in different namespaces (sk1 and sk2)
	configMapNamespaces := []string{"sk1", "sk2"}
	configMapName := "skupper-site"
	data := map[string]string{
		"example.key":      "example.value",
		"console":          "true",
		"flow-collector":   "true",
		"console-user":     "username",
		"console-password": "password",
	}

	for _, namespace := range configMapNamespaces {
		configMap := &corev1.ConfigMap{}
		err := r.Get(ctx, client.ObjectKey{Name: configMapName, Namespace: namespace}, configMap)
		if err != nil {
			if client.IgnoreNotFound(err) != nil {
				logger.Error(err, "Failed to get ConfigMap", "name", configMapName, "namespace", namespace)
				return reconcile.Result{}, err
			}

			// Create the ConfigMap if it doesn't exist
			configMap = r.createConfigMap(ctx, configMapName, namespace, data)
			if err := r.Create(ctx, configMap); err != nil {
				logger.Error(err, "Failed to create ConfigMap", "name", configMapName, "namespace", namespace)
				return reconcile.Result{}, err
			}
			logger.Info("Created ConfigMap", "name", configMapName, "namespace", namespace)
		} else {
			// Update the ConfigMap if necessary
			if r.needsUpdateConfigMap(configMap, data) {
				configMap.Data = data
				if err := r.Update(ctx, configMap); err != nil {
					logger.Error(err, "Failed to update ConfigMap", "name", configMapName, "namespace", namespace)
					return reconcile.Result{}, err
				}
				logger.Info("Updated ConfigMap", "name", configMapName, "namespace", namespace)
			}
		}
	}

	// 6. Manage Secrets between sk1 and sk2 namespaces
	secretNamespace := link.Spec.Link.SecretNamespace
	secretName := link.Spec.Link.SecretName
	secret := &corev1.Secret{}

	// Retrieve the Secret from the cluster
	err = r.Get(ctx, client.ObjectKey{Name: secretName, Namespace: secretNamespace}, secret)
	if err != nil {
		if errors.IsNotFound(err) {
			// Create the Secret if it doesn't exist
			secret = r.createSecret(ctx, secretName, secretNamespace)
			if err := r.Create(ctx, secret); err != nil {
				logger.Error(err, "Failed to create Secret", "name", secretName, "namespace", secretNamespace)
				return reconcile.Result{}, err
			}
			logger.Info("Created Secret", "name", secretName, "namespace", secretNamespace)
		} else {
			logger.Error(err, "Failed to get Secret", "name", secretName, "namespace", secretNamespace)
			return reconcile.Result{}, err
		}
	}

	// Log the success of copying the sk1-token to the sk2 namespace
	logger.Info("Secret sk1-token copied to namespace sk2 successfully")

	// Copy the Secret to the sk2 namespace
	err = r.copySecretToNamespace(ctx, secret, "sk2")
	if err != nil {
		logger.Error(err, "Failed to copy Secret to sk2 namespace", "name", secretName)
		return reconcile.Result{}, err
	}

	logger.Info("Secret sk1-token copied to namespace sk2 successfully")

	// 7. Log Skupper link status
	// err = r.logSkupperLinkStatus(ctx, "sk2")
	// if err != nil {
	//     logger.Error(err, "Failed to get Skupper link status", "namespace", "sk2")
	//     return reconcile.Result{}, err
	// }

	// 8. Fetch the AC3Network instance and reconcile SkupperRouter instances
	// ac3Network := &ac3v1alpha1.AC3Network{}
	// if err := r.Get(ctx, req.NamespacedName, ac3Network); err != nil {
	//     logger.Error(err, "Failed to fetch AC3Network")
	//     return reconcile.Result{}, client.IgnoreNotFound(err)
	// }

	// // List all instances of SkupperRouter
	// routerList := SkupperRouterList{}
	// if err := r.List(ctx, &routerList, client.InNamespace(req.Namespace)); err != nil {
	//     logger.Error(err, "Failed to list SkupperRouter instances")
	//     return reconcile.Result{}, err
	// }

	// // Reconcile each SkupperRouter instance
	// for _, routerInstance := range routerList.Items {
	//     err := r.reconcileSkupperRouter(ctx, routerInstance)
	//     if err != nil {
	//         logger.Error(err, "Failed to reconcile SkupperRouter", "name", routerInstance.Name, "namespace", routerInstance.Namespace)
	//         return reconcile.Result{}, err
	//     }
	// }

	logger.Info("IM HERE")

	// Target clusters and namespaces
	sourceCluster := link.Spec.Link.SourceCluster
	targetCluster := link.Spec.Link.TargetCluster
	sourceNamespace := link.Spec.Link.SourceNamespace
	targetNamespaces := link.Spec.Link.TargetNamespace
	secretName2 := link.Spec.Link.SecretName2
	
	for _, targetNamespace := range targetNamespaces {

	

	// Step 1: Switch to ac3-cluster-2 and get the secret from the sk1 namespace
	for contextName, _ := range kubeconfig.Contexts {
		if contextName == sourceCluster {
			logger.Info("Switching context to ac3-cluster-2", "context", contextName)

			// Get Kubernetes client for ac3-cluster-2
			sourceConfig, err := clientcmd.NewNonInteractiveClientConfig(*kubeconfig, contextName, nil, nil).ClientConfig()
			if err != nil {
				logger.Error(err, "Failed to create Kubernetes client config for ac3-cluster-2", "context", contextName)
				return reconcile.Result{}, err
			}

			sourceClientset, err := kubernetes.NewForConfig(sourceConfig)
			if err != nil {
				logger.Error(err, "Failed to create Kubernetes clientset for ac3-cluster-2", "context", contextName)
				return reconcile.Result{}, err
			}

			// Get the secret from sk1 namespace
			secret, err := sourceClientset.CoreV1().Secrets(sourceNamespace).Get(ctx, secretName2, metav1.GetOptions{})
			if err != nil {
				logger.Error(err, "Failed to get secret from sk1 namespace in ac3-cluster-2", "secretName2", secretName2)
				return reconcile.Result{}, err
			}

			logger.Info("Successfully retrieved sk1-token from ac3-cluster-2", "secretName2", secret)

			// Step 2: Switch to ac3-cluster-1 and copy the secret to the default namespace
			for targetContextName, _ := range kubeconfig.Contexts {
				if targetContextName == targetCluster {
					logger.Info("Switching context to ac3-cluster-1", "context", targetContextName)

					// Get Kubernetes client for ac3-cluster-1
					targetConfig, err := clientcmd.NewNonInteractiveClientConfig(*kubeconfig, targetContextName, nil, nil).ClientConfig()
					if err != nil {
						logger.Error(err, "Failed to create Kubernetes client config for ac3-cluster-1", "context", targetContextName)
						return reconcile.Result{}, err
					}

					targetClientset, err := kubernetes.NewForConfig(targetConfig)
					if err != nil {
						logger.Error(err, "Failed to create Kubernetes clientset for ac3-cluster-1", "context", targetContextName)
						return reconcile.Result{}, err
					}

					// Create a deep copy of the secret and set the new namespace
					secretCopy := secret.DeepCopy()
					secretCopy.Namespace = targetNamespace 
					secretCopy.ResourceVersion = "" // Clear the resource version to allow creation in the new namespace

					// Ensure the data field is copied from the source secret
					//secretCopy.Data = secret.Data

					// Log the secret data to check that it's being copied correctly
					logger.Info("Preparing to copy sk1-token to default namespace on ac3-cluster-1", "secretCopy", secretCopy)

					// Create the secret in the namespace on ac3-cluster-1
					_, err = targetClientset.CoreV1().Secrets(targetNamespace).Create(ctx, secretCopy, metav1.CreateOptions{})
					if err != nil {
						if strings.Contains(err.Error(), "already exists"){
							logger.Info("Secret already exists in default namespace on ac3-cluster-1", "secretName2", secret.Name)
							continue
						}
						logger.Error(err, "Failed to create secret in default namespace on ac3-cluster-1", "context", targetContextName)
						return reconcile.Result{}, err
					}

					// Log success and ensure secret contents are correct
					logger.Info("Successfully copied sk1-token to default namespace on ac3-cluster-1",
						"secretName2", secret.Name,
						"targetNamespace", targetNamespace ,
						"data", secretCopy.Data)

					// Step to create a ConfigMap in the default namespace on ac3-cluster-1
					configMapData := map[string]string{
						"example.key": "example.value",
					}

					configMap := &corev1.ConfigMap{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "skupper-site",
							Namespace: targetNamespace ,
						},
						Data: configMapData,
					}

					// Create the ConfigMap in the namespace on ac3-cluster-1
					_, err = targetClientset.CoreV1().ConfigMaps(targetNamespace).Create(ctx, configMap, metav1.CreateOptions{})
					if err != nil {
						logger.Error(err, "Failed to create ConfigMap in default namespace on ac3-cluster-1", "context", targetContextName)
						return reconcile.Result{}, err
					}

					logger.Info("Successfully created ConfigMap skupper-site in namespace on ac3-cluster-1", "namespace", targetNamespace )

					break
				}
			}
			break
		}
	}
}

	logger.Info("Reconcile loop completed successfully")
	return reconcile.Result{}, nil

}

// Helper function to create a Secret
func (r *AC3NetworkReconciler) createSecret(ctx context.Context, name string, namespace string) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels: map[string]string{
				"skupper.io/type": "connection-token-request",
			},
		},
		Data: map[string][]byte{
			"connectionToken": []byte("some-token-data"),
		},
	}
}

// copySecretToNamespace copies a secret to another namespace
func (r *AC3NetworkReconciler) copySecretToNamespace(ctx context.Context, secret *corev1.Secret, targetNamespace string) error {
	// Retrieve the original secret
	//sleep for 15 seconds
	logger := log.FromContext(ctx)
	time.Sleep(15 * time.Second)
	err := r.Get(ctx, types.NamespacedName{Name: secret.Name, Namespace: secret.Namespace}, secret)
	if err != nil {
		return err
	}

	// Create a deep copy of the secret
	secretCopy := secret.DeepCopy()
	// Set the target namespace
	secretCopy.ObjectMeta.Namespace = targetNamespace
	// Clear the ResourceVersion for the new object
	secretCopy.ObjectMeta.ResourceVersion = ""
	// Attempt to create the secret in the target namespace
	//print out secret copy to log
	logger.Info("Secret copy", "secret", secretCopy)
	err = r.Create(ctx, secretCopy)
	if err != nil && errors.IsAlreadyExists(err) {
		// If the secret already exists, update it
		existingSecret := &corev1.Secret{}
		err = r.Get(ctx, types.NamespacedName{Name: secret.Name, Namespace: targetNamespace}, existingSecret)
		if err != nil {
			return err
		}

		// Update the existing secret's data
		existingSecret.Data = secretCopy.Data

		// Retrieve the existing secret from the target namespace
		// Update the existing secret in the target namespace
		err = r.Update(ctx, existingSecret)
		if err != nil {
			return err
		}
		// Update the existing secret's data with the data from the copied secret
	}
	return err
}

// linkSkupperSites links the Skupper sites using the token
func (r *AC3NetworkReconciler) linkSkupperSites(ctx context.Context, targetNamespace string, secret *corev1.Secret) error {
	// Create the Skupper link by applying the secret in the target namespace
	secretToApply := secret.DeepCopy()
	secretToApply.Namespace = targetNamespace

	// Clear the ResourceVersion to ensure it's a new object creation
	secretToApply.ResourceVersion = ""

	err := r.Create(ctx, secretToApply)
	if err != nil && errors.IsAlreadyExists(err) {
		// If already exists, update it
		existingSecret := &corev1.Secret{}
		err = r.Get(ctx, types.NamespacedName{Name: secret.Name, Namespace: targetNamespace}, existingSecret)
		if err != nil {
			return err
		}
		existingSecret.Data = secretToApply.Data
		err = r.Update(ctx, existingSecret)
		if err != nil {
			return err
		}
	} else if err != nil {
		return err
	}

	return nil
}

// logSkupperLinkStatus logs the status of the Skupper link for a given namespace
func (r *AC3NetworkReconciler) logSkupperLinkStatus(ctx context.Context, namespace string) error {
	cmd := exec.Command("skupper", "link", "status", "-n", namespace)
	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("failed to get Skupper link status: %v", err)
	}

	log.FromContext(ctx).Info("Skupper link status", "namespace", namespace, "status", string(output))
	return nil
}

// reconcileSkupperRouter reconciles a SkupperRouter instance
func (r *AC3NetworkReconciler) reconcileSkupperRouter(ctx context.Context, routerInstance SkupperRouter) error {
	logger := log.FromContext(ctx)
	logger.Info("Reconciling SkupperRouter", "name", routerInstance.Name, "namespace", routerInstance.Namespace)

	// Example: Ensure a deployment is created for each SkupperRouter
	deployment := &appsv1.Deployment{}
	err := r.Get(ctx, types.NamespacedName{Name: routerInstance.Name, Namespace: routerInstance.Namespace}, deployment)
	if err != nil {
		if errors.IsNotFound(err) {
			// Deployment not found, create it
			deployment = &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      routerInstance.Name,
					Namespace: routerInstance.Namespace,
				},
				Spec: appsv1.DeploymentSpec{
					Replicas: int32Ptr(1),
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"app": routerInstance.Name,
						},
					},
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{
								"app": routerInstance.Name,
							},
						},
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name:  "skupper-router",
									Image: "quay.io/ryjenkin/ac3no3:166",
									Ports: []corev1.ContainerPort{
										{
											Name:          "amqp",
											ContainerPort: 5672,
											Protocol:      corev1.ProtocolTCP,
										},
									},
									Resources: corev1.ResourceRequirements{
										Requests: corev1.ResourceList{
											corev1.ResourceCPU:    resource.MustParse("100m"),
											corev1.ResourceMemory: resource.MustParse("128Mi"),
										},
									},
								},
							},
						},
					},
				},
			}
			if err := r.Create(ctx, deployment); err != nil {
				logger.Error(err, "Failed to create Deployment for SkupperRouter", "name", routerInstance.Name, "namespace", routerInstance.Namespace)
				return err
			}
			logger.Info("Created Deployment for SkupperRouter", "name", routerInstance.Name, "namespace", routerInstance.Namespace)
		} else {
			logger.Error(err, "Failed to get Deployment for SkupperRouter", "name", routerInstance.Name, "namespace", routerInstance.Namespace)
			return err
		}
	} else {
		// Deployment exists, update it if necessary
		// Placeholder: Add logic here if needed to update the deployment
		logger.Info("Deployment already exists for SkupperRouter", "name", routerInstance.Name, "namespace", routerInstance.Namespace)
	}

	return nil
}

// Helper function to create a ConfigMap
func (r *AC3NetworkReconciler) createConfigMap(ctx context.Context, name string, namespace string, data map[string]string) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Data: data,
	}
}

// Helper function to determine if a ConfigMap needs updating
func (r *AC3NetworkReconciler) needsUpdateConfigMap(configMap *corev1.ConfigMap, data map[string]string) bool {
	if configMap.Data == nil {
		configMap.Data = map[string]string{}
	}
	for k, v := range data {
		if configMap.Data[k] != v {
			return true
		}
	}
	return false
}

// int32Ptr returns a pointer to an int32
func int32Ptr(i int32) *int32 {
	return &i
}

// SetupWithManager sets up the controller with the Manager.
func (r *AC3NetworkReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&ac3v1alpha1.AC3Network{}).
		Owns(&ac3v1alpha1.Link{}).
		Complete(r)
}
