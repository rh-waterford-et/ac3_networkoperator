package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type LinkSpec struct {
	SourceCluster   string `json:"sourceCluster"`
	TargetCluster   string `json:"targetCluster"`
	SourceNamespace string `json:"sourceNamespace"`
	TargetNamespace string `json:"targetNamespace"`
	SecretNamespace string `json:"secretNamespace"`
	SecretName      string `json:"secretName"`
	SecretName2     string `json:"secretName2"`
	Port            int    `json:"port"`
}

type LinkStatus struct {
	State   string `json:"state,omitempty"`
	Message string `json:"message,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

type Link struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   LinkSpec   `json:"spec,omitempty"`
	Status LinkStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

type LinkList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Link `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Link{}, &LinkList{})
}
