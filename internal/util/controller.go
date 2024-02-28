package util

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	fauliv1alpha1 "github.com/fauli/fauli-operator/api/v1alpha1"
)

// MutateFn is a function which mutates the existing object into it's desired state.
type MutateFn func() error

func CreateOrUpdate(_ context.Context, c client.Client, scheme *runtime.Scheme, log logr.Logger, obj client.Object, owner client.Object, mutate func() error) error {

	ownerVersion := ""
	if owner != nil {
		controllerutil.SetControllerReference(owner, obj, scheme)
		ownerVersion = owner.GetResourceVersion()
	}

	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		result, err := createOrUpdate(context.TODO(), c, obj, mutate)
		if err != nil {
			return err
		}

		var gvk schema.GroupVersionKind
		gvk, err = apiutil.GVKForObject(obj, scheme)
		if err == nil {
			log.Info("Reconciled", "Kind", gvk.Kind, "Namespace", obj.GetNamespace(), "Name", obj.GetName(), "Owner", ownerVersion, "Version", obj.GetResourceVersion(), "Status", result)
		}

		return nil
	})

	return err
}

func createOrUpdate(ctx context.Context, c client.Client, obj client.Object, f MutateFn) (controllerutil.OperationResult, error) {
	key := client.ObjectKeyFromObject(obj)
	if err := c.Get(ctx, key, obj); err != nil {
		if !errors.IsNotFound(err) {
			return controllerutil.OperationResultNone, err
		}
		if err := mutate(f, key, obj); err != nil {
			return controllerutil.OperationResultNone, err
		}
		if err := c.Create(ctx, obj); err != nil {
			return controllerutil.OperationResultNone, err
		}
		return controllerutil.OperationResultCreated, nil
	}

	existing := obj.DeepCopyObject() //nolint
	if err := mutate(f, key, obj); err != nil {
		return controllerutil.OperationResultNone, err
	}

	if equality.Semantic.DeepEqual(existing, obj) {
		return controllerutil.OperationResultNone, nil
	}

	if err := c.Update(ctx, obj); err != nil {
		return controllerutil.OperationResultNone, err
	}
	return controllerutil.OperationResultUpdated, nil
}

func mutate(f MutateFn, key client.ObjectKey, obj client.Object) error {
	if err := f(); err != nil {
		return err
	}
	if newKey := client.ObjectKeyFromObject(obj); key != newKey {
		return fmt.Errorf("MutateFn cannot mutate object name and/or object namespace")
	}
	return nil
}

func DeploymentForFapp(deploy *appsv1.Deployment, fapp *fauliv1alpha1.Fapp) {

	labels := labelsForFapp(fapp.Name)
	// print all labels
	for key, value := range labels {
		fmt.Println("Key:", key, "Value:", value)
	}
	deploy.ObjectMeta.Labels = labels
	deploy.Spec.Selector = &metav1.LabelSelector{
		MatchLabels: labels,
	}

	deploy.Spec.Replicas = &fapp.Spec.Instances
}

func PodSpecForFapp(pts *corev1.PodTemplateSpec, fapp *fauliv1alpha1.Fapp, ns *corev1.Namespace) error {

	var appContainer corev1.Container
	pts.ObjectMeta.Labels = labelsForFapp(fapp.Name)

	if len(pts.Spec.Containers) == 0 {
		appContainer = corev1.Container{}
	} else {
		appContainer = pts.Spec.Containers[0]
	}

	appContainer.Name = fmt.Sprintf("%s-%s", fapp.Name, "container")
	appContainer.Image = fapp.Spec.Image
	appContainer.ImagePullPolicy = "Always"

	appContainer.SecurityContext = &corev1.SecurityContext{
		RunAsUser:                &[]int64{1001}[0],
		AllowPrivilegeEscalation: &[]bool{false}[0],
		// Set the securtity context to use the runtime default seccomp profile
		SeccompProfile: &corev1.SeccompProfile{
			Type: corev1.SeccompProfileTypeRuntimeDefault,
		},
	}

	appContainer.Ports = []corev1.ContainerPort{
		{
			ContainerPort: 8080,
			Name:          fapp.Name,
			Protocol:      "TCP",
		},
	}
	pts.Spec.Containers = []corev1.Container{appContainer}
	return nil

}

func labelsForFapp(name string) map[string]string {
	return map[string]string{
		"app.kubernetes.io/name":       name,
		"app.kubernetes.io/managed-by": "Fauli-Operator",
		"app.kubernetes.io/part-of":    "Fauli-Application",
		"app.kubernetes.io/created-by": "controller-manager",
		"managed-by":                   "sloth-operator",
	}
}
