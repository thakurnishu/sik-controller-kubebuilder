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

package apps

import (
	"context"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// DeploymentReconciler reconciles a Deployment object
type DeploymentReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=apps,resources=deployments/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=apps,resources=deployments/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Deployment object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.16.3/pkg/reconcile
func (r *DeploymentReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	deploy := &appsv1.Deployment{}
	service := &corev1.Service{}
	ingress := &netv1.Ingress{}

	nameSpace := req.Namespace

	if systemNamespaces := map[string]bool{
		"kube-node-lease":           true,
		"kube-public":               true,
		"kube-system":               true,
		"local-path-storage":        true,
		"ingress-nginx":             true,
		"coredns":                   true,
		"sik-deployment_controller": true,
	}; systemNamespaces[nameSpace] {
		return ctrl.Result{}, nil
	}

	// Get Deployment
	err := r.Client.Get(ctx, req.NamespacedName, deploy)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		if apierrors.IsAlreadyExists(err) {
			return ctrl.Result{}, nil
		}
	} else {
		// create service
		fmt.Println("Creating Service")
		svc, err := r.createService(deploy, ctx)
		if err != nil {
			if apierrors.IsAlreadyExists(err) {
				return ctrl.Result{}, nil
			}
			log.Error(err, "Failed to create service")
		}

		// create ingress
		fmt.Println("Creating Ingress")
		_, err = r.createIngress(deploy, svc, ctx)
		if err != nil {
			if apierrors.IsAlreadyExists(err) {
				return ctrl.Result{}, nil
			}
			log.Error(err, "Failed to create ingress")
		}
	}

	// Check if Service is deleted
	err = r.Get(ctx, req.NamespacedName, service)
	if err != nil {
		if apierrors.IsNotFound(err) {
			// re-create svc
			fmt.Println("Re-creating Service")
			_, err := r.createService(deploy, ctx)
			if err != nil {
				if apierrors.IsAlreadyExists(err) {
					return ctrl.Result{}, nil
				}
				log.Error(err, "Failed to create service")
			}
		} else {
			log.Error(err, "Failed to get svc")
		}
	}

	// Check if Svc is deleted
	err = r.Get(ctx, req.NamespacedName, ingress)
	if err != nil {
		if apierrors.IsNotFound(err) {
			// re-create ingress
			fmt.Println("Re-creating Ingress")
			_, err := r.createService(deploy, ctx)
			if err != nil {
				if apierrors.IsAlreadyExists(err) {
					return ctrl.Result{}, nil
				}
				log.Error(err, "Failed to create ingress")
			}
		} else {
			log.Error(err, "Failed to get ingress")
		}
	}

	return ctrl.Result{}, nil
}

func (r *DeploymentReconciler) createService(deploy *appsv1.Deployment, ctx context.Context) (*corev1.Service, error) {

	podTemplate := deploy.Spec.Template

	// Get pod ports
	container := podTemplate.Spec.Containers[0]
	portInfo := container.Ports[0]
	isController := true

	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      deploy.Name + "-svc",
			Namespace: deploy.Namespace,
			OwnerReferences: []metav1.OwnerReference{
				{
					Name:       deploy.Name,
					APIVersion: deploy.APIVersion,
					Kind:       deploy.Kind,
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

	err := r.Client.Create(ctx, svc, &client.CreateOptions{})
	if err != nil {
		return nil, err
	}

	return svc, err
}

func (r *DeploymentReconciler) createIngress(deploy *appsv1.Deployment, svc *corev1.Service, ctx context.Context) (*netv1.Ingress, error) {

	isController := true
	ingressClassName := "nginx"
	path := fmt.Sprintf("/%s", deploy.Name)
	pathType := "Prefix"
	svcPort := svc.Spec.Ports[0].Port

	ingress := &netv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      deploy.Name + "-ingress",
			Namespace: deploy.Namespace,
			Annotations: map[string]string{
				"nginx.ingress.kubernetes.io/rewrite-target": "/",
			},
			OwnerReferences: []metav1.OwnerReference{
				{
					Name:       deploy.Name,
					APIVersion: deploy.APIVersion,
					Kind:       deploy.Kind,
					UID:        deploy.UID,
					Controller: &isController,
				},
			},
		},
		Spec: netv1.IngressSpec{
			IngressClassName: &ingressClassName,
			Rules: []netv1.IngressRule{
				{
					IngressRuleValue: netv1.IngressRuleValue{
						HTTP: &netv1.HTTPIngressRuleValue{
							Paths: []netv1.HTTPIngressPath{
								{
									Path:     path,
									PathType: (*netv1.PathType)(&pathType),
									Backend: netv1.IngressBackend{
										Service: &netv1.IngressServiceBackend{
											Name: svc.Name,
											Port: netv1.ServiceBackendPort{
												Number: svcPort,
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	err := r.Client.Create(ctx, ingress, &client.CreateOptions{})
	if err != nil {
		return nil, err
	}

	return ingress, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *DeploymentReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&appsv1.Deployment{}).
		Complete(r)
}
