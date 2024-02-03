/*
Copyright 2024 Nishant Singh.

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

package core

import (
	"context"
	"fmt"
	"strings"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// ServiceReconciler reconciles a Service object
type ServiceReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=services/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=core,resources=services/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Service object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.17.0/pkg/reconcile
func (r *ServiceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	svc := &corev1.Service{}
	nameSpace := req.Namespace
	svcName := req.Name
	deployName := strings.Trim(svcName, "-svc")

	config := ctrl.GetConfigOrDie()
	clientSet, err := kubernetes.NewForConfig(config)
	if err != nil {
		return ctrl.Result{}, err
	}

	deploy, err := clientSet.AppsV1().Deployments(nameSpace).Get(ctx, deployName, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		log.Error(err, "getting deployment")
	}

	log.Info(fmt.Sprintf("Deployment Name : %s, NameSpace : %s\n", deploy.Name, deploy.Namespace))

	err = r.Client.Get(ctx, req.NamespacedName, svc, &client.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			// re-create svc
			log.Info("re-creating svc")
			err := r.reCreateService(ctx, deploy, clientSet, log)
			if err != nil {
				return ctrl.Result{}, err
			}
		}
		if apierrors.IsAlreadyExists(err) {
			return ctrl.Result{}, nil
		}
	}

	return ctrl.Result{}, nil
}

func (r *ServiceReconciler) reCreateService(ctx context.Context, deploy *appsv1.Deployment, clientSet *kubernetes.Clientset, log logr.Logger) error {

	podTemplate := deploy.Spec.Template

	fmt.Println("Inside reCreateService")
	// Get pod ports
	container := podTemplate.Spec.Containers[0]
	portInfo := container.Ports[0]
	isController := true

	log.Info(fmt.Sprintf("Pod Template: %+v", podTemplate))
	log.Info(fmt.Sprintf("Container: %+v", container))
	log.Info(fmt.Sprintf("Port Info: %+v", portInfo))
	log.Info(fmt.Sprintf("Kind: %+v", deploy.Kind))
	log.Info(fmt.Sprintf("API Version: %+v", deploy.APIVersion))

	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      deploy.Name + "-svc",
			Namespace: deploy.Namespace,
			OwnerReferences: []metav1.OwnerReference{
				{
					Name:       deploy.Name,
					APIVersion: "apps/v1",
					Kind:       "Deployment",
					UID:        deploy.UID,
					Controller: &isController,
				},
			},
		},
		Spec: corev1.ServiceSpec{
			Selector: podTemplate.Labels,
			Ports: []corev1.ServicePort{
				{
					Name: podTemplate.Name,
					Port: portInfo.ContainerPort,
				},
			},
		},
	}

	createdService, err := clientSet.CoreV1().Services(deploy.Namespace).Create(ctx, svc, metav1.CreateOptions{})
	if err != nil {
		return err
	}
	log.Info(fmt.Sprintf("Service created successfully: %+v", createdService))

	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ServiceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1.Service{}).
		Complete(r)
}
