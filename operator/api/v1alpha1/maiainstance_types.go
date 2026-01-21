package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// MaiaInstanceSpec defines the desired state of MaiaInstance.
type MaiaInstanceSpec struct {
	// Replicas is the number of MAIA replicas (1-10).
	// Stateless mode requires external storage.
	// +kubebuilder:default=1
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=10
	// +optional
	Replicas *int32 `json:"replicas,omitempty"`

	// Image is the container image configuration.
	// +optional
	Image ImageSpec `json:"image,omitempty"`

	// Storage is the storage configuration.
	// +optional
	Storage StorageSpec `json:"storage,omitempty"`

	// Security is the security configuration.
	// +optional
	Security SecuritySpec `json:"security,omitempty"`

	// Embedding is the embedding model configuration.
	// +optional
	Embedding EmbeddingSpec `json:"embedding,omitempty"`

	// Tenancy is the multi-tenancy configuration.
	// +optional
	Tenancy TenancySpec `json:"tenancy,omitempty"`

	// RateLimit is the rate limiting configuration.
	// +optional
	RateLimit RateLimitSpec `json:"rateLimit,omitempty"`

	// Logging is the logging configuration.
	// +optional
	Logging LoggingSpec `json:"logging,omitempty"`

	// Metrics is the metrics configuration.
	// +optional
	Metrics MetricsSpec `json:"metrics,omitempty"`

	// Ingress is the ingress configuration.
	// +optional
	Ingress IngressSpec `json:"ingress,omitempty"`

	// Resources are the resource requirements.
	// +optional
	Resources ResourcesSpec `json:"resources,omitempty"`

	// Backup is the backup configuration.
	// +optional
	Backup BackupSpec `json:"backup,omitempty"`
}

// ImageSpec defines the container image configuration.
type ImageSpec struct {
	// Repository is the container image repository.
	// +kubebuilder:default="ghcr.io/ar4mirez/maia"
	// +optional
	Repository string `json:"repository,omitempty"`

	// Tag is the container image tag. If not specified, defaults to "latest".
	// It is recommended to always specify a semantic version (e.g., "v1.0.0").
	// +optional
	Tag string `json:"tag,omitempty"`

	// PullPolicy is the image pull policy.
	// +kubebuilder:validation:Enum=Always;IfNotPresent;Never
	// +kubebuilder:default="IfNotPresent"
	// +optional
	PullPolicy corev1.PullPolicy `json:"pullPolicy,omitempty"`
}

// StorageSpec defines the storage configuration.
type StorageSpec struct {
	// Size is the storage size (e.g., "10Gi").
	// +kubebuilder:default="10Gi"
	// +kubebuilder:validation:Pattern=`^[0-9]+(Gi|Mi|Ti)$`
	// +optional
	Size string `json:"size,omitempty"`

	// StorageClassName is the Kubernetes storage class name.
	// +optional
	StorageClassName *string `json:"storageClassName,omitempty"`

	// DataDir is the path for data storage.
	// +kubebuilder:default="/data"
	// +optional
	DataDir string `json:"dataDir,omitempty"`

	// SyncWrites enables synchronous writes.
	// +kubebuilder:default=false
	// +optional
	SyncWrites bool `json:"syncWrites,omitempty"`

	// GCInterval is the garbage collection interval.
	// +kubebuilder:default="5m"
	// +optional
	GCInterval string `json:"gcInterval,omitempty"`
}

// SecuritySpec defines the security configuration.
type SecuritySpec struct {
	// APIKeySecretRef is the reference to a secret containing the API key.
	// +optional
	APIKeySecretRef *SecretKeySelector `json:"apiKeySecretRef,omitempty"`

	// TLSSecretRef is the reference to a TLS secret for HTTPS.
	// +optional
	TLSSecretRef *corev1.LocalObjectReference `json:"tlsSecretRef,omitempty"`
}

// SecretKeySelector selects a key from a Secret.
type SecretKeySelector struct {
	// Name is the name of the secret.
	Name string `json:"name"`

	// Key is the key within the secret.
	// +kubebuilder:default="api-key"
	// +optional
	Key string `json:"key,omitempty"`
}

// EmbeddingSpec defines the embedding model configuration.
type EmbeddingSpec struct {
	// Model is the embedding model to use.
	// +kubebuilder:validation:Enum=local;openai;ollama
	// +kubebuilder:default="local"
	// +optional
	Model string `json:"model,omitempty"`

	// OpenAISecretRef is the reference to a secret containing the OpenAI API key.
	// +optional
	OpenAISecretRef *SecretKeySelector `json:"openaiSecretRef,omitempty"`

	// OllamaEndpoint is the Ollama API endpoint URL.
	// +optional
	OllamaEndpoint string `json:"ollamaEndpoint,omitempty"`
}

// TenancySpec defines the multi-tenancy configuration.
type TenancySpec struct {
	// Enabled enables multi-tenancy.
	// +kubebuilder:default=false
	// +optional
	Enabled bool `json:"enabled,omitempty"`

	// RequireTenant requires a tenant for all requests.
	// +kubebuilder:default=false
	// +optional
	RequireTenant bool `json:"requireTenant,omitempty"`

	// DefaultTenantID is the default tenant ID.
	// +kubebuilder:default="default"
	// +optional
	DefaultTenantID string `json:"defaultTenantId,omitempty"`

	// EnforceScopesEnabled enables scope enforcement.
	// +kubebuilder:default=false
	// +optional
	EnforceScopesEnabled bool `json:"enforceScopesEnabled,omitempty"`

	// DedicatedStorage uses dedicated storage directories per tenant.
	// +kubebuilder:default=false
	// +optional
	DedicatedStorage bool `json:"dedicatedStorage,omitempty"`
}

// RateLimitSpec defines the rate limiting configuration.
type RateLimitSpec struct {
	// Enabled enables rate limiting.
	// +kubebuilder:default=false
	// +optional
	Enabled bool `json:"enabled,omitempty"`

	// RequestsPerSecond is the rate limit in requests per second.
	// +kubebuilder:default=100
	// +optional
	RequestsPerSecond int `json:"requestsPerSecond,omitempty"`

	// Burst is the burst capacity.
	// +kubebuilder:default=200
	// +optional
	Burst int `json:"burst,omitempty"`
}

// LoggingSpec defines the logging configuration.
type LoggingSpec struct {
	// Level is the logging level.
	// +kubebuilder:validation:Enum=debug;info;warn;error
	// +kubebuilder:default="info"
	// +optional
	Level string `json:"level,omitempty"`

	// Format is the logging format.
	// +kubebuilder:validation:Enum=json;text
	// +kubebuilder:default="json"
	// +optional
	Format string `json:"format,omitempty"`
}

// MetricsSpec defines the metrics configuration.
type MetricsSpec struct {
	// Enabled enables metrics collection.
	// +kubebuilder:default=true
	// +optional
	Enabled bool `json:"enabled,omitempty"`

	// ServiceMonitor is the Prometheus ServiceMonitor configuration.
	// +optional
	ServiceMonitor ServiceMonitorSpec `json:"serviceMonitor,omitempty"`
}

// ServiceMonitorSpec defines the Prometheus ServiceMonitor configuration.
type ServiceMonitorSpec struct {
	// Enabled creates a ServiceMonitor resource.
	// +kubebuilder:default=false
	// +optional
	Enabled bool `json:"enabled,omitempty"`

	// Interval is the scrape interval.
	// +kubebuilder:default="30s"
	// +optional
	Interval string `json:"interval,omitempty"`

	// Labels are additional labels for the ServiceMonitor.
	// +optional
	Labels map[string]string `json:"labels,omitempty"`
}

// IngressSpec defines the ingress configuration.
type IngressSpec struct {
	// Enabled creates an Ingress resource.
	// +kubebuilder:default=false
	// +optional
	Enabled bool `json:"enabled,omitempty"`

	// ClassName is the ingress class name.
	// +optional
	ClassName *string `json:"className,omitempty"`

	// Host is the ingress hostname.
	// +optional
	Host string `json:"host,omitempty"`

	// Annotations are additional annotations for the Ingress.
	// +optional
	Annotations map[string]string `json:"annotations,omitempty"`

	// TLS enables TLS for the ingress.
	// +kubebuilder:default=false
	// +optional
	TLS bool `json:"tls,omitempty"`
}

// ResourcesSpec defines the resource requirements.
type ResourcesSpec struct {
	// Limits are the resource limits.
	// +optional
	Limits ResourceQuantities `json:"limits,omitempty"`

	// Requests are the resource requests.
	// +optional
	Requests ResourceQuantities `json:"requests,omitempty"`
}

// ResourceQuantities defines CPU and memory quantities.
type ResourceQuantities struct {
	// CPU is the CPU quantity (e.g., "1000m").
	// +optional
	CPU string `json:"cpu,omitempty"`

	// Memory is the memory quantity (e.g., "1Gi").
	// +optional
	Memory string `json:"memory,omitempty"`
}

// BackupSpec defines the backup configuration.
type BackupSpec struct {
	// Enabled enables backups.
	// +kubebuilder:default=false
	// +optional
	Enabled bool `json:"enabled,omitempty"`

	// Schedule is the cron schedule for backups.
	// +kubebuilder:default="0 2 * * *"
	// +optional
	Schedule string `json:"schedule,omitempty"`

	// RetentionDays is the backup retention period in days.
	// +kubebuilder:default=30
	// +optional
	RetentionDays int `json:"retentionDays,omitempty"`

	// StorageSize is the backup storage size.
	// +kubebuilder:default="20Gi"
	// +optional
	StorageSize string `json:"storageSize,omitempty"`

	// Compress enables backup compression.
	// +kubebuilder:default=true
	// +optional
	Compress bool `json:"compress,omitempty"`
}

// MaiaInstanceStatus defines the observed state of MaiaInstance.
type MaiaInstanceStatus struct {
	// Phase is the current lifecycle phase.
	// +kubebuilder:validation:Enum=Pending;Running;Failed;Updating
	// +optional
	Phase string `json:"phase,omitempty"`

	// Conditions represent the latest available observations of the MaiaInstance's state.
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// Replicas is the current number of replicas.
	// +optional
	Replicas int32 `json:"replicas,omitempty"`

	// ReadyReplicas is the number of ready replicas.
	// +optional
	ReadyReplicas int32 `json:"readyReplicas,omitempty"`

	// TenantCount is the number of active tenants.
	// +optional
	TenantCount int `json:"tenantCount,omitempty"`

	// TotalMemories is the total number of memories stored.
	// +optional
	TotalMemories int `json:"totalMemories,omitempty"`

	// StorageUsed is the storage used (bytes as string).
	// +optional
	StorageUsed string `json:"storageUsed,omitempty"`

	// LastBackup is the timestamp of the last backup.
	// +optional
	LastBackup *metav1.Time `json:"lastBackup,omitempty"`

	// Version is the current MAIA version.
	// +optional
	Version string `json:"version,omitempty"`

	// Endpoint is the service endpoint URL.
	// +optional
	Endpoint string `json:"endpoint,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=maia;mi
// +kubebuilder:printcolumn:name="Status",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Tenants",type=integer,JSONPath=`.status.tenantCount`
// +kubebuilder:printcolumn:name="Memories",type=integer,JSONPath=`.status.totalMemories`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// MaiaInstance is the Schema for the MAIA memory service.
type MaiaInstance struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   MaiaInstanceSpec   `json:"spec,omitempty"`
	Status MaiaInstanceStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// MaiaInstanceList contains a list of MaiaInstance.
type MaiaInstanceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []MaiaInstance `json:"items"`
}

func init() {
	SchemeBuilder.Register(&MaiaInstance{}, &MaiaInstanceList{})
}

// InstancePhase constants.
const (
	InstancePhasePending  = "Pending"
	InstancePhaseRunning  = "Running"
	InstancePhaseFailed   = "Failed"
	InstancePhaseUpdating = "Updating"
)

// Condition types for MaiaInstance.
const (
	ConditionTypeReady       = "Ready"
	ConditionTypeProgressing = "Progressing"
	ConditionTypeDegraded    = "Degraded"
)
