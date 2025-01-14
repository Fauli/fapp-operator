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

package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// SlothApplicationSpec defines the desired state of SlothApplication
type SlothApplicationSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// What image should be deployed
	Image string `json:"image"`

	// How many replicas to run
	Replicas int32 `json:"replicas,omitempty"`

	// Do you need an ingress?
	IsExposed bool `json:"isExposed,omitempty"`

	// What port should the ingress listen on?
	Port int32 `json:"port,omitempty"`

	// What resources should be allocated to the pod?
	Resources corev1.ResourceRequirements `json:"resources,omitempty"`
}

// SlothApplicationStatus defines the observed state of SlothApplication
type SlothApplicationStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type" protobuf:"bytes,1,rep,name=conditions"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=slapp;sloth;sapp
// +kubebuilder:printcolumn:JSONPath=".spec.replicas",name=Replicas,type=integer
// +kubebuilder:printcolumn:JSONPath=".spec.image",name=image,type=string
// +kubebuilder:printcolumn:name=age,type=date,JSONPath=`.metadata.creationTimestamp`
// SlothApplication is the Schema for the slothapplications API
type SlothApplication struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SlothApplicationSpec   `json:"spec,omitempty"`
	Status SlothApplicationStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// SlothApplicationList contains a list of SlothApplication
type SlothApplicationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []SlothApplication `json:"items"`
}

func init() {
	SchemeBuilder.Register(&SlothApplication{}, &SlothApplicationList{})
}
