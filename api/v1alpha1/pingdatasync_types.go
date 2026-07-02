package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// PingDataSyncStatus describes the observed state of a PingDataSync resource.
type PingDataSyncStatus struct {
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

// PingDataSync is the Schema for the pingdatasyncs API.
type PingDataSync struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              PingDataSyncSpec   `json:"spec,omitempty"`
	Status            PingDataSyncStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// PingDataSyncList contains a list of PingDataSync resources.
type PingDataSyncList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []PingDataSync `json:"items"`
}

func init() {
	SchemeBuilder.Register(&PingDataSync{}, &PingDataSyncList{})
}
