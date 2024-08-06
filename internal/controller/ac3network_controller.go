package controller

import (
    "context"
    "fmt"
    "time"

    "k8s.io/apimachinery/pkg/runtime"
    "k8s.io/apimachinery/pkg/runtime/schema"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    ctrl "sigs.k8s.io/controller-runtime"
    "sigs.k8s.io/controller-runtime/pkg/client"
    "sigs.k8s.io/controller-runtime/pkg/log"
    "k8s.io/apimachinery/pkg/api/errors"
    "os/exec"


    ac3v1alpha1 "github.com/raycarroll/ac3no/api/v1alpha1"
    appsv1 "k8s.io/api/apps/v1"
    "k8s.io/apimachinery/pkg/types"
    corev1 "k8s.io/api/core/v1"
    "k8s.io/apimachinery/pkg/api/resource"
    

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

func (r *AC3NetworkReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    logger := log.FromContext(ctx)

    logger.Info("Starting Reconcile loop", "request", req)

    // Define the namespaces for ConfigMap
    configMapNamespaces := []string{"sk1", "sk2"}

    // Handle the ConfigMap creation and update in both sk1 and sk2 namespaces
    for _, namespace := range configMapNamespaces {
        configMapName := "skupper-site"
        data := map[string]string{
            "example.key": "example.value",
        }

        // Check if the ConfigMap exists
        configMap := &corev1.ConfigMap{}
        err := r.Get(ctx, types.NamespacedName{Name: configMapName, Namespace: namespace}, configMap)
        if err != nil {
            if client.IgnoreNotFound(err) != nil {
                logger.Error(err, "Failed to get ConfigMap", "name", configMapName, "namespace", namespace)
                return ctrl.Result{}, err
            }

            // If ConfigMap is not found, create it
            configMap = r.createConfigMap(ctx, configMapName, namespace, data)
            if err := r.Create(ctx, configMap); err != nil {
                logger.Error(err, "Failed to create ConfigMap", "name", configMapName, "namespace", namespace)
                return ctrl.Result{}, err
            }
            logger.Info("Created ConfigMap", "name", configMapName, "namespace", namespace)
        } else {
            // If ConfigMap is found, check if it needs to be updated
            if r.needsUpdateConfigMap(configMap, data) {
                configMap.Data = data
                if err := r.Update(ctx, configMap); err != nil {
                    logger.Error(err, "Failed to update ConfigMap", "name", configMapName, "namespace", namespace)
                    return ctrl.Result{}, err
                }
                logger.Info("Updated ConfigMap", "name", configMapName, "namespace", namespace)
            }
        }
    }

    // Handle the Secret only for the sk1 namespace
    secretNamespace := "sk1"
    secretName := "sk1-token"

    // Check if the Secret exists in sk1 namespace
    secret := &corev1.Secret{}
    err := r.Get(ctx, types.NamespacedName{Name: secretName, Namespace: secretNamespace}, secret)
    if err != nil && errors.IsNotFound(err) {
        // If Secret is not found, create it
        secret = r.createSecret(ctx, secretName, secretNamespace)
        if err := r.Create(ctx, secret); err != nil {
            logger.Error(err, "Failed to create Secret", "name", secretName, "namespace", secretNamespace)
            return ctrl.Result{}, err
        }
        logger.Info("Created Secret", "name", secretName, "namespace", secretNamespace)
    } else if err != nil {
        logger.Error(err, "Failed to get Secret", "name", secretName, "namespace", secretNamespace)
        return ctrl.Result{}, err
    }

    // // Ensure the Secret has the required label
    // if value, ok := secret.Labels["skupper.io/type"]; !ok || value != "connection-token-request" {
    //     if secret.Labels == nil {
    //         secret.Labels = make(map[string]string)
    //     }
    //     secret.Labels["skupper.io/type"] = "connection-token-request"
        
        // // Update the secret and remove the namespace field after updating
        // if err := r.Update(ctx, secret); err != nil {
        //     logger.Error(err, "Failed to label Secret", "name", secretName, "namespace", secretNamespace)
        //     return ctrl.Result{}, err
        // }
        // logger.Info("Labeled Secret", "name", secretName, "namespace", secretNamespace)

        // // Remove the namespace from the secret after update
        // secret.ObjectMeta.Namespace = ""
    //}

    // Now copy the secret to the sk2 namespace
    err = r.copySecretToNamespace(ctx, secret, "sk2")
    if err != nil {
        logger.Error(err, "Failed to copy Secret to sk2 namespace", "name", secretName)
        return ctrl.Result{}, err
    }
    // Log the success of copying the sk1-token to the sk2 namespace
    logger.Info("Secret sk1-token copied to namespace sk2 successfully")

    // Link the namespaces using the Skupper token
    // err = r.linkSkupperSites(ctx, "sk2", secret)
    // if err != nil {
    //     logger.Error(err, "Failed to link Skupper sites", "sourceNamespace", secretNamespace, "targetNamespace", "sk2")
    //     return ctrl.Result{}, err
    // }
    logger.Info("Skupper sites linked successfully", "sourceNamespace", secretNamespace, "targetNamespace", "sk2")

    // Log the Skupper link status
    err = r.logSkupperLinkStatus(ctx, "sk2")
    if err != nil {
        logger.Error(err, "Failed to get Skupper link status", "namespace", "sk2")
        return ctrl.Result{}, err
    }

    // Fetch the AC3Network instance
    ac3Network := &ac3v1alpha1.AC3Network{}
    if err := r.Get(ctx, req.NamespacedName, ac3Network); err != nil {
        logger.Error(err, "Failed to fetch AC3Network")
        return ctrl.Result{}, client.IgnoreNotFound(err)
    }

    // List all instances of SkupperRouter
    routerList := SkupperRouterList{}
    if err := r.List(ctx, &routerList, client.InNamespace(req.Namespace)); err != nil {
        logger.Error(err, "Failed to list SkupperRouter instances")
        return ctrl.Result{}, err
    }

    logger.Info("SkupperRouter instances listed", "count", len(routerList.Items))

    // Iterate over each instance of SkupperRouter
    for _, routerInstance := range routerList.Items {
        // Log the SkupperRouter instance details
        logger.Info("SkupperRouter instance found", "name", routerInstance.Name, "namespace", routerInstance.Namespace)
        // Handle SkupperRouter instances
        err = r.reconcileSkupperRouter(ctx, routerInstance)
        if err != nil {
            logger.Error(err, "Failed to reconcile SkupperRouter", "name", routerInstance.Name, "namespace", routerInstance.Namespace)
            return ctrl.Result{}, err
        }
    }

    logger.Info("Reconcile loop completed", "request", req)

    return ctrl.Result{}, nil
}



// createSecret creates a new Secret with the specified name and namespace
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


// createConfigMap creates a new ConfigMap with the specified name, namespace, and data
func (r *AC3NetworkReconciler) createConfigMap(ctx context.Context, name string, namespace string, data map[string]string) *corev1.ConfigMap {
    return &corev1.ConfigMap{
        ObjectMeta: metav1.ObjectMeta{
            Name:      name,
            Namespace: namespace,
        },
        Data: data,
    }
}

// needsUpdateConfigMap determines if a ConfigMap needs to be updated based on its data
func (r *AC3NetworkReconciler) needsUpdateConfigMap(configMap *corev1.ConfigMap, data map[string]string) bool {
    if len(configMap.Data) != len(data) {
        return true
    }
    for key, value := range data {
        if configMap.Data[key] != value {
            return true
        }
    }
    return false
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


// logSkupperLinkStatus simulates logging the Skupper link status for a given namespace
func (r *AC3NetworkReconciler) logSkupperLinkStatus(ctx context.Context, namespace string) error {
    cmd := exec.Command("skupper", "link", "status", "--namespace", namespace)
    out, err := cmd.CombinedOutput()
    if err != nil {
        return fmt.Errorf("failed to execute skupper link status: %v", err)
    }
    log.FromContext(ctx).Info("Skupper link status", "namespace", namespace, "output", string(out))
    return nil
}

// reconcileSkupperRouter manages individual SkupperRouter instances
func (r *AC3NetworkReconciler) reconcileSkupperRouter(ctx context.Context, instance SkupperRouter) error {
    deployment := &appsv1.Deployment{}
    err := r.Get(ctx, types.NamespacedName{Name: instance.Name, Namespace: instance.Namespace}, deployment)
    if err != nil {
        if errors.IsNotFound(err) {
            deployment = r.createSkupperRouterDeployment(ctx, instance)
            if err := r.Create(ctx, deployment); err != nil {
                return fmt.Errorf("failed to create deployment: %v", err)
            }
            log.FromContext(ctx).Info("Created Deployment", "name", deployment.Name, "namespace", deployment.Namespace)
        } else {
            return fmt.Errorf("failed to get deployment: %v", err)
        }
    } else {
        if r.needsUpdateSkupperRouterDeployment(deployment, instance) {
            deployment.Spec = r.createSkupperRouterDeployment(ctx, instance).Spec
            if err := r.Update(ctx, deployment); err != nil {
                return fmt.Errorf("failed to update deployment: %v", err)
            }
            log.FromContext(ctx).Info("Updated Deployment", "name", deployment.Name, "namespace", deployment.Namespace)
        }
    }
    return nil
}

// createSkupperRouterDeployment creates a new Deployment for a SkupperRouter instance
func (r *AC3NetworkReconciler) createSkupperRouterDeployment(ctx context.Context, instance SkupperRouter) *appsv1.Deployment {
    replicas := int32(1)
    return &appsv1.Deployment{
        ObjectMeta: metav1.ObjectMeta{
            Name:      instance.Name,
            Namespace: instance.Namespace,
        },
        Spec: appsv1.DeploymentSpec{
            Replicas: &replicas,
            Selector: &metav1.LabelSelector{
                MatchLabels: map[string]string{
                    "app": instance.Name,
                },
            },
            Template: corev1.PodTemplateSpec{
                ObjectMeta: metav1.ObjectMeta{
                    Labels: map[string]string{
                        "app": instance.Name,
                    },
                },
                Spec: corev1.PodSpec{
                    Containers: []corev1.Container{
                        {
                            Name:  "skupper-router",
                            Image: "quay.io/ryjenkin/ac3no3:59",
                            Resources: corev1.ResourceRequirements{
                                Limits: corev1.ResourceList{
                                    corev1.ResourceCPU:    resource.MustParse("500m"),
                                    corev1.ResourceMemory: resource.MustParse("128Mi"),
                                },
                                Requests: corev1.ResourceList{
                                    corev1.ResourceCPU:    resource.MustParse("250m"),
                                    corev1.ResourceMemory: resource.MustParse("64Mi"),
                                },
                            },
                        },
                    },
                },
            },
        },
    }
}

// needsUpdateSkupperRouterDeployment determines if a Deployment needs to be updated based on the SkupperRouter instance
func (r *AC3NetworkReconciler) needsUpdateSkupperRouterDeployment(deployment *appsv1.Deployment, instance SkupperRouter) bool {
    if deployment.Spec.Template.Spec.Containers[0].Image != "quay.io/skupper/skupper-router:latest" {
        return true
    }
    return false
}

// SetupWithManager sets up the controller with the Manager.
func (r *AC3NetworkReconciler) SetupWithManager(mgr ctrl.Manager) error {
    return ctrl.NewControllerManagedBy(mgr).
        For(&ac3v1alpha1.AC3Network{}).
        Complete(r)
}
