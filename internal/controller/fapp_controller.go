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

	appsv1 "k8s.io/api/apps/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	fauliv1alpha1 "github.com/fauli/fauli-operator/api/v1alpha1"
)

// FappReconciler reconciles a Fapp object
type FappReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
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
	log.Info("GOT SOMETHING")

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

		// Let's re-fetch the memcached Custom Resource after update the status
		// so that we have the latest state of the resource on the cluster and we will avoid
		// raise the issue "the object has been modified, please apply
		// your changes to the latest version and try again" which would re-trigger the reconciliation
		// if we try to update it again in the following operations
		if err := r.Get(ctx, req.NamespacedName, fapp); err != nil {
			log.Error(err, "Failed to re-fetch memcached")
			return ctrl.Result{}, err
		}

	}

	// Let's add a finalizer. Then, we can define some operations which should
	// occurs before the custom resource to be deleted.
	// More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/finalizers
	if !controllerutil.ContainsFinalizer(fapp, fappFinalizer) {
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

	// Check if the Memcached instance is marked to be deleted, which is
	// indicated by the deletion timestamp being set.
	isFappMarkedToBeDeleted := fapp.GetDeletionTimestamp() != nil
	if isFappMarkedToBeDeleted {
		if controllerutil.ContainsFinalizer(fapp, fappFinalizer) {
			log.Info("DELETE THE FAPP!")

			// HERE WE WOULD DO THE ACTUAL DELETION OF THE ITEMS!!!

			meta.SetStatusCondition(&fapp.Status.Conditions, metav1.Condition{Type: typeDegradedFapp,
				Status: metav1.ConditionTrue, Reason: "Finalizing",
				Message: fmt.Sprintf("Finalizer operations for custom resource %s name were successfully accomplished", fapp.Name)})

			if err := r.Status().Update(ctx, fapp); err != nil {
				log.Error(err, "Failed to update Memcached status")
				return ctrl.Result{}, err
			}

			log.Info("Removing Finalizer for Memcached after successfully perform the operations")
			if ok := controllerutil.RemoveFinalizer(fapp, fappFinalizer); !ok {
				log.Error(err, "Failed to remove finalizer for Memcached")
				return ctrl.Result{Requeue: true}, nil
			}

			if err := r.Update(ctx, fapp); err != nil {
				log.Error(err, "Failed to remove finalizer for Memcached")
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	// Check if the deployment already exists, if not create a new one
	found := &appsv1.Deployment{}
	err = r.Get(ctx, types.NamespacedName{Name: fapp.Name, Namespace: fapp.Namespace}, found)

	if err != nil && apierrors.IsNotFound(err) {
		log.Info("Would now create a deployment")
		// r.Recorder.Event(fapp, "Information", "Because_I_Can", fmt.Sprintf("Custom Resource %s is being deleted from the namespace %s",
		// 	fapp.Name,
		// 	fapp.Namespace))
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *FappReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&fauliv1alpha1.Fapp{}).
		Complete(r)
}
