/*
Copyright 2026 2388522369.

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

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	aiv1 "github.com/2388522369/inference-operator/api/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// InferenceServiceReconciler reconciles a InferenceService object
type InferenceServiceReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=ai.ai.example.com,resources=inferenceservices,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=ai.ai.example.com,resources=inferenceservices/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=ai.ai.example.com,resources=inferenceservices/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the InferenceService object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.17.0/pkg/reconcile
func (r *InferenceServiceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	var infService aiv1.InferenceService
	if err := r.Get(ctx, req.NamespacedName, &infService); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	var deploy appsv1.Deployment
	err := r.Get(ctx, types.NamespacedName{Name: infService.Name, Namespace: infService.Namespace}, &deploy)
	if err != nil && errors.IsNotFound(err) {
		log.Info("Deployment not found, creating...")

		desiredDeploy := r.buildDeployment(&infService)
		if err := r.Create(ctx, desiredDeploy); err != nil {
			log.Error(err, "Failed to create Deployment")
			return ctrl.Result{}, err
		}

		log.Info("Deployment created sucessfully")
	} else if err != nil {
		log.Error(err, "Failed to get Deployment")
		return ctrl.Result{}, err
	}

	infService.Status.Phase = "Pending"
	if err := r.Status().Update(ctx, &infService); err != nil {
		log.Error(err, "Failed to update status")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *InferenceServiceReconciler) buildDeployment(inf *aiv1.InferenceService) *appsv1.Deployment {
	labels := map[string]string{
		"app":       "inference",
		"modelName": inf.Spec.ModelName,
	}

	deploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      inf.Name,
			Namespace: inf.Namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &inf.Spec.Replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "inference-server",
							Image: inf.Spec.Image,
						},
					},
				},
			},
		},
	}

	_ = controllerutil.SetControllerReference(inf, deploy, r.Scheme)

	return deploy
}

// SetupWithManager sets up the controller with the Manager.
func (r *InferenceServiceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&aiv1.InferenceService{}).
		Complete(r)
}
