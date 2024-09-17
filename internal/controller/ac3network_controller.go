package controller

import (
    "context"
    "fmt"
    "os/exec"
    "time"

    corev1 "k8s.io/api/core/v1"
    appsv1 "k8s.io/api/apps/v1"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    "k8s.io/apimachinery/pkg/runtime"
    "k8s.io/apimachinery/pkg/runtime/schema"
    "k8s.io/apimachinery/pkg/types"
    // "k8s.io/apimachinery/pkg/util/intstr"
    "k8s.io/client-go/kubernetes"
    "k8s.io/client-go/tools/clientcmd"
    "sigs.k8s.io/controller-runtime/pkg/reconcile"
    "k8s.io/apimachinery/pkg/api/resource"
    "sigs.k8s.io/controller-runtime/pkg/client"
    "k8s.io/apimachinery/pkg/api/errors"
    // "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
    "sigs.k8s.io/controller-runtime/pkg/log"
    ctrl "sigs.k8s.io/controller-runtime"

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
        "example.key": "example.value",
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
    secretNamespace := "sk1"
    secretName := "sk1-token"
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

    // 9. Iterate through contexts and switch to ac3-cluster-1 before copying the token
    // This ensures we are in the right context when copying the token to the default namespace of ac3-cluster-1
    targetContextName := "ac3-cluster-1"
    for contextName, _ := range kubeconfig.Contexts {
        if contextName == targetContextName {
            logger.Info("Switching context to ac3-cluster-1", "context", contextName)
            _, err := clientcmd.NewNonInteractiveClientConfig(*kubeconfig, contextName, nil, nil).ClientConfig()
            if err != nil {
                logger.Error(err, "Failed to create Kubernetes client config for ac3-cluster-1", "context", contextName)
                return reconcile.Result{}, err
            }

            // Copy the token to the default namespace on ac3-cluster-1
            err = r.copySecretToNamespace(ctx, secret, "default")
            if err != nil {
                logger.Error(err, "Failed to copy Secret to default namespace on ac3-cluster-1")
                return reconcile.Result{}, err
            }

            logger.Info("Secret sk1-token copied to default namespace on ac3-cluster-1 successfully")
            break
        }
    }

    logger.Info("Reconcile loop completed successfully", "request", req)

    return reconcile.Result{}, nil
}






// Helper function to create a Secret
func (r *AC3NetworkReconciler) createSecret(ctx context.Context, name string, namespace string) *corev1.Secret {
    return &corev1.Secret{
        ObjectMeta: metav1.ObjectMeta{
            Name:      name,
            Namespace: namespace,
        },
        Data: map[string][]byte{
            "example.key": []byte("example.value"),
        },
    }
}

// Helper function to copy a Secret to another namespace
func (r *AC3NetworkReconciler) copySecretToNamespace(ctx context.Context, secret *corev1.Secret, targetNamespace string) error {
    logger := log.FromContext(ctx)
    logger.Info("Copying Secret to another namespace", "sourceNamespace", secret.Namespace, "targetNamespace", targetNamespace)

    time.Sleep(15 * time.Second) // Wait for 15 seconds

    err := r.Get(ctx, types.NamespacedName{Name: secret.Name, Namespace: secret.Namespace}, secret)
    if err != nil {
        logger.Error(err, "Failed to retrieve original Secret after sleep", "name", secret.Name, "namespace", secret.Namespace)
        return err
    }

    newSecret := &corev1.Secret{
        ObjectMeta: metav1.ObjectMeta{
            Name:      secret.Name,
            Namespace: targetNamespace,
            Labels:    secret.Labels,
        },
        Data: secret.Data,
    }

    err = r.Create(ctx, newSecret)
    if err != nil {
        logger.Error(err, "Failed to copy Secret to target namespace", "targetNamespace", targetNamespace)
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
                                    Image: "quay.io/ryjenkin/ac3no3:100",
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

