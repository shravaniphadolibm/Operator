package controller

import (
	"context"
	"fmt"
	"reflect"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	cachev1alpha1 "github.com/example/memcached-operator-new/api/v1alpha1"
)

// MemcachedReconciler reconciles a Memcached object
type MemcachedReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=cache.example.com,resources=memcacheds,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=cache.example.com,resources=memcacheds/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=pods,verbs=get;list;watch

func (r *MemcachedReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Fetch the Memcached object
	memcached := &cachev1alpha1.Memcached{}
	err := r.Get(ctx, req.NamespacedName, memcached)
	if err != nil {
		if errors.IsNotFound(err) {
			logger.Info("Memcached resource not found. Ignoring since object must be deleted")
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Failed to get Memcached", "NamespacedName", req.NamespacedName)
		return ctrl.Result{}, err
	}

	// Check if the deployment already exists, if not create a new one
	deploy := &appsv1.Deployment{}
	err = r.Get(ctx, types.NamespacedName{Name: memcached.Name, Namespace: memcached.Namespace}, deploy)
	if err != nil {
		if errors.IsNotFound(err) {
			// Define a new deployment
			dep := r.deploymentForMemcached(memcached)
			logger.Info("Creating a new Deployment", "Deployment.Namespace", dep.Namespace, "Deployment.Name", dep.Name)
			err = r.Create(ctx, dep)
			if err != nil {
				logger.Error(err, "Failed to create new Deployment", "Deployment.Namespace", dep.Namespace, "Deployment.Name", dep.Name)
				r.updateMemcachedStatus(ctx, memcached, "Failed to create deployment: "+err.Error())
				return ctrl.Result{}, err
			}
			// Deployment created successfully - return and requeue to update the status
			r.updateMemcachedStatus(ctx, memcached, "Deployment created successfully")
			return ctrl.Result{Requeue: true}, nil
		} else {
			logger.Error(err, "Failed to get Deployment")
			r.updateMemcachedStatus(ctx, memcached, "Failed to get deployment: "+err.Error())
			return ctrl.Result{}, err
		}
	}

	// Ensure the deployment size is the same as the spec
	size := memcached.Spec.Size
	if *deploy.Spec.Replicas != size {
		deploy.Spec.Replicas = &size
		err = r.Update(ctx, deploy)
		if err != nil {
			logger.Error(err, "Failed to update Deployment", "Deployment.Namespace", deploy.Namespace, "Deployment.Name", deploy.Name)
			r.updateMemcachedStatus(ctx, memcached, "Failed to update deployment: "+err.Error())
			return ctrl.Result{}, err
		}
		// Spec updated return and requeue
		r.updateMemcachedStatus(ctx, memcached, "Scaling deployment")
		return ctrl.Result{RequeueAfter: time.Minute}, nil // Consider exponential backoff
	}

	// List the pods for this memcached deployment
	podList := &corev1.PodList{}
	listOpts := []client.ListOption{
		client.InNamespace(memcached.Namespace),
		client.MatchingLabels(labelsForMemcached(memcached.Name)),
	}
	if err = r.List(ctx, podList, listOpts...); err != nil {
		logger.Error(err, "Failed to list pods", "Memcached.Namespace", memcached.Namespace, "Memcached.Name", memcached.Name)
		r.updateMemcachedStatus(ctx, memcached, "Failed to list pods: "+err.Error())
		return ctrl.Result{}, err
	}
	podNames := getPodNames(podList.Items)

	// Update status.Nodes if needed
	if !reflect.DeepEqual(podNames, memcached.Status.Nodes) {
		memcached.Status.Nodes = podNames
		err := r.Status().Update(ctx, memcached)
		if err != nil {
			logger.Error(err, "Failed to update Memcached status")
			r.updateMemcachedStatus(ctx, memcached, "Failed to update pod names in status: "+err.Error())
			return ctrl.Result{}, err
		}
		r.updateMemcachedStatus(ctx, memcached, fmt.Sprintf("Nodes updated: %v", podNames))
	}

	return ctrl.Result{}, nil
}

// deploymentForMemcached returns a memcached Deployment object
func (r *MemcachedReconciler) deploymentForMemcached(m *cachev1alpha1.Memcached) *appsv1.Deployment {
	ls := labelsForMemcached(m.Name)
	replicas := m.Spec.Size

	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      m.Name,
			Namespace: m.Namespace,
			Labels:    ls,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: ls,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: ls,
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Image: "memcached:1.4.36-alpine",
						Name:  "memcached",
						Ports: []corev1.ContainerPort{{
							ContainerPort: 11211,
							Name:          "memcached",
						}},
					}},
				},
			},
		},
	}
	// Set Memcached instance as the owner and controller
	ctrl.SetControllerReference(m, dep, r.Scheme)
	return dep
}

// labelsForMemcached returns the labels for selecting the resources
// belonging to the memcached CRD.
func labelsForMemcached(name string) map[string]string {
	return map[string]string{"app": "memcached", "memcached_cr": name}
}

// getPodNames returns the pod names of the array of pods passed in
func getPodNames(pods []corev1.Pod) []string {
	var podNames []string
	for _, pod := range pods {
		podNames = append(podNames, pod.Name)
	}
	return podNames
}

// updateMemcachedStatus updates the status of the Memcached CRD
func (r *MemcachedReconciler) updateMemcachedStatus(ctx context.Context, memcached *cachev1alpha1.Memcached, message string) error {
	logger := log.FromContext(ctx)
	memcached.Status.Message = message // Add a Message field to your CRD status
	err := r.Status().Update(ctx, memcached)
	if err != nil {
		logger.Error(err, "Failed to update Memcached status")
		return err
	}
	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *MemcachedReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&cachev1alpha1.Memcached{}).
		Owns(&appsv1.Deployment{}).
		Complete(r)
}
