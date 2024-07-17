package controller

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

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

func (r *AC3NetworkReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	logger.Info("Starting Reconcile loop", "request", req)

	// Define name and namespace for the ConfigMap
	name := "example-configmap-name" // Replace with your desired ConfigMap name
	namespace := "ac3no" // Replace with your desired namespace
	data := map[string]string{
		"example.key": "example.value", // Add your key-value pairs here
	}

	// Check if the ConfigMap exists
	configMap := &corev1.ConfigMap{}
	err := r.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, configMap)
	if err != nil {
		if client.IgnoreNotFound(err) != nil {
			logger.Error(err, "Failed to get ConfigMap", "name", name, "namespace", namespace)
			return ctrl.Result{}, err
		}

		// If ConfigMap is not found, create it
		configMap = r.createConfigMap(ctx, name, namespace, data)
		if err := r.Create(ctx, configMap); err != nil {
			logger.Error(err, "Failed to create ConfigMap", "name", name, "namespace", namespace)
			return ctrl.Result{}, err
		}
		logger.Info("Created ConfigMap", "name", name, "namespace", namespace)
	} else {
		// If ConfigMap is found, check if it needs to be updated
		if r.needsUpdateConfigMap(configMap, data) {
			configMap.Data = data
			if err := r.Update(ctx, configMap); err != nil {
				logger.Error(err, "Failed to update ConfigMap", "name", name, "namespace", namespace)
				return ctrl.Result{}, err
			}
			logger.Info("Updated ConfigMap", "name", name, "namespace", namespace)
		}
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
		logger.Info("Inspecting SkupperRouter instance", "name", routerInstance.Spec.Name, "namespace", routerInstance.Spec.Namespace)

		// Ensure the namespace is not empty
		if routerInstance.Spec.Namespace == "" {
			logger.Error(fmt.Errorf("namespace cannot be empty"), "Invalid SkupperRouter namespace", "name", routerInstance.Spec.Name)
			continue
		}

		// Check if the SkupperRouter has a corresponding deployment
		deployment := &appsv1.Deployment{}
		err := r.Get(ctx, types.NamespacedName{Name: routerInstance.Spec.Name, Namespace: routerInstance.Spec.Namespace}, deployment)
		if err != nil {
			if client.IgnoreNotFound(err) != nil {
				logger.Error(err, "Failed to get Deployment", "name", routerInstance.Spec.Name, "namespace", routerInstance.Spec.Namespace)
				return ctrl.Result{}, err
			}

			// If deployment is not found, create it
			deployment = r.createDeployment(ctx, &routerInstance)
			if err := r.Create(ctx, deployment); err != nil {
				logger.Error(err, "Failed to create Deployment", "name", routerInstance.Spec.Name, "namespace", routerInstance.Spec.Namespace)
				return ctrl.Result{}, err
			}
			logger.Info("Created Deployment", "name", routerInstance.Spec.Name, "namespace", routerInstance.Spec.Namespace)
		} else {
			// If deployment is found, check if it needs to be updated
			if r.needsUpdate(&routerInstance, deployment) {
				if err := r.Update(ctx, deployment); err != nil {
					logger.Error(err, "Failed to update Deployment", "name", routerInstance.Spec.Name, "namespace", routerInstance.Spec.Namespace)
					return ctrl.Result{}, err
				}
				logger.Info("Updated Deployment", "name", routerInstance.Spec.Name, "namespace", routerInstance.Spec.Namespace)
			}
		}

		// Log that instance was processed
		logger.Info("Processed router instance", "name", routerInstance.Spec.Name, "namespace", routerInstance.Spec.Namespace)
	}

	logger.Info("Finished Reconcile loop")

	return ctrl.Result{}, nil
}

func (r *AC3NetworkReconciler) createDeployment(ctx context.Context, routerInstance *SkupperRouter) *appsv1.Deployment {
	logger := log.FromContext(ctx)

	logger.Info("Creating Deployment", "name", routerInstance.Spec.Name, "namespace", routerInstance.Spec.Namespace)

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      routerInstance.Spec.Name,
			Namespace: routerInstance.Spec.Namespace,  // Ensure the namespace is set here
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": "ac3-network-controller",
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app": "ac3-network-controller",
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "ac3-network-controller",
							Image: "quay.io/ryjenkin/ac3no3:15", // Replace with your actual image
							Ports: []corev1.ContainerPort{
								{
									ContainerPort: 8080, // Example port, change as necessary
								},
							},
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceMemory: resource.MustParse("64Mi"),
									corev1.ResourceCPU:    resource.MustParse("250m"),
								},
								Limits: corev1.ResourceList{
									corev1.ResourceMemory: resource.MustParse("128Mi"),
									corev1.ResourceCPU:    resource.MustParse("500m"),
								},
							},
						},
					},
				},
			},
		},
	}
	controllerutil.SetControllerReference(routerInstance, deployment, r.Scheme)
	return deployment
}

func (r *AC3NetworkReconciler) createConfigMap(ctx context.Context, name string, namespace string, data map[string]string) *corev1.ConfigMap {
	logger := log.FromContext(ctx)

	logger.Info("Creating ConfigMap", "name", name, "namespace", namespace)

	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Data: data,
	}
	return configMap
}

func (r *AC3NetworkReconciler) needsUpdateConfigMap(configMap *corev1.ConfigMap, data map[string]string) bool {
	// Add your logic to check if the ConfigMap needs to be updated
	for key, value := range data {
		if existingValue, exists := configMap.Data[key]; !exists || existingValue != value {
			return true
		}
	}
	return false
}

func (r *AC3NetworkReconciler) needsUpdate(routerInstance *SkupperRouter, deployment *appsv1.Deployment) bool {
	// Add your logic to check if the deployment needs to be updated
	// For example, compare container images, resource requests/limits, etc.
	return false
}

func (r *AC3NetworkReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// Register the SkupperRouter and SkupperRouterList types with the scheme
	scheme := mgr.GetScheme()
	scheme.AddKnownTypes(schema.GroupVersion{Group: "ac3.redhat.com", Version: "v1alpha1"}, &SkupperRouter{}, &SkupperRouterList{})

	return ctrl.NewControllerManagedBy(mgr).
		For(&ac3v1alpha1.AC3Network{}).
		Complete(r)
}
