package controller

import (
	"context"
	"fmt"
	"k8s.io/utils/pointer"
	"os/exec"
	"strings"
	"time"

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

	ac3v1alpha1 "github.com/rh-waterford-et/ac3_networkoperator/api/v1alpha1"
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

type ApplicationSpec struct {
	Name string `json:"name"`
	// Port int    `json:"port"`
}
type ServiceSpec struct {
	Name string `json:"name"`
	// Port int    `json:"port"`
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
// +kubebuilder:rbac:groups="",resources=secrets;configmaps,verbs=get;list;watch;create;update;patch;delete

// Reconcile reads that state of the cluster for a AC3Network object and makes changes based on the state read
func (r *AC3NetworkReconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("Starting Reconcile loop", "request", req)
	ctx = logr.NewContext(ctx, logger)
	link := &ac3v1alpha1.AC3Network{}

	logger.Info("Fetching Link resource", "namespace", req.Namespace, "name", req.Name)

	if err := r.Client.Get(ctx, req.NamespacedName, link); err != nil {
		if errors.IsNotFound(err) {
			logger.Error(err, "Link resource not found, possibly deleted")
			return reconcile.Result{}, nil
		}
		logger.Error(err, "Failed to get Link")
		return reconcile.Result{}, err
	}

	logger.Info("CR Detail", "SourceCluster", link.Spec)

	// 1. Fetch the ConfigMap with the combined kubeconfig
	configMap := &corev1.ConfigMap{}
	err := r.Get(ctx, client.ObjectKey{Name: "ac3-combined-kubeconfig", Namespace: "ac3no-system"}, configMap)
	if err != nil {
		logger.Error(err, "Failed to get ConfigMap", "name", "ac3-combined-kubeconfig", "namespace", "ac3no-system")
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

	// 5. Manage ConfigMaps in different namespaces (sk1 and sk2)
	//put in a for loop and putit in a get to retrieve the ac3network
	ac3Network := &ac3v1alpha1.AC3Network{}
	if err := r.Get(ctx, req.NamespacedName, ac3Network); err != nil {
		logger.Error(err, "Failed to fetch AC3Network")
		return reconcile.Result{}, client.IgnoreNotFound(err)
	}
	//make a for loop for each link
	//retrieving CRD and going through each link
	for _, link := range ac3Network.Spec.Links {
		configMapNamespaces := []string{link.SourceNamespace, link.TargetNamespace}
		configMapName := "skupper-site"
		data := map[string]string{
			"example.key":                 "example.value",
			"console":                     "true",
			"flow-collector":              "true",
			"console-user":                "username",
			"console-password":            "password",
			"router-cpu":                  "2",     // Example: 2 cores
			"router-memory":               "256Mi", // Example: 256 MiB
			"router-cpu-limit":            "5",     // Example: 1 core
			"router-memory-limit":         "512Mi", // Example: 512 MiB
			"controller-cpu":              "250m",  // Example: 250 millicores
			"controller-memory":           "128Mi", // Example: 128 MiB
			"controller-cpu-limit":        "500m",  // Example: 500 millicores
			"controller-memory-limit":     "256Mi", // Example: 256 MiB
			"flow-collector-cpu":          "250m",  // Example: 250 millicores
			"flow-collector-memory":       "256Mi", // Example: 256 MiB
			"flow-collector-cpu-limit":    "500m",  // Example: 500 millicores
			"flow-collector-memory-limit": "512Mi", // Example: 512 MiB
			"prometheus-cpu":              "500m",  // Example: 500 millicores
			"prometheus-memory":           "512Mi", // Example: 512 MiB
			"prometheus-cpu-limit":        "1",     // Example: 1 core
			"prometheus-memory-limit":     "1Gi",   // Example: 1 GiB
			"enable-skupper-events":       "true",
		}

		////get secret from source
		//// deep copy
		////create on target
		//
		err = r.createUpdateSecret(ctx, link.SourceNamespace, link.TargetNamespace, pointer.Int(5))
		if err != nil {
			logger.Error(err, "Failed to copy secret to namespace")
			return reconcile.Result{}, err
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

		// Log the success of copying the sk1-token to the sk2 namespace
		logger.Info("Secret sk1-token copied to namespace sk2 successfully")

		//var cost int = 5
		//// Copy the Secret to the sk2 namespace
		//err = r.copySecretToNamespace(ctx, secret, "sk2", &cost)
		//if err != nil {
		//	logger.Error(err, "Failed to copy Secret to sk2 namespace", "name", secretName)
		//	return reconcile.Result{}, err
		//}

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

		// Target clusters and namespaces
		sourceCluster := link.SourceCluster
		targetCluster := link.TargetCluster
		sourceNamespace := link.SourceNamespace
		targetNamespace := link.TargetNamespace
		appNames := link.Applications
		serviceNames := link.Services

		// Call the function to update deployments with Skupper annotation
		err = r.updateDeploymentsWithSkupperAnnotation(ctx, sourceNamespace, appNames, logger)
		if err != nil {
			logger.Error(err, "Failed to update deployments with Skupper annotation")
			return reconcile.Result{}, err
		}

		// Call the function to update services with Skupper annotation
		err = r.updateServicesWithSkupperAnnotation(ctx, sourceNamespace, serviceNames, logger)
		if err != nil {
			logger.Error(err, "Failed to update services with Skupper annotation")
			return reconcile.Result{}, err
		}

		// Step 1: Switch to ac3-cluster-2 and get the secret from the source namespace
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

				// Get the secret from source namespace
				secret, err := sourceClientset.CoreV1().Secrets(sourceNamespace).Get(ctx, "Token", metav1.GetOptions{})
				if err != nil {
					logger.Error(err, "Failed to get secret from source namespace in ac3-cluster-2", "namespace", sourceNamespace)
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
						//cost += 5
						//secretCopy.Annotations["skupper.io/cost"] = strconv.Itoa(cost)

						// Ensure the data field is copied from the source secret
						//secretCopy.Data = secret.Data

						// Log the secret data to check that it's being copied correctly
						logger.Info("Preparing to copy sk1-token to default namespace on ac3-cluster-1", "secretCopy", secretCopy)

						// Create the secret in the namespace on ac3-cluster-1
						_, err = targetClientset.CoreV1().Secrets(targetNamespace).Create(ctx, secretCopy, metav1.CreateOptions{})
						if err != nil {
							if strings.Contains(err.Error(), "already exists") {
								logger.Info("Secret already exists in default namespace on ac3-cluster-1", "secretName2", secret.Name)
								continue
							}
							logger.Error(err, "Failed to create secret in default namespace on ac3-cluster-1", "context", targetContextName)
							return reconcile.Result{}, err
						}

						// Log success and ensure secret contents are correct
						logger.Info("Successfully copied sk1-token to default namespace on ac3-cluster-1",
							"secretName2", secret.Name,
							"targetNamespace", targetNamespace,
							"data", secretCopy.Data)

						// Step to create a ConfigMap in the default namespace on ac3-cluster-1
						configMapData := map[string]string{
							"example.key": "example.value",
						}

						configMap := &corev1.ConfigMap{
							ObjectMeta: metav1.ObjectMeta{
								Name:      "skupper-site",
								Namespace: targetNamespace,
							},
							Data: configMapData,
						}

						// Create the ConfigMap in the namespace on ac3-cluster-1
						_, err = targetClientset.CoreV1().ConfigMaps(targetNamespace).Create(ctx, configMap, metav1.CreateOptions{})
						if err != nil {
							logger.Error(err, "Failed to create ConfigMap in default namespace on ac3-cluster-1", "context", targetContextName)
							return reconcile.Result{}, err
						}

						logger.Info("Successfully created ConfigMap skupper-site in namespace on ac3-cluster-1", "namespace", targetNamespace)

						break
					}
				}
				break
			}
		}
	}

	logger.Info("Reconcile loop completed successfully")
	// I want to have my reconcile look every 30 seconds
	return reconcile.Result{RequeueAfter: 30 * time.Second}, nil

}

// Helper function to create a Secret
func (r *AC3NetworkReconciler) createSecret(ctx context.Context, name string, namespace string) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels: map[string]string{
				"skupper.io/type": "connection-token-request",
				//I want to add a skupper.io cost label that goes +1 for each secret created
			},
			Annotations: map[string]string{
				"skupper.io/cost": "5",
			},
		},
		Data: map[string][]byte{
			"connectionToken": []byte("some-token-data"),
		},
	}
}

// et some var outside of the copysecrets function to keep track of the last cost
// set before copy secret function
// add it to copy secret
// set an pointer to an int inside copysecret function
// use the same int i
// copySecretToNamespace copies a secret to another namespace
func (r *AC3NetworkReconciler) createUpdateSecret(ctx context.Context, sourceNamespace, targetNamespace string, cost *int) error {
	// Retrieve the original secret
	//sleep for 15 seconds
	logger := log.FromContext(ctx)
	time.Sleep(15 * time.Second)

	secret := &corev1.Secret{}

	// Retrieve the Secret from the cluster
	err := r.Get(ctx, client.ObjectKey{Name: "Token", Namespace: sourceNamespace}, secret)
	if err != nil {
		if errors.IsNotFound(err) {
			// Create the Secret if it doesn't exist
			secret = r.createSecret(ctx, "Token", sourceNamespace)

			if err := r.Create(ctx, secret); err != nil {
				logger.Error(err, "Failed to create Secret", "name", "Token", "namespace", sourceNamespace)
				return err
			}
			logger.Info("Created Secret with updated cost", "name", "Token", "namespace", sourceNamespace)
		} else {
			logger.Error(err, "Failed to get Secret", "name", "Token", "namespace", sourceNamespace)
			return err

		}
	}
	//sleep
	time.Sleep(15 * time.Second)

	////------------------------------
	//// Create a deep copy of the secret
	//secretCopy := secret.DeepCopy()
	//// Set the target namespace
	//secretCopy.ObjectMeta.Namespace = targetNamespace
	//// Clear the ResourceVersion for the new object
	//secretCopy.ObjectMeta.ResourceVersion = ""
	//// Attempt to create the secret in the target namespace
	////print out secret copy to log
	//logger.Info("Secret copy", "secret", secretCopy)
	//err = r.Create(ctx, secretCopy)
	//if err != nil && errors.IsAlreadyExists(err) {
	//	// If the secret already exists, update it
	//	existingSecret := &corev1.Secret{}
	//	err = r.Get(ctx, types.NamespacedName{Name: secret.Name, Namespace: targetNamespace}, existingSecret)
	//	if err != nil {
	//		return err
	//	}
	//
	//	// Update the existing secret's data
	//	existingSecret.Data = secretCopy.Data
	//
	//	//set annotation
	//	if cost == nil {
	//		*cost += 10
	//	}
	//
	//	secretCopy.Annotations["skupper.io/cost"] = strconv.Itoa(*cost)
	//	// Retrieve the existing secret from the target namespace
	//	// Update the existing secret in the target namespace
	//	err = r.Update(ctx, existingSecret)
	//	if err != nil {
	//		return err
	//	}
	//	// Update the existing secret's data with the data from the copied secret
	//}
	return err
}

func (r *AC3NetworkReconciler) updateDeploymentsWithSkupperAnnotation(ctx context.Context, sourceNamespace string, appNames []string, logger logr.Logger) error {
	// List all deployments in the sourceNamespace
	deployments := &appsv1.DeploymentList{}
	err := r.List(ctx, deployments, client.InNamespace(sourceNamespace))
	if err != nil {
		logger.Error(err, "Failed to list deployments in source namespace", "namespace", sourceNamespace)
		return err
	}

	// Iterate through deployments and add Skupper annotation to matching deployments
	for _, deployment := range deployments.Items {
		logger.Info("Checking deployment", "deploymentName", deployment.Name)

		// Check if the deployment name matches any app name in the CRD
		for _, appName := range appNames {
			logger.Info("Checking app name", "appName", appName, "deployment", deployment.Name)

			if deployment.Name == appName {
				// Add or update the Skupper annotation
				if deployment.Annotations == nil {
					deployment.Annotations = map[string]string{}
				}
				deployment.Annotations["skupper.io/proxy"] = "tcp"

				// Update the deployment
				err = r.Update(ctx, &deployment)
				if err != nil {
					logger.Error(err, "Failed to update deployment with Skupper annotation", "deploymentName", deployment.Name)
					return err
				}
				logger.Info("Updated deployment with Skupper annotation", "deploymentName", deployment.Name)
			}
		}
	}

	return nil
}

func (r *AC3NetworkReconciler) updateServicesWithSkupperAnnotation(ctx context.Context, sourceNamespace string, serviceNames []string, logger logr.Logger) error {
	// List all services in the sourceNamespace
	servicesList := &corev1.ServiceList{}
	err := r.List(ctx, servicesList, client.InNamespace(sourceNamespace))
	if err != nil {
		logger.Error(err, "Failed to list services in sourceNamespace", "namespace", sourceNamespace)
		return err
	}

	// Iterate through the services defined in the CRD
	for _, serviceName := range serviceNames {
		logger.Info("Checking service", "serviceName", serviceName)

		// Check if the service exists in the namespace
		for _, service := range servicesList.Items {
			if service.Name == serviceName {
				logger.Info("Found matching service", "serviceName", service.Name)

				// Add or update annotations for the service
				if service.Annotations == nil {
					service.Annotations = map[string]string{}
				}
				service.Annotations["skupper.io/target"] = "tcp"

				// Update the service
				err = r.Update(ctx, &service)
				if err != nil {
					logger.Error(err, "Failed to update service", "serviceName", service.Name)
					return err
				}
				logger.Info("Updated service with annotation", "serviceName", service.Name)
			}
		}
	}

	return nil
}

// addCost increments the skupper.io/cost label by 1
// in this below function I want each new link to have a cost of 5 and then each new secret created to go up by 10, right now it is going up from 5 to 15 and then ewach new secret is staying 15, can you try solve this?

// linkSkupperSites links the Skupper sites using the token
//func (r *AC3NetworkReconciler) linkSkupperSites(ctx context.Context, targetNamespace string, secret *corev1.Secret) error {
//	// Create the Skupper link by applying the secret in the target namespace
//	secretToApply := secret.DeepCopy()
//	secretToApply.Namespace = targetNamespace
//
//	// Clear the ResourceVersion to ensure it's a new object creation
//	secretToApply.ResourceVersion = ""
//
//	err := r.Create(ctx, secretToApply)
//	if err != nil && errors.IsAlreadyExists(err) {
//		// If already exists, update it
//		existingSecret := &corev1.Secret{}
//		err = r.Get(ctx, types.NamespacedName{Name: secret.Name, Namespace: targetNamespace}, existingSecret)
//		if err != nil {
//			return err
//		}
//		existingSecret.Data = secretToApply.Data
//		err = r.Update(ctx, existingSecret)
//		if err != nil {
//			return err
//		}
//	} else if err != nil {
//		return err
//	}
//
//	return nil
//}

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
									Image: "quay.io/ryjenkin/ac3no3:224",
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
		Complete(r)
}
