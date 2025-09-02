package controller

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"k8s.io/utils/pointer"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"

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
type NetworkReconciler struct {
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

// +kubebuilder:rbac:groups=ac3.redhat.com,resources=multiclusternetworks,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=ac3.redhat.com,resources=multiclusternetworks/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=ac3.redhat.com,resources=multiclusternetworks/finalizers,verbs=update
// +kubebuilder:rbac:groups=ac3.redhat.com,resources=skupperrouters,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=ac3.redhat.com,resources=skupperrouters/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=ac3.redhat.com,resources=skupperrouters/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=secrets;configmaps,verbs=get;list;watch;create;update;patch;delete

// Reconcile reads that state of the cluster for a Network object and makes changes based on the state read
func (r *NetworkReconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("Starting Reconcile loop", "request", req)
	ctx = logr.NewContext(ctx, logger)
	linkCR := &ac3v1alpha1.MultiClusterNetwork{}

	logger.Info("Fetching Link resource", "namespace", req.Namespace, "name", req.Name)

	if err := r.Client.Get(ctx, req.NamespacedName, linkCR); err != nil {
		if errors.IsNotFound(err) {
			logger.Error(err, "Link resource not found, possibly deleted")
			return reconcile.Result{}, nil
		}
		logger.Error(err, "Failed to get Link")
		return reconcile.Result{}, err
	}

	//logger.Info("CR Detail", "SourceCluster", link.Spec)

	// 1. Fetch the ConfigMap with the combined kubeconfig
	configMap := &corev1.ConfigMap{}
	err := r.Get(ctx, client.ObjectKey{Name: "combined-kubeconfig", Namespace: "sk1"}, configMap)
	if err != nil {
		logger.Error(err, "Failed to get ConfigMap", "name", "combined-kubeconfig", "namespace", "sk1")
		return reconcile.Result{}, err
	}

	// 2. Extract kubeconfig content
	kubeconfigContent, ok := configMap.Data["kubeconfig"]
	if !ok {
		err := fmt.Errorf("kubeconfig not found")
		logger.Error(err, "ConfigMap does not contain kubeconfig", "name", "combined-kubeconfig")
		return reconcile.Result{}, err
	}

	// 3. Parse kubeconfig
	kubeconfig, err := clientcmd.Load([]byte(kubeconfigContent))
	if err != nil {
		logger.Error(err, "Failed to parse kubeconfig")
		return reconcile.Result{}, err
	}

	// 5. Manage ConfigMaps in different namespaces
	//put in a for loop and putit in a get to retrieve the ac3network
	multiclusterNetwork := &ac3v1alpha1.MultiClusterNetwork{}
	if err := r.Get(ctx, req.NamespacedName, multiclusterNetwork); err != nil {
		logger.Error(err, "Failed to fetch MultiClusterNetwork")
		return reconcile.Result{}, client.IgnoreNotFound(err)
	}
	//make a for loop for each link
	//retrieving CRD and going through each link
	logger.Info("Expect Not Here")
	for _, link := range multiclusterNetwork.Spec.Links {
		logger.Info("Not Here")
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

		err = r.createUpdateSecret(ctx, kubeconfig, link.SourceCluster, link.SourceNamespace, pointer.Int(5))
		if err != nil {
			logger.Error(err, "Failed to copy secret to namespace")
			return reconcile.Result{}, err
		}

		logger.Info("HERE")

		// Create ConfigMap in source cluster using source cluster client
		sourceConfig, err := clientcmd.NewNonInteractiveClientConfig(*kubeconfig, link.SourceCluster, nil, nil).ClientConfig()
		if err != nil {
			logger.Error(err, "Failed to create Kubernetes client config for source cluster", "context", link.SourceCluster)
			return reconcile.Result{}, err
		}

		sourceClientset, err := kubernetes.NewForConfig(sourceConfig)
		if err != nil {
			logger.Error(err, "Failed to create Kubernetes clientset for source cluster", "context", link.SourceCluster)
			return reconcile.Result{}, err
		}

		// Create ConfigMap in source cluster
		sourceConfigMap := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      configMapName,
				Namespace: link.SourceNamespace,
			},
			Data: data,
		}

		_, err = sourceClientset.CoreV1().ConfigMaps(link.SourceNamespace).Create(ctx, sourceConfigMap, metav1.CreateOptions{})
		if err != nil {
			if strings.Contains(err.Error(), "already exists") {
				logger.Info("ConfigMap already exists in source cluster", "name", configMapName, "namespace", link.SourceNamespace, "cluster", link.SourceCluster)
			} else {
				logger.Error(err, "Failed to create ConfigMap in source cluster", "context", link.SourceCluster, "namespace", link.SourceNamespace)
				return reconcile.Result{}, err
			}
		} else {
			logger.Info("Successfully created ConfigMap in source cluster", "name", configMapName, "namespace", link.SourceNamespace, "cluster", link.SourceCluster)
		}

		// Also create ConfigMap in target cluster if needed
		for _, namespace := range configMapNamespaces {
			// Skip source namespace as we already handled it above
			if namespace == link.SourceNamespace {
				continue
			}

			logger.Info("Creating ConfigMap in target namespace", "namespace", namespace)
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
					// Continue with next namespace instead of returning
					continue
				}
				logger.Info("Created ConfigMap", "name", configMapName, "namespace", namespace)

			} else {
				//log here
				logger.Info("ConfigMap already exists", "name", configMapName, "namespace", namespace)

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

		// Check if there are services to process and call exposeService function
		if len(serviceNames) > 0 {
			err = r.exposeService(ctx, kubeconfig, sourceCluster, targetCluster, sourceNamespace, targetNamespace, serviceNames, link.Port, logger)
			if err != nil {
				logger.Error(err, "Failed to expose services")
				return reconcile.Result{}, err
			}
		}

		// Step 1: Switch to cluster-2 and get the secret from the source namespace
		for contextName, _ := range kubeconfig.Contexts {
			if contextName == sourceCluster {
				logger.Info("Switching context to cluster-2", "context", contextName)

				// Get Kubernetes client for cluster-2
				sourceConfig, err := clientcmd.NewNonInteractiveClientConfig(*kubeconfig, contextName, nil, nil).ClientConfig()
				if err != nil {
					logger.Error(err, "Failed to create Kubernetes client config for cluster-2", "context", contextName)
					return reconcile.Result{}, err
				}

				sourceClientset, err := kubernetes.NewForConfig(sourceConfig)
				if err != nil {
					logger.Error(err, "Failed to create Kubernetes clientset for cluster-2", "context", contextName)
					return reconcile.Result{}, err
				}

				// Wait for Skupper to generate the real token
				var secret *corev1.Secret
				for i := 0; i < 30; i++ { // Wait up to 30 seconds
					secret, err = sourceClientset.CoreV1().Secrets(sourceNamespace).Get(ctx, "token", metav1.GetOptions{})
					if err == nil && secret.Labels["skupper.io/type"] == "connection-token" {
						// Real token generated!
						break
					}
					time.Sleep(1 * time.Second)
				}
				if err != nil {
					logger.Error(err, "Failed to get secret from source namespace in cluster-2", "namespace", sourceNamespace)
					return reconcile.Result{}, err
				}

				logger.Info("Successfully retrieved token from cluster-2", "secretName2", secret)

				// Step 2: Switch to cluster-1 and copy the secret to the default namespace
				for targetContextName, _ := range kubeconfig.Contexts {
					if targetContextName == targetCluster {
						logger.Info("Switching context to cluster-1", "context", targetContextName)

						// Get Kubernetes client for cluster-1
						targetConfig, err := clientcmd.NewNonInteractiveClientConfig(*kubeconfig, targetContextName, nil, nil).ClientConfig()
						if err != nil {
							logger.Error(err, "Failed to create Kubernetes client config for cluster-1", "context", targetContextName)
							return reconcile.Result{}, err
						}

						targetClientset, err := kubernetes.NewForConfig(targetConfig)
						if err != nil {
							logger.Error(err, "Failed to create Kubernetes clientset for cluster-1", "context", targetContextName)
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
						logger.Info("Preparing to copy token to default namespace on cluster-1", "secretCopy", secretCopy)

						// Create the secret in the namespace on cluster-1
						_, err = targetClientset.CoreV1().Secrets(targetNamespace).Create(ctx, secretCopy, metav1.CreateOptions{})
						if err != nil {
							if strings.Contains(err.Error(), "already exists") {
								logger.Info("Secret already exists in default namespace on cluster-1", "secretName2", secret.Name)
								continue
							}
							logger.Error(err, "Failed to create secret in default namespace on cluster-1", "context", targetContextName)
							return reconcile.Result{}, err
						}

						// Log success and ensure secret contents are correct
						logger.Info("Successfully copied token to default namespace on cluster-1",
							"secretName2", secret.Name,
							"targetNamespace", targetNamespace,
							"data", secretCopy.Data)

						// Step to create a ConfigMap in the default namespace on cluster-1
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

						// Create the ConfigMap in the namespace on cluster-1
						_, err = targetClientset.CoreV1().ConfigMaps(targetNamespace).Create(ctx, configMap, metav1.CreateOptions{})
						if err != nil {
							logger.Error(err, "Failed to create ConfigMap in default namespace on cluster-1", "context", targetContextName)
							return reconcile.Result{}, err
						}

						logger.Info("Successfully created ConfigMap skupper-site in namespace on cluster-1", "namespace", targetNamespace)

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
func (r *NetworkReconciler) createSecret(ctx context.Context, name string, namespace string) *corev1.Secret {
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
// createUpdateSecret creates the secret in the correct cluster context
func (r *NetworkReconciler) createUpdateSecret(ctx context.Context, kubeconfig *clientcmdapi.Config, clusterName, namespace string, cost *int) error {
	logger := log.FromContext(ctx)

	//log the cluster name
	logger.Info("Creating secret in cluster", "cluster", clusterName)
	config, err := clientcmd.NewNonInteractiveClientConfig(*kubeconfig, clusterName, nil, nil).ClientConfig()
	if err != nil {
		logger.Error(err, "Failed to get client config for cluster", "cluster", clusterName)
		return err
	}
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		logger.Error(err, "Failed to create clientset for cluster", "cluster", clusterName)
		return err
	}

	// Try to get the secret
	_, err = clientset.CoreV1().Secrets(namespace).Get(ctx, "token", metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "token",
					Namespace: namespace,
					Labels: map[string]string{
						"skupper.io/type": "connection-token-request",
					},
					Annotations: map[string]string{
						"skupper.io/cost": fmt.Sprintf("%d", *cost),
					},
				},
				Data: map[string][]byte{
					"connectionToken": []byte("some-token-data"),
				},
			}
			_, err = clientset.CoreV1().Secrets(namespace).Create(ctx, secret, metav1.CreateOptions{})
			if err != nil {
				logger.Error(err, "Failed to create Secret", "name", "token", "namespace", namespace)
				return err
			}
			logger.Info("Created Secret with updated cost", "name", "token", "namespace", namespace)
		} else {
			logger.Error(err, "Failed to get Secret", "name", "token", "namespace", namespace)
			return err
		}
	}
	return nil
}

func (r *NetworkReconciler) updateDeploymentsWithSkupperAnnotation(ctx context.Context, sourceNamespace string, appNames []string, logger logr.Logger) error {
	// List all deployments in the sourceNamespace
	deployments := &appsv1.DeploymentList{}
	err := r.List(ctx, deployments, client.InNamespace(sourceNamespace))
	if err != nil {
		logger.Error(err, "Failed to list deployments in source namespace", "namespace", sourceNamespace)
		return err
	}

	// Iterate through deployments and add Skupper annotation to matching deployments
	for _, deployment := range deployments.Items {
		// logger.Info("Checking deployment", "deploymentName", deployment.Name)

		// Check if the deployment name matches any app name in the CRD
		for _, appName := range appNames {
			//logger.Info("Checking app name", "appName", appName, "deployment", deployment.Name)

			if deployment.Name == appName {
				// Add or update the Skupper annotation
				if deployment.Annotations == nil {
					deployment.Annotations = map[string]string{}
				}
				deployment.Annotations["skupper.io/proxy"] = "tcp"
				deployment.Annotations["skupper.io/port"] = "8080"
				deployment.Annotations["skupper.io/address"] = deployment.Name

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

func (r *NetworkReconciler) createSkupperProxyServices(ctx context.Context, kubeconfig *clientcmdapi.Config, sourceCluster, targetCluster, sourceNamespace, targetNamespace string, serviceNames []string, port int, logger logr.Logger) error {
	// Get the original services from source cluster, create proxy services on target cluster
	logger.Info("Creating Skupper proxy services on target cluster", "targetCluster", targetCluster, "targetNamespace", targetNamespace, "serviceNames", serviceNames)

	// Get Kubernetes client for source cluster to read original services
	sourceConfig, err := clientcmd.NewNonInteractiveClientConfig(*kubeconfig, sourceCluster, nil, nil).ClientConfig()
	if err != nil {
		logger.Error(err, "Failed to create Kubernetes client config for source cluster", "context", sourceCluster)
		return err
	}

	sourceClientset, err := kubernetes.NewForConfig(sourceConfig)
	if err != nil {
		logger.Error(err, "Failed to create Kubernetes clientset for source cluster", "context", sourceCluster)
		return err
	}

	// List all services in the sourceNamespace on source cluster
	servicesList, err := sourceClientset.CoreV1().Services(sourceNamespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		logger.Error(err, "Failed to list services in sourceNamespace", "namespace", sourceNamespace)
		return err
	}

	// Iterate through the services defined in the CRD
	for _, serviceName := range serviceNames {
		logger.Info("Checking service", "serviceName", serviceName)

		// Find the original service
		var originalService *corev1.Service
		for _, service := range servicesList.Items {
			if service.Name == serviceName {
				originalService = &service
				break
			}
		}

		if originalService == nil {
			logger.Info("Original service not found", "serviceName", serviceName)
			continue
		}

		logger.Info("Found original service", "serviceName", originalService.Name)

		// Create the new service with -skupper suffix
		skupperServiceName := serviceName + "-skupper"

		// Check if the skupper service already exists on target cluster
		_, err = sourceClientset.CoreV1().Services(sourceNamespace).Get(ctx, skupperServiceName, metav1.GetOptions{})
		if err == nil {
			logger.Info("Skupper service already exists", "serviceName", skupperServiceName)
			continue
		}

		// Create new service with copied spec and labels
		skupperServiceSpec := originalService.Spec.DeepCopy()
		// Clear IP allocation fields to let Kubernetes assign new IPs
		skupperServiceSpec.ClusterIP = ""
		skupperServiceSpec.ClusterIPs = nil

		skupperService := &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      skupperServiceName,
				Namespace: sourceNamespace,
				Labels:    originalService.Labels,
				Annotations: map[string]string{
					"skupper.io/proxy":  "tcp",
					"skupper.io/port":   fmt.Sprintf("%d", port),
					"skupper.io/target": serviceName, // Target is the original service
				},
			},
			Spec: *skupperServiceSpec,
		}

		//print out contents of spec
		logger.Info("Skupper service spec", "spec", skupperServiceSpec)
		logger.Info("Skupper service labels", "labels", skupperService.Labels)

		// Create the new service on source cluster
		_, err = sourceClientset.CoreV1().Services(sourceNamespace).Create(ctx, skupperService, metav1.CreateOptions{})
		if err != nil {
			logger.Error(err, "Failed to create skupper service", "serviceName", skupperServiceName)
			return err
		}
		logger.Info("Created skupper service on source cluster", "serviceName", skupperServiceName, "cluster", sourceCluster)
	}

	return nil
}

// createExternalNameServices creates ExternalName services on target clusters
func (r *NetworkReconciler) createExternalNameServices(ctx context.Context, kubeconfig *clientcmdapi.Config, targetCluster, targetNamespace, sourceNamespace string, serviceNames []string, logger logr.Logger) error {
	logger.Info("Creating ExternalName services on target cluster", "cluster", targetCluster, "namespace", targetNamespace)

	// Get Kubernetes client for target cluster
	targetConfig, err := clientcmd.NewNonInteractiveClientConfig(*kubeconfig, targetCluster, nil, nil).ClientConfig()
	if err != nil {
		logger.Error(err, "Failed to create Kubernetes client config for target cluster", "context", targetCluster)
		return err
	}

	targetClientset, err := kubernetes.NewForConfig(targetConfig)
	if err != nil {
		logger.Error(err, "Failed to create Kubernetes clientset for target cluster", "context", targetCluster)
		return err
	}

	// Create ExternalName service for each service in the CRD
	for _, serviceName := range serviceNames {
		logger.Info("Creating ExternalName service", "serviceName", serviceName)

		// Check if the service already exists
		_, err = targetClientset.CoreV1().Services(targetNamespace).Get(ctx, serviceName, metav1.GetOptions{})
		if err == nil {
			logger.Info("ExternalName service already exists", "serviceName", serviceName)
			continue
		}

		// Create ExternalName service pointing to the skupper service in target namespace
		skupperServiceFQDN := fmt.Sprintf("%s-skupper.%s.svc.cluster.local", serviceName, targetNamespace)

		externalNameService := &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      serviceName,
				Namespace: targetNamespace,
			},
			Spec: corev1.ServiceSpec{
				Type:         corev1.ServiceTypeExternalName,
				ExternalName: skupperServiceFQDN,
			},
		}

		// Create the ExternalName service
		_, err = targetClientset.CoreV1().Services(targetNamespace).Create(ctx, externalNameService, metav1.CreateOptions{})
		if err != nil {
			if strings.Contains(err.Error(), "already exists") {
				logger.Info("ExternalName service already exists", "serviceName", serviceName)
				continue
			}
			logger.Error(err, "Failed to create ExternalName service", "serviceName", serviceName)
			return err
		}

		logger.Info("Successfully created ExternalName service",
			"serviceName", serviceName,
			"namespace", targetNamespace,
			"externalName", skupperServiceFQDN)
	}

	return nil
}

// logSkupperLinkStatus logs the status of the Skupper link for a given namespace
func (r *NetworkReconciler) logSkupperLinkStatus(ctx context.Context, namespace string) error {
	cmd := exec.Command("skupper", "link", "status", "-n", namespace)
	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("failed to get Skupper link status: %v", err)
	}

	log.FromContext(ctx).Info("Skupper link status", "namespace", namespace, "status", string(output))
	return nil
}

// reconcileSkupperRouter reconciles a SkupperRouter instance
func (r *NetworkReconciler) reconcileSkupperRouter(ctx context.Context, routerInstance SkupperRouter) error {
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
									Image: "quay.io/ryjenkin/ac3no3:282",
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
func (r *NetworkReconciler) createConfigMap(ctx context.Context, name string, namespace string, data map[string]string) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Data: data,
	}
}

// Helper function to determine if a ConfigMap needs updating
func (r *NetworkReconciler) needsUpdateConfigMap(configMap *corev1.ConfigMap, data map[string]string) bool {
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

// exposeService processes services based on their ownership structure
func (r *NetworkReconciler) exposeService(ctx context.Context, kubeconfig *clientcmdapi.Config, sourceCluster, targetCluster, sourceNamespace, targetNamespace string, serviceNames []string, port int, logger logr.Logger) error {
	logger.Info("Processing services for exposure", "sourceCluster", sourceCluster, "targetCluster", targetCluster, "serviceNames", serviceNames)

	// Get Kubernetes client for source cluster to read services
	sourceConfig, err := clientcmd.NewNonInteractiveClientConfig(*kubeconfig, sourceCluster, nil, nil).ClientConfig()
	if err != nil {
		logger.Error(err, "Failed to create Kubernetes client config for source cluster", "context", sourceCluster)
		return err
	}

	sourceClientset, err := kubernetes.NewForConfig(sourceConfig)
	if err != nil {
		logger.Error(err, "Failed to create Kubernetes clientset for source cluster", "context", sourceCluster)
		return err
	}

	// List all services in the sourceNamespace on source cluster
	servicesList, err := sourceClientset.CoreV1().Services(sourceNamespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		logger.Error(err, "Failed to list services in sourceNamespace", "namespace", sourceNamespace)
		return err
	}

	// Iterate through the services defined in the CRD
	for _, serviceName := range serviceNames {
		logger.Info("Processing service", "serviceName", serviceName)

		// Find the original service
		var originalService *corev1.Service
		for _, service := range servicesList.Items {
			if service.Name == serviceName {
				originalService = &service
				break
			}
		}

		if originalService == nil {
			logger.Info("Original service not found", "serviceName", serviceName)
			continue
		}

		logger.Info("Found original service", "serviceName", originalService.Name)

		// Check if service has AppliedManifestWork owner
		hasAppliedManifestWorkOwner := false
		if originalService.OwnerReferences != nil {
			for _, ownerRef := range originalService.OwnerReferences {
				if ownerRef.Kind == "AppliedManifestWork" {
					hasAppliedManifestWorkOwner = true
					logger.Info("Service has AppliedManifestWork owner", "serviceName", serviceName, "ownerKind", ownerRef.Kind, "ownerName", ownerRef.Name)
					break
				}
			}
		}

		if hasAppliedManifestWorkOwner {
			// Service has AppliedManifestWork owner - use full proxy/external name approach
			logger.Info("Using full proxy/external name approach for service with AppliedManifestWork owner", "serviceName", serviceName)

			// Call createSkupperProxyServices for this specific service
			err = r.createSkupperProxyServices(ctx, kubeconfig, sourceCluster, targetCluster, sourceNamespace, targetNamespace, []string{serviceName}, port, logger)
			if err != nil {
				logger.Error(err, "Failed to create Skupper proxy services for service with AppliedManifestWork owner", "serviceName", serviceName)
				return err
			}

			// Call createExternalNameServices for this specific service
			err = r.createExternalNameServices(ctx, kubeconfig, targetCluster, targetNamespace, sourceNamespace, []string{serviceName}, logger)
			if err != nil {
				logger.Error(err, "Failed to create ExternalName services for service with AppliedManifestWork owner", "serviceName", serviceName)
				return err
			}
		} else {
			// Service has no AppliedManifestWork owner - use simple annotation approach
			logger.Info("Using simple annotation approach for service without AppliedManifestWork owner", "serviceName", serviceName)

			err = r.annotateService(ctx, sourceClientset, sourceNamespace, serviceName, port, logger)
			if err != nil {
				logger.Error(err, "Failed to annotate service", "serviceName", serviceName)
				return err
			}
		}
	}

	return nil
}

// annotateService simply annotates a service with Skupper annotations
func (r *NetworkReconciler) annotateService(ctx context.Context, clientset *kubernetes.Clientset, namespace, serviceName string, port int, logger logr.Logger) error {
	logger.Info("Annotating service", "serviceName", serviceName, "namespace", namespace)

	// Get the service
	service, err := clientset.CoreV1().Services(namespace).Get(ctx, serviceName, metav1.GetOptions{})
	if err != nil {
		logger.Error(err, "Failed to get service for annotation", "serviceName", serviceName, "namespace", namespace)
		return err
	}

	// Initialize annotations if nil
	if service.Annotations == nil {
		service.Annotations = map[string]string{}
	}

	// Add or update Skupper annotations
	service.Annotations["skupper.io/proxy"] = "tcp"
	service.Annotations["skupper.io/port"] = fmt.Sprintf("%d", port)

	// Update the service
	_, err = clientset.CoreV1().Services(namespace).Update(ctx, service, metav1.UpdateOptions{})
	if err != nil {
		logger.Error(err, "Failed to update service with annotations", "serviceName", serviceName, "namespace", namespace)
		return err
	}

	logger.Info("Successfully annotated service", "serviceName", serviceName, "namespace", namespace)
	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *NetworkReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&ac3v1alpha1.MultiClusterNetwork{}).
		Complete(r)
}
