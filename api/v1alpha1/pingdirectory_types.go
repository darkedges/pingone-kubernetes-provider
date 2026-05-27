package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// PingDirectoryStatus describes the observed state of a PingDirectory resource.
type PingDirectoryStatus struct {
	// Phase is the current lifecycle phase: Pending, Ready, or Failed.
	Phase string `json:"phase,omitempty"`
	// Release is the Helm release name that manages this product.
	Release string `json:"release,omitempty"`
	// Conditions holds the latest available observations of the resource's state.
	Conditions []metav1.Condition `json:"conditions,omitempty"`
	// ObservedGeneration is the generation last processed by the reconciler.
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Environment",type="string",JSONPath=".spec.environmentRef"
// +kubebuilder:printcolumn:name="Phase",type="string",JSONPath=".status.phase"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// PingDirectory is the Schema for the pingdirectories API.
type PingDirectory struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              PingDirectorySpec   `json:"spec,omitempty"`
	Status            PingDirectoryStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// PingDirectoryList contains a list of PingDirectory resources.
type PingDirectoryList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []PingDirectory `json:"items"`
}

func init() {
	SchemeBuilder.Register(&PingDirectory{}, &PingDirectoryList{})
}
