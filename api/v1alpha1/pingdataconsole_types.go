package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// PingDataConsoleStatus describes the observed state of a PingDataConsole resource.
type PingDataConsoleStatus struct {
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

// PingDataConsole is the Schema for the pingdataconsoles API.
type PingDataConsole struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              PingDataConsoleSpec   `json:"spec,omitempty"`
	Status            PingDataConsoleStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// PingDataConsoleList contains a list of PingDataConsole resources.
type PingDataConsoleList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []PingDataConsole `json:"items"`
}

func init() {
	SchemeBuilder.Register(&PingDataConsole{}, &PingDataConsoleList{})
}
