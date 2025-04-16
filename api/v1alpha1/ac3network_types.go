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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

type AC3Link struct {
	SourceCluster   string   `json:"sourceCluster"`
	TargetCluster   string   `json:"targetCluster"`
	SourceNamespace string   `json:"sourceNamespace"`
	TargetNamespace string   `json:"targetNamespace"`
	Applications    []string `json:"applications,omitempty"`
	Services        []string `json:"services,omitempty"`
	Port            int      `json:"port"`
}

// AC3NetworkSpec defines the desired state of AC3Network
type AC3NetworkSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Foo is an example field of AC3Network. Edit ac3network_types.go to remove/update
	Links []AC3Link `json:"links"`
}

// AC3NetworkStatus defines the observed state of AC3Network
type AC3NetworkStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// AC3Network is the Schema for the ac3networks API
type AC3Network struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AC3NetworkSpec   `json:"spec,omitempty"`
	Status AC3NetworkStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// AC3NetworkList contains a list of AC3Network
type AC3NetworkList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AC3Network `json:"items"`
}

func init() {
	SchemeBuilder.Register(&AC3Network{}, &AC3NetworkList{})
}
