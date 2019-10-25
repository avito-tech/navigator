package v1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// CanaryRelease is a top-level type
type CanaryRelease struct {
	metav1.TypeMeta `json:",inline"`
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec CanaryReleaseSpec `json:"spec,omitempty"`
}

// CanaryRelease spec
type CanaryReleaseSpec struct {
	Backends []Backends `json:"backends"`
}

// Backends defines backends for balancing
type Backends struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	Weight    int    `json:"weight"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// no client needed for list as it's been created in above
type CanaryReleaseList struct {
	metav1.TypeMeta `json:",inline"`
	// +optional
	metav1.ListMeta `son:"metadata,omitempty"`

	Items []CanaryRelease `json:"items"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// Nexus is a top-level type
type Nexus struct {
	metav1.TypeMeta `json:",inline"`
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec NexusSpec `json:"spec,omitempty"`
}

// NexusSpec is a Nexus spec
type NexusSpec struct {
	AppName  string    `json:"appName"`
	Services []Service `json:"services"`
}

// Service matching k8s service entity
type Service struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// no client needed for list as it's been created in above
type NexusList struct {
	metav1.TypeMeta `json:",inline"`
	// +optional
	metav1.ListMeta `son:"metadata,omitempty"`

	Items []Nexus `json:"items"`
}
