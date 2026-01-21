package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// MaiaTenantSpec defines the desired state of MaiaTenant.
type MaiaTenantSpec struct {
	// InstanceRef is the reference to the MaiaInstance this tenant belongs to.
	// +kubebuilder:validation:Required
	InstanceRef InstanceReference `json:"instanceRef"`

	// DisplayName is a human-readable display name.
	// +optional
	DisplayName string `json:"displayName,omitempty"`

	// Plan is the tenant plan level.
	// +kubebuilder:validation:Enum=free;starter;professional;enterprise
	// +kubebuilder:default="free"
	// +optional
	Plan string `json:"plan,omitempty"`

	// Quotas are the resource quotas for the tenant.
	// +optional
	Quotas TenantQuotas `json:"quotas,omitempty"`

	// Config is the tenant-specific configuration.
	// +optional
	Config TenantConfig `json:"config,omitempty"`

	// APIKeys are the API keys for this tenant.
	// +optional
	APIKeys []TenantAPIKey `json:"apiKeys,omitempty"`

	// Suspended indicates whether the tenant is suspended.
	// +kubebuilder:default=false
	// +optional
	Suspended bool `json:"suspended,omitempty"`

	// SuspendReason is the reason for suspension.
	// +optional
	SuspendReason string `json:"suspendReason,omitempty"`
}

// InstanceReference is a reference to a MaiaInstance.
type InstanceReference struct {
	// Name is the name of the MaiaInstance.
	// +kubebuilder:validation:Required
	Name string `json:"name"`

	// Namespace is the namespace of the MaiaInstance.
	// Defaults to the same namespace as the MaiaTenant.
	// +optional
	Namespace string `json:"namespace,omitempty"`
}

// TenantQuotas defines the resource quotas for a tenant.
type TenantQuotas struct {
	// MaxMemories is the maximum number of memories (0 = unlimited).
	// +kubebuilder:validation:Minimum=0
	// +optional
	MaxMemories int64 `json:"maxMemories,omitempty"`

	// MaxStorageBytes is the maximum storage in bytes (0 = unlimited).
	// +kubebuilder:validation:Minimum=0
	// +optional
	MaxStorageBytes int64 `json:"maxStorageBytes,omitempty"`

	// MaxNamespaces is the maximum number of namespaces.
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:default=10
	// +optional
	MaxNamespaces int `json:"maxNamespaces,omitempty"`

	// RequestsPerMinute is the rate limit in requests per minute (0 = unlimited).
	// +kubebuilder:validation:Minimum=0
	// +optional
	RequestsPerMinute int `json:"requestsPerMinute,omitempty"`

	// RequestsPerDay is the rate limit in requests per day (0 = unlimited).
	// +kubebuilder:validation:Minimum=0
	// +optional
	RequestsPerDay int64 `json:"requestsPerDay,omitempty"`
}

// TenantConfig defines tenant-specific configuration.
type TenantConfig struct {
	// DefaultTokenBudget is the default token budget.
	// +kubebuilder:default=4000
	// +optional
	DefaultTokenBudget int `json:"defaultTokenBudget,omitempty"`

	// MaxTokenBudget is the maximum token budget.
	// +kubebuilder:default=16000
	// +optional
	MaxTokenBudget int `json:"maxTokenBudget,omitempty"`

	// AllowedEmbeddingModels are the allowed embedding models.
	// +optional
	AllowedEmbeddingModels []string `json:"allowedEmbeddingModels,omitempty"`

	// Features are the feature flags for this tenant.
	// +optional
	Features TenantFeatures `json:"features,omitempty"`
}

// TenantFeatures defines feature flags for a tenant.
type TenantFeatures struct {
	// Inference enables inference features.
	// +kubebuilder:default=false
	// +optional
	Inference bool `json:"inference,omitempty"`

	// VectorSearch enables vector search.
	// +kubebuilder:default=true
	// +optional
	VectorSearch bool `json:"vectorSearch,omitempty"`

	// FullTextSearch enables full-text search.
	// +kubebuilder:default=true
	// +optional
	FullTextSearch bool `json:"fullTextSearch,omitempty"`

	// ContextAssembly enables context assembly.
	// +kubebuilder:default=true
	// +optional
	ContextAssembly bool `json:"contextAssembly,omitempty"`
}

// TenantAPIKey defines an API key for a tenant.
type TenantAPIKey struct {
	// Name is the API key name.
	// +kubebuilder:validation:Required
	Name string `json:"name"`

	// SecretRef is the reference to a secret containing the API key.
	// +kubebuilder:validation:Required
	SecretRef APIKeySecretRef `json:"secretRef"`

	// Scopes are the API key scopes.
	// +kubebuilder:default={"read","write"}
	// +optional
	Scopes []string `json:"scopes,omitempty"`

	// ExpiresAt is the optional expiration time.
	// +optional
	ExpiresAt *metav1.Time `json:"expiresAt,omitempty"`
}

// APIKeySecretRef is a reference to a secret containing an API key.
type APIKeySecretRef struct {
	// Name is the name of the secret.
	// +kubebuilder:validation:Required
	Name string `json:"name"`

	// Key is the key within the secret.
	// +kubebuilder:default="api-key"
	// +optional
	Key string `json:"key,omitempty"`
}

// MaiaTenantStatus defines the observed state of MaiaTenant.
type MaiaTenantStatus struct {
	// Phase is the current lifecycle phase.
	// +kubebuilder:validation:Enum=Pending;Active;Suspended;Failed
	// +optional
	Phase string `json:"phase,omitempty"`

	// Conditions represent the latest available observations of the MaiaTenant's state.
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// TenantID is the actual tenant ID in MAIA.
	// +optional
	TenantID string `json:"tenantId,omitempty"`

	// MemoryCount is the number of memories.
	// +optional
	MemoryCount int `json:"memoryCount,omitempty"`

	// NamespaceCount is the number of namespaces.
	// +optional
	NamespaceCount int `json:"namespaceCount,omitempty"`

	// StorageUsed is the storage used in bytes.
	// +optional
	StorageUsed int64 `json:"storageUsed,omitempty"`

	// QuotaUsage is the current quota usage percentages.
	// +optional
	QuotaUsage QuotaUsage `json:"quotaUsage,omitempty"`

	// APIKeyCount is the number of API keys.
	// +optional
	APIKeyCount int `json:"apiKeyCount,omitempty"`

	// LastActivity is the timestamp of the last activity.
	// +optional
	LastActivity *metav1.Time `json:"lastActivity,omitempty"`

	// CreatedAt is the creation timestamp.
	// +optional
	CreatedAt *metav1.Time `json:"createdAt,omitempty"`
}

// QuotaUsage defines quota usage percentages.
type QuotaUsage struct {
	// Memories is the memory quota usage percentage (0-100).
	// +optional
	Memories float64 `json:"memories,omitempty"`

	// Storage is the storage quota usage percentage (0-100).
	// +optional
	Storage float64 `json:"storage,omitempty"`

	// RequestsPerMinute is the RPM quota usage percentage (0-100).
	// +optional
	RequestsPerMinute float64 `json:"requestsPerMinute,omitempty"`

	// RequestsPerDay is the RPD quota usage percentage (0-100).
	// +optional
	RequestsPerDay float64 `json:"requestsPerDay,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=mt
// +kubebuilder:printcolumn:name="Instance",type=string,JSONPath=`.spec.instanceRef.name`
// +kubebuilder:printcolumn:name="Status",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Plan",type=string,JSONPath=`.spec.plan`
// +kubebuilder:printcolumn:name="Memories",type=integer,JSONPath=`.status.memoryCount`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// MaiaTenant defines a tenant within a MAIA instance.
type MaiaTenant struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   MaiaTenantSpec   `json:"spec,omitempty"`
	Status MaiaTenantStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// MaiaTenantList contains a list of MaiaTenant.
type MaiaTenantList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []MaiaTenant `json:"items"`
}

func init() {
	SchemeBuilder.Register(&MaiaTenant{}, &MaiaTenantList{})
}

// TenantPhase constants.
const (
	TenantPhasePending   = "Pending"
	TenantPhaseActive    = "Active"
	TenantPhaseSuspended = "Suspended"
	TenantPhaseFailed    = "Failed"
)

// Condition types for MaiaTenant.
const (
	TenantConditionTypeSynced    = "Synced"
	TenantConditionTypeReady     = "Ready"
	TenantConditionTypeSuspended = "Suspended"
)

// API Key scope constants.
const (
	ScopeAll       = "*"
	ScopeRead      = "read"
	ScopeWrite     = "write"
	ScopeDelete    = "delete"
	ScopeAdmin     = "admin"
	ScopeContext   = "context"
	ScopeInference = "inference"
	ScopeSearch    = "search"
	ScopeStats     = "stats"
)

// Plan constants.
const (
	PlanFree         = "free"
	PlanStarter      = "starter"
	PlanProfessional = "professional"
	PlanEnterprise   = "enterprise"
)
