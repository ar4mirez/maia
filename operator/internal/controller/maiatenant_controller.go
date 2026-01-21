package controller

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	maiav1alpha1 "github.com/ar4mirez/maia/operator/api/v1alpha1"
	"github.com/ar4mirez/maia/operator/pkg/maia"
)

const (
	maiaTenantFinalizer = "maia.cuemby.com/tenant-finalizer"
)

// MaiaTenantReconciler reconciles a MaiaTenant object.
type MaiaTenantReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
}

// +kubebuilder:rbac:groups=maia.cuemby.com,resources=maiatenants,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=maia.cuemby.com,resources=maiatenants/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=maia.cuemby.com,resources=maiatenants/finalizers,verbs=update
// +kubebuilder:rbac:groups=maia.cuemby.com,resources=maiainstances,verbs=get;list;watch
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch;create;update;patch
// +kubebuilder:rbac:groups=core,resources=events,verbs=create;patch

// Reconcile is part of the main kubernetes reconciliation loop.
func (r *MaiaTenantReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Fetch the MaiaTenant
	tenant := &maiav1alpha1.MaiaTenant{}
	if err := r.Get(ctx, req.NamespacedName, tenant); err != nil {
		if errors.IsNotFound(err) {
			logger.Info("MaiaTenant resource not found, ignoring since object must be deleted")
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Failed to get MaiaTenant")
		return ctrl.Result{}, err
	}

	// Fetch the referenced MaiaInstance
	instance, err := r.getReferencedInstance(ctx, tenant)
	if err != nil {
		logger.Error(err, "Failed to get referenced MaiaInstance")
		r.setCondition(ctx, tenant, maiav1alpha1.TenantConditionTypeReady, metav1.ConditionFalse,
			"InstanceNotFound", err.Error())
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	// Check if instance is ready
	if instance.Status.Phase != maiav1alpha1.InstancePhaseRunning {
		logger.Info("MaiaInstance is not ready", "phase", instance.Status.Phase)
		r.setCondition(ctx, tenant, maiav1alpha1.TenantConditionTypeReady, metav1.ConditionFalse,
			"InstanceNotReady", "MaiaInstance is not in Running phase")
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	// Get MAIA API client
	maiaClient, err := r.getMAIAClient(ctx, instance)
	if err != nil {
		logger.Error(err, "Failed to create MAIA client")
		r.setCondition(ctx, tenant, maiav1alpha1.TenantConditionTypeReady, metav1.ConditionFalse,
			"ClientError", err.Error())
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	// Handle deletion
	if tenant.GetDeletionTimestamp() != nil {
		if controllerutil.ContainsFinalizer(tenant, maiaTenantFinalizer) {
			if err := r.finalizeTenant(ctx, tenant, maiaClient); err != nil {
				logger.Error(err, "Failed to finalize tenant")
				return ctrl.Result{}, err
			}
			controllerutil.RemoveFinalizer(tenant, maiaTenantFinalizer)
			if err := r.Update(ctx, tenant); err != nil {
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	// Add finalizer if not present
	if !controllerutil.ContainsFinalizer(tenant, maiaTenantFinalizer) {
		controllerutil.AddFinalizer(tenant, maiaTenantFinalizer)
		if err := r.Update(ctx, tenant); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Set initial status
	if tenant.Status.Phase == "" {
		tenant.Status.Phase = maiav1alpha1.TenantPhasePending
		if err := r.Status().Update(ctx, tenant); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Reconcile tenant in MAIA
	if err := r.reconcileTenant(ctx, tenant, maiaClient); err != nil {
		logger.Error(err, "Failed to reconcile tenant")
		r.setCondition(ctx, tenant, maiav1alpha1.TenantConditionTypeSynced, metav1.ConditionFalse,
			"SyncFailed", err.Error())
		tenant.Status.Phase = maiav1alpha1.TenantPhaseFailed
		if statusErr := r.Status().Update(ctx, tenant); statusErr != nil {
			logger.Error(statusErr, "Failed to update status")
		}
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	// Handle suspension
	if tenant.Spec.Suspended {
		if err := r.handleSuspension(ctx, tenant, maiaClient); err != nil {
			logger.Error(err, "Failed to handle suspension")
			return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
		}
	} else if tenant.Status.Phase == maiav1alpha1.TenantPhaseSuspended {
		if err := r.handleActivation(ctx, tenant, maiaClient); err != nil {
			logger.Error(err, "Failed to handle activation")
			return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
		}
	}

	// Reconcile API keys
	if err := r.reconcileAPIKeys(ctx, tenant, maiaClient); err != nil {
		logger.Error(err, "Failed to reconcile API keys")
		// Don't fail the reconciliation for API key issues
	}

	// Update status with usage
	if err := r.updateStatus(ctx, tenant, maiaClient); err != nil {
		logger.Error(err, "Failed to update status")
		return ctrl.Result{}, err
	}

	// Requeue after 60 seconds to sync status
	return ctrl.Result{RequeueAfter: 60 * time.Second}, nil
}

func (r *MaiaTenantReconciler) getReferencedInstance(ctx context.Context, tenant *maiav1alpha1.MaiaTenant) (*maiav1alpha1.MaiaInstance, error) {
	namespace := tenant.Namespace
	if tenant.Spec.InstanceRef.Namespace != "" {
		namespace = tenant.Spec.InstanceRef.Namespace
	}

	instance := &maiav1alpha1.MaiaInstance{}
	if err := r.Get(ctx, types.NamespacedName{
		Name:      tenant.Spec.InstanceRef.Name,
		Namespace: namespace,
	}, instance); err != nil {
		return nil, err
	}

	return instance, nil
}

func (r *MaiaTenantReconciler) getMAIAClient(ctx context.Context, instance *maiav1alpha1.MaiaInstance) (*maia.Client, error) {
	// Get the endpoint from instance status
	endpoint := instance.Status.Endpoint
	if endpoint == "" {
		return nil, fmt.Errorf("MaiaInstance endpoint not available")
	}

	// Get API key from secret if configured
	var apiKey string
	if instance.Spec.Security.APIKeySecretRef != nil {
		secret := &corev1.Secret{}
		if err := r.Get(ctx, types.NamespacedName{
			Name:      instance.Spec.Security.APIKeySecretRef.Name,
			Namespace: instance.Namespace,
		}, secret); err != nil {
			return nil, fmt.Errorf("failed to get API key secret: %w", err)
		}

		key := "api-key"
		if instance.Spec.Security.APIKeySecretRef.Key != "" {
			key = instance.Spec.Security.APIKeySecretRef.Key
		}

		apiKey = string(secret.Data[key])
	}

	return maia.NewClient(endpoint, maia.WithAPIKey(apiKey)), nil
}

func (r *MaiaTenantReconciler) finalizeTenant(ctx context.Context, tenant *maiav1alpha1.MaiaTenant, client *maia.Client) error {
	logger := log.FromContext(ctx)
	logger.Info("Finalizing MaiaTenant", "tenantId", tenant.Status.TenantID)

	if tenant.Status.TenantID != "" {
		if err := client.DeleteTenant(ctx, tenant.Status.TenantID); err != nil {
			// Log but don't fail if tenant doesn't exist
			logger.Error(err, "Failed to delete tenant from MAIA", "tenantId", tenant.Status.TenantID)
		}
	}

	r.Recorder.Event(tenant, corev1.EventTypeNormal, "Deleted", "Tenant deleted from MAIA")
	return nil
}

func (r *MaiaTenantReconciler) reconcileTenant(ctx context.Context, tenant *maiav1alpha1.MaiaTenant, client *maia.Client) error {
	logger := log.FromContext(ctx)

	// Check if tenant exists
	var existingTenant *maia.Tenant
	if tenant.Status.TenantID != "" {
		var err error
		existingTenant, err = client.GetTenant(ctx, tenant.Status.TenantID)
		if err != nil {
			return fmt.Errorf("failed to get tenant: %w", err)
		}
	}

	if existingTenant == nil {
		// Create tenant
		logger.Info("Creating tenant in MAIA")
		createReq := &maia.CreateTenantRequest{
			Name:   tenant.Name,
			Plan:   r.mapPlan(tenant.Spec.Plan),
			Config: r.buildTenantConfig(tenant),
			Quotas: r.buildTenantQuotas(tenant),
			Metadata: map[string]any{
				"kubernetes.namespace": tenant.Namespace,
				"kubernetes.name":      tenant.Name,
			},
		}

		created, err := client.CreateTenant(ctx, createReq)
		if err != nil {
			return fmt.Errorf("failed to create tenant: %w", err)
		}

		tenant.Status.TenantID = created.ID
		tenant.Status.CreatedAt = &metav1.Time{Time: created.CreatedAt}
		r.Recorder.Event(tenant, corev1.EventTypeNormal, "Created", "Tenant created in MAIA")
	} else {
		// Update tenant if needed
		logger.Info("Updating tenant in MAIA", "tenantId", tenant.Status.TenantID)
		updateReq := &maia.UpdateTenantRequest{
			Plan:   r.mapPlan(tenant.Spec.Plan),
			Config: r.buildTenantConfig(tenant),
			Quotas: r.buildTenantQuotas(tenant),
		}

		if _, err := client.UpdateTenant(ctx, tenant.Status.TenantID, updateReq); err != nil {
			return fmt.Errorf("failed to update tenant: %w", err)
		}
		r.Recorder.Event(tenant, corev1.EventTypeNormal, "Updated", "Tenant updated in MAIA")
	}

	r.setCondition(ctx, tenant, maiav1alpha1.TenantConditionTypeSynced, metav1.ConditionTrue,
		"Synced", "Tenant is synced with MAIA")

	return nil
}

func (r *MaiaTenantReconciler) handleSuspension(ctx context.Context, tenant *maiav1alpha1.MaiaTenant, client *maia.Client) error {
	if tenant.Status.Phase == maiav1alpha1.TenantPhaseSuspended {
		return nil // Already suspended
	}

	reason := tenant.Spec.SuspendReason
	if reason == "" {
		reason = "Suspended by operator"
	}

	if err := client.SuspendTenant(ctx, tenant.Status.TenantID, reason); err != nil {
		return fmt.Errorf("failed to suspend tenant: %w", err)
	}

	tenant.Status.Phase = maiav1alpha1.TenantPhaseSuspended
	r.setCondition(ctx, tenant, maiav1alpha1.TenantConditionTypeSuspended, metav1.ConditionTrue,
		"Suspended", reason)
	r.Recorder.Event(tenant, corev1.EventTypeWarning, "Suspended", reason)

	return nil
}

func (r *MaiaTenantReconciler) handleActivation(ctx context.Context, tenant *maiav1alpha1.MaiaTenant, client *maia.Client) error {
	if err := client.ActivateTenant(ctx, tenant.Status.TenantID); err != nil {
		return fmt.Errorf("failed to activate tenant: %w", err)
	}

	tenant.Status.Phase = maiav1alpha1.TenantPhaseActive
	r.setCondition(ctx, tenant, maiav1alpha1.TenantConditionTypeSuspended, metav1.ConditionFalse,
		"Activated", "Tenant has been activated")
	r.Recorder.Event(tenant, corev1.EventTypeNormal, "Activated", "Tenant has been activated")

	return nil
}

func (r *MaiaTenantReconciler) reconcileAPIKeys(ctx context.Context, tenant *maiav1alpha1.MaiaTenant, maiaClient *maia.Client) error {
	logger := log.FromContext(ctx)

	if tenant.Status.TenantID == "" {
		return nil
	}

	// Get existing API keys
	existingKeys, err := maiaClient.ListAPIKeys(ctx, tenant.Status.TenantID)
	if err != nil {
		return fmt.Errorf("failed to list API keys: %w", err)
	}

	existingKeyNames := make(map[string]bool)
	for _, key := range existingKeys {
		existingKeyNames[key.Name] = true
	}

	// Create missing API keys
	for _, keySpec := range tenant.Spec.APIKeys {
		if existingKeyNames[keySpec.Name] {
			continue // Key already exists
		}

		logger.Info("Creating API key", "name", keySpec.Name)

		createReq := &maia.CreateAPIKeyRequest{
			Name:   keySpec.Name,
			Scopes: keySpec.Scopes,
		}
		if keySpec.ExpiresAt != nil {
			createReq.ExpiresAt = &keySpec.ExpiresAt.Time
		}

		resp, err := maiaClient.CreateAPIKey(ctx, tenant.Status.TenantID, createReq)
		if err != nil {
			logger.Error(err, "Failed to create API key", "name", keySpec.Name)
			continue
		}

		// Store the API key in the referenced secret
		if err := r.storeAPIKeyInSecret(ctx, tenant, keySpec, resp.Key); err != nil {
			logger.Error(err, "Failed to store API key in secret", "name", keySpec.Name)
		}
	}

	// Update API key count
	tenant.Status.APIKeyCount = len(tenant.Spec.APIKeys)

	return nil
}

func (r *MaiaTenantReconciler) storeAPIKeyInSecret(ctx context.Context, tenant *maiav1alpha1.MaiaTenant, keySpec maiav1alpha1.TenantAPIKey, rawKey string) error {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      keySpec.SecretRef.Name,
			Namespace: tenant.Namespace,
		},
	}

	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, secret, func() error {
		if secret.Data == nil {
			secret.Data = make(map[string][]byte)
		}

		key := "api-key"
		if keySpec.SecretRef.Key != "" {
			key = keySpec.SecretRef.Key
		}

		secret.Data[key] = []byte(rawKey)
		return controllerutil.SetControllerReference(tenant, secret, r.Scheme)
	})

	return err
}

func (r *MaiaTenantReconciler) updateStatus(ctx context.Context, tenant *maiav1alpha1.MaiaTenant, client *maia.Client) error {
	if tenant.Status.TenantID == "" {
		return nil
	}

	// Get tenant usage
	usage, err := client.GetTenantUsage(ctx, tenant.Status.TenantID)
	if err != nil {
		return fmt.Errorf("failed to get tenant usage: %w", err)
	}

	tenant.Status.MemoryCount = usage.MemoryCount
	tenant.Status.NamespaceCount = usage.NamespaceCount
	tenant.Status.StorageUsed = usage.StorageBytes

	// Calculate quota usage percentages
	if tenant.Spec.Quotas.MaxMemories > 0 {
		tenant.Status.QuotaUsage.Memories = float64(usage.MemoryCount) / float64(tenant.Spec.Quotas.MaxMemories) * 100
	}
	if tenant.Spec.Quotas.MaxStorageBytes > 0 {
		tenant.Status.QuotaUsage.Storage = float64(usage.StorageBytes) / float64(tenant.Spec.Quotas.MaxStorageBytes) * 100
	}

	// Set phase based on state
	if tenant.Spec.Suspended {
		tenant.Status.Phase = maiav1alpha1.TenantPhaseSuspended
	} else if tenant.Status.TenantID != "" {
		tenant.Status.Phase = maiav1alpha1.TenantPhaseActive
		r.setCondition(ctx, tenant, maiav1alpha1.TenantConditionTypeReady, metav1.ConditionTrue,
			"Ready", "Tenant is active and ready")
	}

	tenant.Status.LastActivity = &metav1.Time{Time: time.Now()}

	return r.Status().Update(ctx, tenant)
}

func (r *MaiaTenantReconciler) setCondition(ctx context.Context, tenant *maiav1alpha1.MaiaTenant,
	conditionType string, status metav1.ConditionStatus, reason, message string) {
	meta.SetStatusCondition(&tenant.Status.Conditions, metav1.Condition{
		Type:               conditionType,
		Status:             status,
		Reason:             reason,
		Message:            message,
		ObservedGeneration: tenant.Generation,
	})
}

func (r *MaiaTenantReconciler) mapPlan(plan string) string {
	switch plan {
	case maiav1alpha1.PlanFree:
		return "free"
	case maiav1alpha1.PlanStarter:
		return "standard" // Map starter to standard
	case maiav1alpha1.PlanProfessional:
		return "standard"
	case maiav1alpha1.PlanEnterprise:
		return "premium"
	default:
		return "free"
	}
}

func (r *MaiaTenantReconciler) buildTenantConfig(tenant *maiav1alpha1.MaiaTenant) *maia.TenantConfig {
	config := &maia.TenantConfig{
		DefaultTokenBudget:     tenant.Spec.Config.DefaultTokenBudget,
		MaxTokenBudget:         tenant.Spec.Config.MaxTokenBudget,
		AllowedEmbeddingModels: tenant.Spec.Config.AllowedEmbeddingModels,
	}

	if config.DefaultTokenBudget == 0 {
		config.DefaultTokenBudget = 4000
	}
	if config.MaxTokenBudget == 0 {
		config.MaxTokenBudget = 16000
	}

	return config
}

func (r *MaiaTenantReconciler) buildTenantQuotas(tenant *maiav1alpha1.MaiaTenant) *maia.TenantQuotas {
	return &maia.TenantQuotas{
		MaxMemories:       tenant.Spec.Quotas.MaxMemories,
		MaxStorageBytes:   tenant.Spec.Quotas.MaxStorageBytes,
		MaxNamespaces:     tenant.Spec.Quotas.MaxNamespaces,
		RequestsPerMinute: tenant.Spec.Quotas.RequestsPerMinute,
		RequestsPerDay:    tenant.Spec.Quotas.RequestsPerDay,
	}
}

// SetupWithManager sets up the controller with the Manager.
func (r *MaiaTenantReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&maiav1alpha1.MaiaTenant{}).
		Owns(&corev1.Secret{}).
		Named("maiatenant").
		Complete(r)
}
