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

type MultiClusterLink struct {
	SourceCluster   string   `json:"sourceCluster"`
	TargetCluster   string   `json:"targetCluster"`
	SourceNamespace string   `json:"sourceNamespace"`
	TargetNamespace string   `json:"targetNamespace"`
	Applications    []string `json:"applications,omitempty"`
	Services        []string `json:"services,omitempty"`
	Port            int      `json:"port"`
}

// MultiClusterNetworkSpec defines the desired state of MultiClusterNetwork
type MultiClusterNetworkSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Links define the multi-cluster network connections
	Links []*MultiClusterLink `json:"links"`
}

// MultiClusterNetworkStatus defines the observed state of MultiClusterNetwork
type MultiClusterNetworkStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// PreviousLinks tracks the previous state of links for cleanup purposes
	PreviousLinks []*MultiClusterLink `json:"previousLinks,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// MultiClusterNetwork is the Schema for the multiclusternetworks API
type MultiClusterNetwork struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   MultiClusterNetworkSpec   `json:"spec,omitempty"`
	Status MultiClusterNetworkStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// MultiClusterNetworkList contains a list of MultiClusterNetwork
type MultiClusterNetworkList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []MultiClusterNetwork `json:"items"`
}

func init() {
	SchemeBuilder.Register(&MultiClusterNetwork{}, &MultiClusterNetworkList{})
}
