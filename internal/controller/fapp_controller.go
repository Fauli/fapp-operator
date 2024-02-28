/*
Copyright 2024.

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
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	fauliv1alpha1 "github.com/fauli/fauli-operator/api/v1alpha1"
	"github.com/fauli/fauli-operator/internal/util"
	"github.com/go-logr/logr"
)

// FappReconciler reconciles a Fapp object
type FappReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Log      logr.Logger
	Recorder record.EventRecorder
}

const (
	fappFinalizer = "fapp.sbebe.ch/finalizer"

	typeAvailableFapp = "Available"
	typeDegradedFapp  = "Degraded"
)

//+kubebuilder:rbac:groups=fauli.sbebe.ch,resources=fapps,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=fauli.sbebe.ch,resources=fapps/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=fauli.sbebe.ch,resources=fapps/finalizers,verbs=update
//+kubebuilder:rbac:groups=core,resources=events,verbs=create;patch
//+kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=pods,verbs=get;list;watch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Fapp object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.15.0/pkg/reconcile
func (r *FappReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)
	// log.Info("GOT SOMETHING")

	// Fetch the Fapp instance
	fapp := &fauliv1alpha1.Fapp{}
	err := r.Get(ctx, req.NamespacedName, fapp)

	if err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("Object has probably been deleted? Ignoring for now...")
			return ctrl.Result{}, nil
		}
		log.Error(err, "Cannot fetch the FAPP!")
		return ctrl.Result{}, err
	}

	// fetch namespace for later use
	ns := &corev1.Namespace{}
	key := types.NamespacedName{
		Name: req.Namespace,
	}
	if err := r.Client.Get(ctx, key, ns); err != nil {
		if !errors.IsNotFound(err) {
			return ctrl.Result{}, err
		}
	}

	// Set the Fapp instance metadata for later use
	objectMeta := metav1.ObjectMeta{
		Name:      fapp.Name,
		Namespace: req.Namespace,
	}

	// Let's just set the status as Unknown when no status are available
	if fapp.Status.Conditions == nil || len(fapp.Status.Conditions) == 0 {
		meta.SetStatusCondition(&fapp.Status.Conditions, metav1.Condition{
			Type:    typeAvailableFapp,
			Status:  metav1.ConditionUnknown,
			Reason:  "Reconciling",
			Message: "Starting reconciliation",
		})

		if err = r.Status().Update(ctx, fapp); err != nil {
			log.Error(err, "Failed to update Fapp status")
			return ctrl.Result{}, err
		}

		// Let's re-fetch the fapp Custom Resource after update the status
		// so that we have the latest state of the resource on the cluster and we will avoid
		// raise the issue "the object has been modified, please apply
		// your changes to the latest version and try again" which would re-trigger the reconciliation
		// if we try to update it again in the following operations
		if err := r.Get(ctx, req.NamespacedName, fapp); err != nil {
			log.Error(err, "Failed to re-fetch fapp")
			return ctrl.Result{}, err
		}

	}

	// Let's add a finalizer. Then, we can define some operations which should
	// occurs before the custom resource to be deleted.
	// More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/finalizers
	if !controllerutil.ContainsFinalizer(fapp, fappFinalizer) {
		// Let's re-fetch the fapp Custom Resource after update the status
		// so that we have the latest state of the resource on the cluster and we will avoid
		// raise the issue "the object has been modified, please apply
		// your changes to the latest version and try again" which would re-trigger the reconciliation
		// if we try to update it again in the following operations
		if err := r.Get(ctx, req.NamespacedName, fapp); err != nil {
			log.Error(err, "Failed to re-fetch fapp")
			return ctrl.Result{}, err
		}
		log.Info("Adding Finalizer for our Fapp")
		if ok := controllerutil.AddFinalizer(fapp, fappFinalizer); !ok {
			log.Error(err, "Failed to add finalizer into the custom resource")
			return ctrl.Result{Requeue: true}, nil
		}

		if err = r.Update(ctx, fapp); err != nil {
			log.Error(err, "Failed to update custom resource to add finalizer")
			return ctrl.Result{}, err
		}
	}

	// Check if the Fapp instance is marked to be deleted, which is
	// indicated by the deletion timestamp being set.
	isFappMarkedToBeDeleted := fapp.GetDeletionTimestamp() != nil
	if isFappMarkedToBeDeleted {
		if controllerutil.ContainsFinalizer(fapp, fappFinalizer) {
			log.Info("DELETE THE FAPP!")

			log.Info("Performing Finalizer Operations for Sloth-App before delete.....")

			// Let's add here an status "Downgrade" to define that this resource begin its process to be terminated.
			meta.SetStatusCondition(&fapp.Status.Conditions, metav1.Condition{
				Type:    typeDegradedFapp,
				Status:  metav1.ConditionUnknown,
				Reason:  "Finalizing",
				Message: fmt.Sprintf("Performing finalizer operations for the custom resource: %s ", fapp.Name)})

			if err := r.Status().Update(ctx, fapp); err != nil {
				log.Error(err, "Failed to update Fapp status")
				return ctrl.Result{}, err
			}

			// Perform all operations required before remove the finalizer and allow
			// the Kubernetes API to remove the custom resource.
			r.doFinalizerOperationsForFapp(fapp)

			// TODO(user): If you add operations to the doFinalizerOperationsForFapp method
			// then you need to ensure that all worked fine before deleting and updating the Downgrade status
			// otherwise, you should requeue here.

			// Re-fetch the fapp Custom Resource before update the status
			// so that we have the latest state of the resource on the cluster and we will avoid
			// raise the issue "the object has been modified, please apply
			// your changes to the latest version and try again" which would re-trigger the reconciliation
			if err := r.Get(ctx, req.NamespacedName, fapp); err != nil {
				log.Error(err, "Failed to re-fetch fapp")
				return ctrl.Result{}, err
			}
			meta.SetStatusCondition(&fapp.Status.Conditions, metav1.Condition{
				Type:   typeDegradedFapp,
				Status: metav1.ConditionTrue, Reason: "Finalizing",
				Message: fmt.Sprintf("Finalizer operations for custom resource %s name were successfully accomplished", fapp.Name)})

			if err := r.Status().Update(ctx, fapp); err != nil {
				log.Error(err, "Failed to update fapp status")
				return ctrl.Result{}, err
			}

			log.Info("Removing Finalizer for fapp after successfully perform the operations")
			if ok := controllerutil.RemoveFinalizer(fapp, fappFinalizer); !ok {
				log.Error(err, "Failed to remove finalizer for fapp")
				return ctrl.Result{Requeue: true}, nil
			}

			if err := r.Update(ctx, fapp); err != nil {
				log.Error(err, "Failed to remove finalizer for fapp")
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	//////// Deployment ////////
	// create or update the deployment resource
	dpl := &appsv1.Deployment{}
	dpl.ObjectMeta = objectMeta
	err = util.CreateOrUpdate(ctx, r.Client, r.Scheme, r.Log, dpl, fapp, func() error {

		util.DeploymentForFapp(dpl, fapp)
		return util.PodSpecForFapp(&dpl.Spec.Template, fapp, ns)
	})
	if err != nil {
		log.Error(err, "Deployment handling failed")
	}
	// TODO: Handle the status of the deployment appropiately

	// Let's re-fetch the fapp Custom Resource after update the status
	// so that we have the latest state of the resource on the cluster and we will avoid
	// raise the issue "the object has been modified, please apply
	// your changes to the latest version and try again" which would re-trigger the reconciliation
	// if we try to update it again in the following operations
	if err := r.Get(ctx, req.NamespacedName, fapp); err != nil {
		log.Error(err, "Failed to re-fetch fapp")
		return ctrl.Result{}, err
	}

	//////// Service ////////
	// Check if the service already exists, if not create a new one
	foundService := &corev1.Service{}
	err = r.Get(ctx, types.NamespacedName{Name: fapp.Name, Namespace: fapp.Namespace}, foundService)

	if err != nil && apierrors.IsNotFound(err) {
		log.Info("Will now create a service")
		// Define a new service
		svc, err := r.serviceForFapp(fapp)
		if err != nil {
			log.Error(err, "Failed to define new Service resource for fapp")

			// The following implementation will update the status
			meta.SetStatusCondition(&fapp.Status.Conditions, metav1.Condition{Type: typeAvailableFapp,
				Status: metav1.ConditionFalse, Reason: "Reconciling",
				Message: fmt.Sprintf("Failed to create Deployment for the custom resource (%s): (%s)", fapp.Name, err)})

			if err := r.Status().Update(ctx, fapp); err != nil {
				log.Error(err, "Failed to update fapp status")
				return ctrl.Result{}, err
			}

			return ctrl.Result{}, err
		}

		log.Info("Creating a new Deployment",
			"Deployment.Namespace", svc.Namespace, "Deployment.Name", svc.Name)
		if err = r.Create(ctx, svc); err != nil {
			log.Error(err, "Failed to create new Deployment",
				"Deployment.Namespace", svc.Namespace, "Deployment.Name", svc.Name)
			return ctrl.Result{}, err
		}

		// Service created successfully
		// We will requeue the reconciliation so that we can ensure the state
		// and move forward for the next operations
		return ctrl.Result{RequeueAfter: time.Minute}, nil
	} else if err != nil {
		log.Error(err, "Failed to get Service")
		// Let's return the error for the reconciliation be re-trigged again
		return ctrl.Result{}, err
	}

	// Let's re-fetch the fapp Custom Resource after update the status
	// so that we have the latest state of the resource on the cluster and we will avoid
	// raise the issue "the object has been modified, please apply
	// your changes to the latest version and try again" which would re-trigger the reconciliation
	// if we try to update it again in the following operations
	if err := r.Get(ctx, req.NamespacedName, fapp); err != nil {
		log.Error(err, "Failed to re-fetch fapp")
		return ctrl.Result{}, err
	}

	//////// Ingress ////////
	// Check if the ingress already exists, if not create a new one, but only if isExposed is true
	if fapp.Spec.IsExposed {
		foundIngress := &networkingv1.Ingress{}
		err = r.Get(ctx, types.NamespacedName{Name: fapp.Name, Namespace: fapp.Namespace}, foundIngress)

		if err != nil && apierrors.IsNotFound(err) {
			log.Info("Will now create an ingress")
			// Define a new service
			ingress, err := r.ingressForFapp(fapp)
			if err != nil {
				log.Error(err, "Failed to define new Service resource for fapp")

				// The following implementation will update the status
				meta.SetStatusCondition(&fapp.Status.Conditions, metav1.Condition{Type: typeAvailableFapp,
					Status: metav1.ConditionFalse, Reason: "Reconciling",
					Message: fmt.Sprintf("Failed to create Deployment for the custom resource (%s): (%s)", fapp.Name, err)})

				if err := r.Status().Update(ctx, fapp); err != nil {
					log.Error(err, "Failed to update fapp status")
					return ctrl.Result{}, err
				}

				return ctrl.Result{}, err
			}

			log.Info("Creating a new Deployment",
				"Deployment.Namespace", ingress.Namespace, "Deployment.Name", ingress.Name)
			if err = r.Create(ctx, ingress); err != nil {
				log.Error(err, "Failed to create new Deployment",
					"Deployment.Namespace", ingress.Namespace, "Deployment.Name", ingress.Name)
				return ctrl.Result{}, err
			}

			// Service created successfully
			// We will requeue the reconciliation so that we can ensure the state
			// and move forward for the next operations
			return ctrl.Result{RequeueAfter: time.Minute}, nil
		} else if err != nil {
			log.Error(err, "Failed to get Service")
			// Let's return the error for the reconciliation be re-trigged again
			return ctrl.Result{}, err
		}
	} else {
		// If the ingress is not exposed, we should check if it exists and delete it
		foundIngress := &networkingv1.Ingress{}
		err = r.Get(ctx, types.NamespacedName{Name: fapp.Name, Namespace: fapp.Namespace}, foundIngress)

		if err != nil && apierrors.IsNotFound(err) {
			log.Info("Ingress not found, nothing to do")
		} else if err != nil {
			log.Error(err, "Failed to get Ingress")
			return ctrl.Result{}, err
		} else {
			log.Info("Deleting Ingress")
			err = r.Delete(ctx, foundIngress)
			if err != nil {
				log.Error(err, "Failed to delete Ingress")
				return ctrl.Result{}, err
			}
		}
	}

	// The following implementation will update the status
	meta.SetStatusCondition(&fapp.Status.Conditions, metav1.Condition{Type: typeAvailableFapp,
		Status: metav1.ConditionTrue, Reason: "Reconciling",
		Message: fmt.Sprintf("Deployment for custom resource (%s) with %d replicas created successfully", fapp.Name, fapp.Spec.Instances)})

	if err := r.Status().Update(ctx, fapp); err != nil {
		log.Error(err, "Failed to update fapp status")
		return ctrl.Result{}, err
	}

	r.Recorder.Event(fapp, "Normal", "Created",
		fmt.Sprintf("The sloth has created %s in namespace %s",
			fapp.Name,
			fapp.Namespace))

	return ctrl.Result{}, nil
}

func (r *FappReconciler) doFinalizerOperationsForFapp(fapp *fauliv1alpha1.Fapp) {
	// TODO(user): Add the cleanup steps that the operator
	// needs to do before the CR can be deleted. Examples
	// of finalizers include performing backups and deleting
	// resources that are not owned by this CR, like a PVC.

	// Note: It is not recommended to use finalizers with the purpose of delete resources which are
	// created and managed in the reconciliation. These ones, such as the Deployment created on this reconcile,
	// are defined as depended of the custom resource. See that we use the method ctrl.SetControllerReference.
	// to set the ownerRef which means that the Deployment will be deleted by the Kubernetes API.
	// More info: https://kubernetes.io/docs/tasks/administer-cluster/use-cascading-deletion/

	// The following implementation will raise an event
	r.Recorder.Event(fapp, "Warning", "Deleting",
		fmt.Sprintf("The sloth has deleted %s in namespace %s",
			fapp.Name,
			fapp.Namespace))
}

func (r *FappReconciler) serviceForFapp(fapp *fauliv1alpha1.Fapp) (*corev1.Service, error) {
	labels := labelsForFapp(fapp.Name)

	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fapp.Name,
			Namespace: fapp.Namespace,
			Labels:    labels,
		},
		Spec: corev1.ServiceSpec{
			Selector: labels,
			Ports: []corev1.ServicePort{{
				Port:       fapp.Spec.Port,
				TargetPort: intstr.FromInt(int(fapp.Spec.Port))}},
		},
	}

	// Set the ownerRef for the Service
	// More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/owners-dependents/
	if err := ctrl.SetControllerReference(fapp, service, r.Scheme); err != nil {
		return nil, err
	}

	return service, nil
}

func (r *FappReconciler) ingressForFapp(fapp *fauliv1alpha1.Fapp) (*networkingv1.Ingress, error) {
	labels := labelsForFapp(fapp.Name)
	ingress := &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fapp.Name,
			Namespace: fapp.Namespace,
			Labels:    labels,
			Annotations: map[string]string{
				"nginx.ingress.kubernetes.io/rewrite-target": "/",
			},
		},
		Spec: networkingv1.IngressSpec{
			Rules: []networkingv1.IngressRule{{
				Host: "fapp.sbebe.ch",
				IngressRuleValue: networkingv1.IngressRuleValue{
					HTTP: &networkingv1.HTTPIngressRuleValue{
						Paths: []networkingv1.HTTPIngressPath{{
							Path: "/",
							PathType: func() *networkingv1.PathType {
								pt := networkingv1.PathTypePrefix
								return &pt
							}(),
							Backend: networkingv1.IngressBackend{
								Service: &networkingv1.IngressServiceBackend{
									Name: fapp.Name,
									Port: networkingv1.ServiceBackendPort{
										Number: fapp.Spec.Port,
									},
								},
							},
						}},
					},
				},
			}},
		},
	}

	return ingress, nil

}

func labelsForFapp(name string) map[string]string {
	return map[string]string{
		"app.kubernetes.io/name":       name,
		"app.kubernetes.io/managed-by": "Fauli-Operator",
		"app.kubernetes.io/part-of":    "Fauli-Application",
		"app.kubernetes.io/created-by": "controller-manager",
	}
}

// SetupWithManager sets up the controller with the Manager.
func (r *FappReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&fauliv1alpha1.Fapp{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Service{}).
		Owns(&networkingv1.Ingress{}).
		Complete(r)
}
