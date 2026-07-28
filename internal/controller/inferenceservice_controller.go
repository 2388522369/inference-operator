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
	"fmt"
	"net/http"
	"time"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	aiv1 "github.com/2388522369/inference-operator/api/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const inferenceServiceFinalizer = "ai.ai.example.com/finalizer"

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

	// ========== 检查 CR 是否被标记为删除  ==========
	if infService.DeletionTimestamp != nil {
		//如果CR上有我们的Finalizer，才需要清理
		if controllerutil.ContainsFinalizer(&infService, inferenceServiceFinalizer) {
			//执行清理逻辑
			if err := r.finnalizeInferenceService(ctx, &infService); err != nil {
				log.Error(err, "Failed to finalize InferenceService")
				return ctrl.Result{}, err
			}

			//清理成功，移除Finalizer
			controllerutil.RemoveFinalizer(&infService, inferenceServiceFinalizer)
			if err := r.Update(ctx, &infService); err != nil {
				return ctrl.Result{}, err
			}
			log.Info("Finalizer removed, CR will be deleted")
		}
		//停止后续Reconclie，让API Server 完成删除
		return ctrl.Result{}, nil
	}

	//如果CR没有被删除，确保Finalizer已添加
	if !controllerutil.ContainsFinalizer(&infService, inferenceServiceFinalizer) {
		controllerutil.AddFinalizer(&infService, inferenceServiceFinalizer)
		if err := r.Update(ctx, &infService); err != nil {
			return ctrl.Result{}, err
		}
		log.Info("Finalizer added")
		//返回后立即触发下一次Reconclie，避免资源还没创建
		return ctrl.Result{}, nil
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
		// 创建分支的最后，重新获取Deployment以拿到最新Status
		if err := r.Get(ctx, types.NamespacedName{Name: infService.Name, Namespace: infService.Namespace}, &deploy); err != nil {
			return ctrl.Result{}, err
		}
		log.Info("Deployment created sucessfully")

	} else if err == nil {
		desiredDeploy := r.buildDeployment(&infService)

		if *deploy.Spec.Replicas != *desiredDeploy.Spec.Replicas {
			log.Info("Replicas changed,updating Deployment", "current", *deploy.Spec.Replicas, "desired", *desiredDeploy.Spec.Replicas)

			deploy.Spec.Replicas = desiredDeploy.Spec.Replicas
			if err := r.Update(ctx, &deploy); err != nil {
				log.Error(err, "Failed to update Deployment")
				return ctrl.Result{}, err
			}
		}
		// 更新分支的最后，重新获取Deployment以拿到最新Status
		if err := r.Get(ctx, types.NamespacedName{Name: infService.Name, Namespace: infService.Namespace}, &deploy); err != nil {
			return ctrl.Result{}, err
		}
	} else {
		log.Error(err, "Failed to get Deployment")
		return ctrl.Result{}, err
	}

	// ========== 管理 Service ==========
	var svc corev1.Service
	err = r.Get(ctx, types.NamespacedName{Name: infService.Name, Namespace: infService.Namespace}, &svc)

	if err != nil && errors.IsNotFound(err) {
		// Service 不存在，创建
		desiredSvc := r.buildService(&infService)
		if err := r.Create(ctx, desiredSvc); err != nil {
			log.Error(err, "Failed to create Service")
			return ctrl.Result{}, err
		}
	} else if err == nil {
		// Service 已存在，此处可以简单检查是否需要更新（暂时省略）
	} else {
		log.Error(err, "Failed to get Service")
		return ctrl.Result{}, err
	}

	// ========== 执行健康检查 ==========
	// if deploy.Status.ReadyReplicas > 0 {
	// 	healthy := r.checkHealth(&infService)
	// 	infService.Status.Healthy = healthy
	// 	if !healthy {
	// 		log.Info("Health check failed")
	// 		infService.Status.Phase = "Unhealthy"
	// 	}
	// }

	infService.Status.ReadyReplicas = deploy.Status.ReadyReplicas
	if deploy.Status.ReadyReplicas > 0 {
		//healthy := r.checkHealth(&infService)
		//infService.Status.Healthy = healthy
		//if healthy {
		// 	infService.Status.Phase = "Running"
		// } else {
		// 	infService.Status.Phase = "Unhealthy"
		// }
		infService.Status.Phase = "Running"
		infService.Status.Healthy = true // 假设健康
		log.Info("Setting Phase to Running")
	} else {
		infService.Status.Phase = "Pending"
		infService.Status.Healthy = false
		log.Info("Setting Phase to Pending")
	}
	if err := r.Status().Update(ctx, &infService); err != nil {
		log.Error(err, "Failed to update status")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *InferenceServiceReconciler) finnalizeInferenceService(ctx context.Context, inf *aiv1.InferenceService) error {
	log := log.FromContext(ctx)

	//清理Deployment
	var deploy appsv1.Deployment
	err := r.Get(ctx, types.NamespacedName{Name: inf.Name, Namespace: inf.Namespace}, &deploy)
	if err == nil {
		log.Info("Deleting Deployment", "name", deploy.Name)
		if err := r.Delete(ctx, &deploy); err != nil {
			return err
		}
	} else if !errors.IsNotFound(err) {
		return err
	}

	//清理Service
	var svc corev1.Service
	err = r.Get(ctx, types.NamespacedName{Name: inf.Name, Namespace: inf.Namespace}, &svc)
	if err == nil {
		log.Info("Deleting Service", "name", svc.Name)
		if err := r.Delete(ctx, &svc); err != nil {
			return err
		}
	} else if !errors.IsNotFound(err) {
		return err
	}

	log.Info("All sub-resources cleaned up")
	return nil
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

func (r *InferenceServiceReconciler) buildService(inf *aiv1.InferenceService) *corev1.Service {
	labels := map[string]string{
		"app":       "inference",
		"modelName": inf.Spec.ModelName,
	}

	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      inf.Name,
			Namespace: inf.Namespace,
		},
		Spec: corev1.ServiceSpec{
			Selector: labels,
			Ports: []corev1.ServicePort{
				{
					Name:       "http",
					Protocol:   corev1.ProtocolTCP,
					Port:       8000,
					TargetPort: intstr.FromInt(8000),
				},
			},
		},
	}

	_ = controllerutil.SetControllerReference(inf, svc, r.Scheme)

	return svc
}

func (r *InferenceServiceReconciler) checkHealth(inf *aiv1.InferenceService) bool {
	//构造Service的访问地址：<ServiceName>.<Namespace>.svc.cluster.local
	url := fmt.Sprintf("http://%s.%s.svc.cluster.local:8000/health", inf.Name, inf.Namespace)

	client := &http.Client{
		Timeout: 5 * time.Second,
	}

	resp, err := client.Get(url)
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	return resp.StatusCode == 200
}

// SetupWithManager sets up the controller with the Manager.
func (r *InferenceServiceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&aiv1.InferenceService{}).
		Complete(r)
}
