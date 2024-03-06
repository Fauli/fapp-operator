package util

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	policyv1 "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/apimachinery/pkg/util/intstr"

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

	existing := obj.DeepCopyObject()
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
	// // print all labels
	// for key, value := range labels {
	// 	fmt.Println("Key:", key, "Value:", value)
	// }
	deploy.ObjectMeta.Labels = labels
	deploy.Spec.Selector = &metav1.LabelSelector{
		MatchLabels: labels,
	}

	deploy.Spec.Replicas = &fapp.Spec.Replicas
}

func PodSpecForFapp(pts *corev1.PodTemplateSpec, fapp *fauliv1alpha1.Fapp) error {

	var appContainer corev1.Container
	pts.ObjectMeta.Labels = labelsForFapp(fapp.Name)

	if len(pts.Spec.Containers) == 0 {
		appContainer = corev1.Container{}
	} else {
		appContainer = pts.Spec.Containers[0]
	}

	// Set resources for the container
	appContainer.Resources = fapp.Spec.Resources

	// Set the container name and image
	appContainer.Name = fmt.Sprintf("%s-%s", fapp.Name, "container")
	appContainer.Image = fapp.Spec.Image

	// Set the image pull policy to always pull the image
	appContainer.ImagePullPolicy = "Always"

	// Try to be secure :D
	appContainer.SecurityContext = &corev1.SecurityContext{
		RunAsUser:                &[]int64{1001}[0],
		AllowPrivilegeEscalation: &[]bool{false}[0],
		// Set the securtity context to use the runtime default seccomp profile
		SeccompProfile: &corev1.SeccompProfile{
			Type: corev1.SeccompProfileTypeRuntimeDefault,
		},
	}

	// TODO: for nor we only support one container?
	// Set the container ports to what is defined
	appContainer.Ports = []corev1.ContainerPort{
		{
			ContainerPort: fapp.Spec.Port,
			Name:          fapp.Name,
			Protocol:      "TCP",
		},
	}
	pts.Spec.Containers = []corev1.Container{appContainer}
	return nil

}

func ServiceForFapp(svc *corev1.Service, fapp *fauliv1alpha1.Fapp) error {
	labels := labelsForFapp(fapp.Name)
	svc.ObjectMeta.Labels = labels
	svc.Spec.Selector = labels
	svc.Spec.Ports = []corev1.ServicePort{
		{
			Protocol:   "TCP",
			Port:       fapp.Spec.Port,
			TargetPort: intstr.FromInt(int(fapp.Spec.Port)),
		},
	}

	return nil
}

func IngressForFapp(ing *networkingv1.Ingress, fapp *fauliv1alpha1.Fapp) error {
	labels := labelsForFapp(fapp.Name)
	ing.ObjectMeta.Labels = labels
	ing.Spec.Rules = []networkingv1.IngressRule{
		{
			Host: getHostname(fapp),
			IngressRuleValue: networkingv1.IngressRuleValue{
				HTTP: &networkingv1.HTTPIngressRuleValue{
					Paths: []networkingv1.HTTPIngressPath{
						{
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
						},
					},
				},
			},
		},
	}
	return nil
}

func getHostname(fapp *fauliv1alpha1.Fapp) string {
	return fmt.Sprintf("%s.%s", fapp.Name, "sbebe.ch")
}

func PodDisruptionBudgetForFapp(pdb *policyv1.PodDisruptionBudget, fapp *fauliv1alpha1.Fapp) error {

	labels := labelsForFapp(fapp.Name)

	pdb.ObjectMeta.Labels = labels

	pdb.Spec.MaxUnavailable = &intstr.IntOrString{
		Type:   intstr.Int,
		IntVal: getMinHealthyReplicas(fapp.Spec.Replicas),
	}

	pdb.Spec.Selector = &metav1.LabelSelector{
		MatchLabels: labels,
	}

	return nil
}

func getMinHealthyReplicas(replicas int32) int32 {
	return replicas / 2
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
