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

package networking

import (
	"context"
	"fmt"
	"strings"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// IngressReconciler reconciles a Ingress object
type IngressReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=networking.k8s.io,resources=ingresses,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=networking.k8s.io,resources=ingresses/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=networking.k8s.io,resources=ingresses/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Ingress object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.17.0/pkg/reconcile
func (r *IngressReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	ingress := &netv1.Ingress{}
	nameSpace := req.Namespace
	ingressName := req.Name
	deployName := strings.TrimSuffix(ingressName, "-ingress")
	svcName := fmt.Sprintf("%s-svc", deployName)

	config := ctrl.GetConfigOrDie()
	clientSet, err := kubernetes.NewForConfig(config)
	if err != nil {
		return ctrl.Result{}, err
	}

	deploy, err := clientSet.AppsV1().Deployments(nameSpace).Get(ctx, deployName, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			log.Info(fmt.Sprintf("deployment not found : %s", deployName))
			return ctrl.Result{}, nil
		}
		log.Error(err, "getting deployment")
	}
	log.Info(fmt.Sprintf("got deployment %s", deploy.Name))

	svc, err := clientSet.CoreV1().Services(nameSpace).Get(ctx, svcName, metav1.GetOptions{})
	if err != nil {
		log.Error(err, "getting service")
	}
	log.Info(fmt.Sprintf("got service %+v", svc))

	log.Info(fmt.Sprintf("Deployment Name : %s, NameSpace : %s\n", deploy.Name, deploy.Namespace))

	err = r.Client.Get(ctx, req.NamespacedName, ingress, &client.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			// re-create ingress
			log.Info("re-creating ingress")
			err := r.reCreateIngress(ctx, deploy, svc, clientSet, log)
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

func (r *IngressReconciler) reCreateIngress(ctx context.Context, deploy *appsv1.Deployment, svc *corev1.Service, clientSet *kubernetes.Clientset, log logr.Logger) error {

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
					APIVersion: "apps/v1",
					Kind:       "Deployment",
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
		return err
	}
	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *IngressReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&netv1.Ingress{}).
		Complete(r)
}
