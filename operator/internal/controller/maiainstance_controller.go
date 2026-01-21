package controller

import (
	"context"
	"fmt"
	"time"

	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	maiav1alpha1 "github.com/ar4mirez/maia/operator/api/v1alpha1"
)

const (
	maiaInstanceFinalizer = "maia.cuemby.com/finalizer"
	defaultHTTPPort       = 8080
	defaultGRPCPort       = 9090
)

// MaiaInstanceReconciler reconciles a MaiaInstance object.
type MaiaInstanceReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
}

// +kubebuilder:rbac:groups=maia.cuemby.com,resources=maiainstances,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=maia.cuemby.com,resources=maiainstances/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=maia.cuemby.com,resources=maiainstances/finalizers,verbs=update
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=services;configmaps;persistentvolumeclaims;secrets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=networking.k8s.io,resources=ingresses,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=batch,resources=cronjobs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=monitoring.coreos.com,resources=servicemonitors,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=events,verbs=create;patch

// Reconcile is part of the main kubernetes reconciliation loop.
func (r *MaiaInstanceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Fetch the MaiaInstance
	instance := &maiav1alpha1.MaiaInstance{}
	if err := r.Get(ctx, req.NamespacedName, instance); err != nil {
		if errors.IsNotFound(err) {
			logger.Info("MaiaInstance resource not found, ignoring since object must be deleted")
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Failed to get MaiaInstance")
		return ctrl.Result{}, err
	}

	// Handle deletion
	if instance.GetDeletionTimestamp() != nil {
		if controllerutil.ContainsFinalizer(instance, maiaInstanceFinalizer) {
			if err := r.finalizeInstance(ctx, instance); err != nil {
				return ctrl.Result{}, err
			}
			controllerutil.RemoveFinalizer(instance, maiaInstanceFinalizer)
			if err := r.Update(ctx, instance); err != nil {
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	// Add finalizer if not present
	if !controllerutil.ContainsFinalizer(instance, maiaInstanceFinalizer) {
		controllerutil.AddFinalizer(instance, maiaInstanceFinalizer)
		if err := r.Update(ctx, instance); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Set initial status
	if instance.Status.Phase == "" {
		instance.Status.Phase = maiav1alpha1.InstancePhasePending
		if err := r.Status().Update(ctx, instance); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Reconcile ConfigMap
	if err := r.reconcileConfigMap(ctx, instance); err != nil {
		logger.Error(err, "Failed to reconcile ConfigMap")
		r.setCondition(ctx, instance, maiav1alpha1.ConditionTypeDegraded, metav1.ConditionTrue,
			"ConfigMapFailed", err.Error())
		return ctrl.Result{}, err
	}

	// Reconcile PVC
	if err := r.reconcilePVC(ctx, instance); err != nil {
		logger.Error(err, "Failed to reconcile PVC")
		r.setCondition(ctx, instance, maiav1alpha1.ConditionTypeDegraded, metav1.ConditionTrue,
			"PVCFailed", err.Error())
		return ctrl.Result{}, err
	}

	// Reconcile Deployment
	if err := r.reconcileDeployment(ctx, instance); err != nil {
		logger.Error(err, "Failed to reconcile Deployment")
		r.setCondition(ctx, instance, maiav1alpha1.ConditionTypeDegraded, metav1.ConditionTrue,
			"DeploymentFailed", err.Error())
		return ctrl.Result{}, err
	}

	// Reconcile Service
	if err := r.reconcileService(ctx, instance); err != nil {
		logger.Error(err, "Failed to reconcile Service")
		r.setCondition(ctx, instance, maiav1alpha1.ConditionTypeDegraded, metav1.ConditionTrue,
			"ServiceFailed", err.Error())
		return ctrl.Result{}, err
	}

	// Reconcile Ingress (if enabled)
	if instance.Spec.Ingress.Enabled {
		if err := r.reconcileIngress(ctx, instance); err != nil {
			logger.Error(err, "Failed to reconcile Ingress")
			r.setCondition(ctx, instance, maiav1alpha1.ConditionTypeDegraded, metav1.ConditionTrue,
				"IngressFailed", err.Error())
			return ctrl.Result{}, err
		}
	}

	// Reconcile ServiceMonitor (if enabled)
	if instance.Spec.Metrics.Enabled && instance.Spec.Metrics.ServiceMonitor.Enabled {
		if err := r.reconcileServiceMonitor(ctx, instance); err != nil {
			logger.Error(err, "Failed to reconcile ServiceMonitor")
			r.setCondition(ctx, instance, maiav1alpha1.ConditionTypeDegraded, metav1.ConditionTrue,
				"ServiceMonitorFailed", err.Error())
			return ctrl.Result{}, err
		}
	}

	// Reconcile Backup CronJob (if enabled)
	if instance.Spec.Backup.Enabled {
		if err := r.reconcileBackupPVC(ctx, instance); err != nil {
			logger.Error(err, "Failed to reconcile Backup PVC")
			r.setCondition(ctx, instance, maiav1alpha1.ConditionTypeDegraded, metav1.ConditionTrue,
				"BackupPVCFailed", err.Error())
			return ctrl.Result{}, err
		}
		if err := r.reconcileBackupCronJob(ctx, instance); err != nil {
			logger.Error(err, "Failed to reconcile Backup CronJob")
			r.setCondition(ctx, instance, maiav1alpha1.ConditionTypeDegraded, metav1.ConditionTrue,
				"BackupCronJobFailed", err.Error())
			return ctrl.Result{}, err
		}
	}

	// Update status
	if err := r.updateStatus(ctx, instance); err != nil {
		logger.Error(err, "Failed to update status")
		return ctrl.Result{}, err
	}

	// Requeue after 30 seconds to sync status
	return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
}

func (r *MaiaInstanceReconciler) finalizeInstance(ctx context.Context, instance *maiav1alpha1.MaiaInstance) error {
	logger := log.FromContext(ctx)
	logger.Info("Finalizing MaiaInstance")
	r.Recorder.Event(instance, corev1.EventTypeNormal, "Finalizing", "Cleaning up resources")
	return nil
}

func (r *MaiaInstanceReconciler) reconcileConfigMap(ctx context.Context, instance *maiav1alpha1.MaiaInstance) error {
	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      instance.Name + "-config",
			Namespace: instance.Namespace,
		},
	}

	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, configMap, func() error {
		configMap.Data = map[string]string{
			"config.yaml": r.buildConfig(instance),
		}
		return controllerutil.SetControllerReference(instance, configMap, r.Scheme)
	})
	return err
}

func (r *MaiaInstanceReconciler) buildConfig(instance *maiav1alpha1.MaiaInstance) string {
	spec := instance.Spec

	// Build YAML config
	config := fmt.Sprintf(`server:
  port: %d
  grpc_port: %d

storage:
  data_dir: %s
  sync_writes: %t
  gc_interval: %s

logging:
  level: %s
  format: %s

embedding:
  model: %s
`,
		defaultHTTPPort,
		defaultGRPCPort,
		getStorageDataDir(spec.Storage),
		spec.Storage.SyncWrites,
		getGCInterval(spec.Storage),
		getLoggingLevel(spec.Logging),
		getLoggingFormat(spec.Logging),
		getEmbeddingModel(spec.Embedding),
	)

	// Add tenancy config if enabled
	if spec.Tenancy.Enabled {
		config += fmt.Sprintf(`
tenant:
  enabled: true
  require_tenant: %t
  default_tenant_id: %s
  enforce_scopes_enabled: %t
`,
			spec.Tenancy.RequireTenant,
			spec.Tenancy.DefaultTenantID,
			spec.Tenancy.EnforceScopesEnabled,
		)
	}

	// Add rate limit config if enabled
	if spec.RateLimit.Enabled {
		config += fmt.Sprintf(`
rate_limit:
  enabled: true
  requests_per_second: %d
  burst: %d
`,
			getRateLimitRPS(spec.RateLimit),
			getRateLimitBurst(spec.RateLimit),
		)
	}

	// Add metrics config
	if spec.Metrics.Enabled {
		config += `
metrics:
  enabled: true
`
	}

	return config
}

func (r *MaiaInstanceReconciler) reconcilePVC(ctx context.Context, instance *maiav1alpha1.MaiaInstance) error {
	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      instance.Name + "-data",
			Namespace: instance.Namespace,
		},
	}

	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, pvc, func() error {
		// Only set spec on creation
		if pvc.CreationTimestamp.IsZero() {
			storageSize := getStorageSize(instance.Spec.Storage)
			pvc.Spec = corev1.PersistentVolumeClaimSpec{
				AccessModes: []corev1.PersistentVolumeAccessMode{
					corev1.ReadWriteOnce,
				},
				Resources: corev1.VolumeResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceStorage: resource.MustParse(storageSize),
					},
				},
			}
			if instance.Spec.Storage.StorageClassName != nil {
				pvc.Spec.StorageClassName = instance.Spec.Storage.StorageClassName
			}
		}
		return controllerutil.SetControllerReference(instance, pvc, r.Scheme)
	})
	return err
}

func (r *MaiaInstanceReconciler) reconcileDeployment(ctx context.Context, instance *maiav1alpha1.MaiaInstance) error {
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      instance.Name,
			Namespace: instance.Namespace,
		},
	}

	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, deployment, func() error {
		replicas := int32(1)
		if instance.Spec.Replicas != nil {
			replicas = *instance.Spec.Replicas
		}

		labels := map[string]string{
			"app.kubernetes.io/name":       "maia",
			"app.kubernetes.io/instance":   instance.Name,
			"app.kubernetes.io/managed-by": "maia-operator",
		}

		deployment.Spec = appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:            "maia",
							Image:           getImage(instance.Spec.Image),
							ImagePullPolicy: getImagePullPolicy(instance.Spec.Image),
							Ports: []corev1.ContainerPort{
								{
									Name:          "http",
									ContainerPort: defaultHTTPPort,
									Protocol:      corev1.ProtocolTCP,
								},
								{
									Name:          "grpc",
									ContainerPort: defaultGRPCPort,
									Protocol:      corev1.ProtocolTCP,
								},
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "config",
									MountPath: "/config",
								},
								{
									Name:      "data",
									MountPath: getStorageDataDir(instance.Spec.Storage),
								},
							},
							Resources: buildResourceRequirements(instance.Spec.Resources),
							LivenessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									HTTPGet: &corev1.HTTPGetAction{
										Path: "/health",
										Port: intstr.FromInt(defaultHTTPPort),
									},
								},
								InitialDelaySeconds: 10,
								PeriodSeconds:       30,
							},
							ReadinessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									HTTPGet: &corev1.HTTPGetAction{
										Path: "/ready",
										Port: intstr.FromInt(defaultHTTPPort),
									},
								},
								InitialDelaySeconds: 5,
								PeriodSeconds:       10,
							},
							Env: r.buildEnvVars(instance),
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: "config",
							VolumeSource: corev1.VolumeSource{
								ConfigMap: &corev1.ConfigMapVolumeSource{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: instance.Name + "-config",
									},
								},
							},
						},
						{
							Name: "data",
							VolumeSource: corev1.VolumeSource{
								PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
									ClaimName: instance.Name + "-data",
								},
							},
						},
					},
					SecurityContext: &corev1.PodSecurityContext{
						RunAsNonRoot: ptr(true),
						RunAsUser:    ptr(int64(1000)),
						FSGroup:      ptr(int64(1000)),
					},
				},
			},
		}

		return controllerutil.SetControllerReference(instance, deployment, r.Scheme)
	})
	return err
}

func (r *MaiaInstanceReconciler) buildEnvVars(instance *maiav1alpha1.MaiaInstance) []corev1.EnvVar {
	envVars := []corev1.EnvVar{
		{
			Name:  "MAIA_CONFIG",
			Value: "/config/config.yaml",
		},
	}

	// Add API key from secret if configured
	if instance.Spec.Security.APIKeySecretRef != nil {
		key := "api-key"
		if instance.Spec.Security.APIKeySecretRef.Key != "" {
			key = instance.Spec.Security.APIKeySecretRef.Key
		}
		envVars = append(envVars, corev1.EnvVar{
			Name: "MAIA_API_KEY",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: instance.Spec.Security.APIKeySecretRef.Name,
					},
					Key: key,
				},
			},
		})
	}

	// Add OpenAI key if using OpenAI embedding
	if instance.Spec.Embedding.Model == "openai" && instance.Spec.Embedding.OpenAISecretRef != nil {
		key := "openai-api-key"
		if instance.Spec.Embedding.OpenAISecretRef.Key != "" {
			key = instance.Spec.Embedding.OpenAISecretRef.Key
		}
		envVars = append(envVars, corev1.EnvVar{
			Name: "OPENAI_API_KEY",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: instance.Spec.Embedding.OpenAISecretRef.Name,
					},
					Key: key,
				},
			},
		})
	}

	// Add Ollama endpoint if using Ollama
	if instance.Spec.Embedding.Model == "ollama" && instance.Spec.Embedding.OllamaEndpoint != "" {
		envVars = append(envVars, corev1.EnvVar{
			Name:  "OLLAMA_ENDPOINT",
			Value: instance.Spec.Embedding.OllamaEndpoint,
		})
	}

	return envVars
}

func (r *MaiaInstanceReconciler) reconcileService(ctx context.Context, instance *maiav1alpha1.MaiaInstance) error {
	// Create main ClusterIP service
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      instance.Name,
			Namespace: instance.Namespace,
		},
	}

	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, service, func() error {
		labels := map[string]string{
			"app.kubernetes.io/name":       "maia",
			"app.kubernetes.io/instance":   instance.Name,
			"app.kubernetes.io/managed-by": "maia-operator",
		}

		service.Spec = corev1.ServiceSpec{
			Selector: labels,
			Ports: []corev1.ServicePort{
				{
					Name:       "http",
					Port:       defaultHTTPPort,
					TargetPort: intstr.FromInt(defaultHTTPPort),
					Protocol:   corev1.ProtocolTCP,
				},
				{
					Name:       "grpc",
					Port:       defaultGRPCPort,
					TargetPort: intstr.FromInt(defaultGRPCPort),
					Protocol:   corev1.ProtocolTCP,
				},
			},
			Type: corev1.ServiceTypeClusterIP,
		}

		return controllerutil.SetControllerReference(instance, service, r.Scheme)
	})
	if err != nil {
		return err
	}

	// Create headless service
	headlessService := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      instance.Name + "-headless",
			Namespace: instance.Namespace,
		},
	}

	_, err = controllerutil.CreateOrUpdate(ctx, r.Client, headlessService, func() error {
		labels := map[string]string{
			"app.kubernetes.io/name":       "maia",
			"app.kubernetes.io/instance":   instance.Name,
			"app.kubernetes.io/managed-by": "maia-operator",
		}

		headlessService.Spec = corev1.ServiceSpec{
			Selector:  labels,
			ClusterIP: "None",
			Ports: []corev1.ServicePort{
				{
					Name:       "http",
					Port:       defaultHTTPPort,
					TargetPort: intstr.FromInt(defaultHTTPPort),
					Protocol:   corev1.ProtocolTCP,
				},
			},
		}

		return controllerutil.SetControllerReference(instance, headlessService, r.Scheme)
	})
	return err
}

func (r *MaiaInstanceReconciler) reconcileIngress(ctx context.Context, instance *maiav1alpha1.MaiaInstance) error {
	ingress := &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      instance.Name,
			Namespace: instance.Namespace,
		},
	}

	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, ingress, func() error {
		pathType := networkingv1.PathTypePrefix

		ingress.Annotations = instance.Spec.Ingress.Annotations
		ingress.Spec = networkingv1.IngressSpec{
			Rules: []networkingv1.IngressRule{
				{
					Host: instance.Spec.Ingress.Host,
					IngressRuleValue: networkingv1.IngressRuleValue{
						HTTP: &networkingv1.HTTPIngressRuleValue{
							Paths: []networkingv1.HTTPIngressPath{
								{
									Path:     "/",
									PathType: &pathType,
									Backend: networkingv1.IngressBackend{
										Service: &networkingv1.IngressServiceBackend{
											Name: instance.Name,
											Port: networkingv1.ServiceBackendPort{
												Number: defaultHTTPPort,
											},
										},
									},
								},
							},
						},
					},
				},
			},
		}

		if instance.Spec.Ingress.ClassName != nil {
			ingress.Spec.IngressClassName = instance.Spec.Ingress.ClassName
		}

		if instance.Spec.Ingress.TLS {
			ingress.Spec.TLS = []networkingv1.IngressTLS{
				{
					Hosts:      []string{instance.Spec.Ingress.Host},
					SecretName: instance.Name + "-tls",
				},
			}
		}

		return controllerutil.SetControllerReference(instance, ingress, r.Scheme)
	})
	return err
}

func (r *MaiaInstanceReconciler) reconcileServiceMonitor(ctx context.Context, instance *maiav1alpha1.MaiaInstance) error {
	sm := &monitoringv1.ServiceMonitor{
		ObjectMeta: metav1.ObjectMeta{
			Name:      instance.Name,
			Namespace: instance.Namespace,
		},
	}

	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, sm, func() error {
		labels := map[string]string{
			"app.kubernetes.io/name":       "maia",
			"app.kubernetes.io/instance":   instance.Name,
			"app.kubernetes.io/managed-by": "maia-operator",
		}

		// Merge with user-provided labels
		for k, v := range instance.Spec.Metrics.ServiceMonitor.Labels {
			labels[k] = v
		}

		sm.Labels = labels

		interval := instance.Spec.Metrics.ServiceMonitor.Interval
		if interval == "" {
			interval = "30s"
		}

		sm.Spec = monitoringv1.ServiceMonitorSpec{
			Selector: metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app.kubernetes.io/name":     "maia",
					"app.kubernetes.io/instance": instance.Name,
				},
			},
			NamespaceSelector: monitoringv1.NamespaceSelector{
				MatchNames: []string{instance.Namespace},
			},
			Endpoints: []monitoringv1.Endpoint{
				{
					Port:     "http",
					Path:     "/metrics",
					Interval: monitoringv1.Duration(interval),
				},
			},
		}

		return controllerutil.SetControllerReference(instance, sm, r.Scheme)
	})
	return err
}

func (r *MaiaInstanceReconciler) reconcileBackupPVC(ctx context.Context, instance *maiav1alpha1.MaiaInstance) error {
	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      instance.Name + "-backup",
			Namespace: instance.Namespace,
		},
	}

	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, pvc, func() error {
		// Only set spec on creation (PVC specs are immutable)
		if pvc.CreationTimestamp.IsZero() {
			storageSize := instance.Spec.Backup.StorageSize
			if storageSize == "" {
				storageSize = "20Gi"
			}

			pvc.Spec = corev1.PersistentVolumeClaimSpec{
				AccessModes: []corev1.PersistentVolumeAccessMode{
					corev1.ReadWriteOnce,
				},
				Resources: corev1.VolumeResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceStorage: resource.MustParse(storageSize),
					},
				},
			}
			if instance.Spec.Storage.StorageClassName != nil {
				pvc.Spec.StorageClassName = instance.Spec.Storage.StorageClassName
			}
		}
		return controllerutil.SetControllerReference(instance, pvc, r.Scheme)
	})
	return err
}

func (r *MaiaInstanceReconciler) reconcileBackupCronJob(ctx context.Context, instance *maiav1alpha1.MaiaInstance) error {
	cronJob := &batchv1.CronJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      instance.Name + "-backup",
			Namespace: instance.Namespace,
		},
	}

	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, cronJob, func() error {
		labels := map[string]string{
			"app.kubernetes.io/name":       "maia",
			"app.kubernetes.io/instance":   instance.Name,
			"app.kubernetes.io/component":  "backup",
			"app.kubernetes.io/managed-by": "maia-operator",
		}

		schedule := instance.Spec.Backup.Schedule
		if schedule == "" {
			schedule = "0 2 * * *" // Default: 2 AM daily
		}

		retentionDays := instance.Spec.Backup.RetentionDays
		if retentionDays == 0 {
			retentionDays = 30
		}

		compress := instance.Spec.Backup.Compress

		// Build backup command
		backupCmd := fmt.Sprintf(`#!/bin/sh
set -e

BACKUP_DIR="/backup"
DATA_DIR="%s"
TIMESTAMP=$(date +%%Y%%m%%d-%%H%%M%%S)
BACKUP_NAME="maia-backup-${TIMESTAMP}"

echo "Starting backup at $(date)"

# Create backup
if [ "%t" = "true" ]; then
    tar -czf "${BACKUP_DIR}/${BACKUP_NAME}.tar.gz" -C "${DATA_DIR}" .
    sha256sum "${BACKUP_DIR}/${BACKUP_NAME}.tar.gz" > "${BACKUP_DIR}/${BACKUP_NAME}.tar.gz.sha256"
else
    tar -cf "${BACKUP_DIR}/${BACKUP_NAME}.tar" -C "${DATA_DIR}" .
    sha256sum "${BACKUP_DIR}/${BACKUP_NAME}.tar" > "${BACKUP_DIR}/${BACKUP_NAME}.tar.sha256"
fi

# Clean up old backups
echo "Cleaning up backups older than %d days"
find "${BACKUP_DIR}" -name "maia-backup-*.tar*" -mtime +%d -delete

echo "Backup completed at $(date)"
`, getStorageDataDir(instance.Spec.Storage), compress, retentionDays, retentionDays)

		successfulJobsHistoryLimit := int32(3)
		failedJobsHistoryLimit := int32(1)
		backoffLimit := int32(3)

		cronJob.Spec = batchv1.CronJobSpec{
			Schedule:                   schedule,
			SuccessfulJobsHistoryLimit: &successfulJobsHistoryLimit,
			FailedJobsHistoryLimit:     &failedJobsHistoryLimit,
			JobTemplate: batchv1.JobTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: batchv1.JobSpec{
					BackoffLimit: &backoffLimit,
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: labels,
						},
						Spec: corev1.PodSpec{
							RestartPolicy: corev1.RestartPolicyOnFailure,
							Containers: []corev1.Container{
								{
									Name:  "backup",
									Image: getImage(instance.Spec.Image),
									Command: []string{
										"/bin/sh",
										"-c",
										backupCmd,
									},
									VolumeMounts: []corev1.VolumeMount{
										{
											Name:      "data",
											MountPath: getStorageDataDir(instance.Spec.Storage),
											ReadOnly:  true,
										},
										{
											Name:      "backup",
											MountPath: "/backup",
										},
									},
									Resources: corev1.ResourceRequirements{
										Requests: corev1.ResourceList{
											corev1.ResourceCPU:    resource.MustParse("100m"),
											corev1.ResourceMemory: resource.MustParse("128Mi"),
										},
										Limits: corev1.ResourceList{
											corev1.ResourceCPU:    resource.MustParse("500m"),
											corev1.ResourceMemory: resource.MustParse("512Mi"),
										},
									},
								},
							},
							Volumes: []corev1.Volume{
								{
									Name: "data",
									VolumeSource: corev1.VolumeSource{
										PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
											ClaimName: instance.Name + "-data",
										},
									},
								},
								{
									Name: "backup",
									VolumeSource: corev1.VolumeSource{
										PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
											ClaimName: instance.Name + "-backup",
										},
									},
								},
							},
							SecurityContext: &corev1.PodSecurityContext{
								RunAsNonRoot: ptr(true),
								RunAsUser:    ptr(int64(1000)),
								FSGroup:      ptr(int64(1000)),
							},
						},
					},
				},
			},
		}

		return controllerutil.SetControllerReference(instance, cronJob, r.Scheme)
	})
	return err
}

func (r *MaiaInstanceReconciler) updateStatus(ctx context.Context, instance *maiav1alpha1.MaiaInstance) error {
	// Get the deployment to check status
	deployment := &appsv1.Deployment{}
	err := r.Get(ctx, types.NamespacedName{Name: instance.Name, Namespace: instance.Namespace}, deployment)
	if err != nil {
		if errors.IsNotFound(err) {
			instance.Status.Phase = maiav1alpha1.InstancePhasePending
		} else {
			return err
		}
	} else {
		instance.Status.Replicas = deployment.Status.Replicas
		instance.Status.ReadyReplicas = deployment.Status.ReadyReplicas

		if deployment.Status.ReadyReplicas > 0 && deployment.Status.ReadyReplicas == deployment.Status.Replicas {
			instance.Status.Phase = maiav1alpha1.InstancePhaseRunning
			r.setCondition(ctx, instance, maiav1alpha1.ConditionTypeReady, metav1.ConditionTrue,
				"DeploymentReady", "All replicas are ready")
		} else if deployment.Status.ReadyReplicas < deployment.Status.Replicas {
			instance.Status.Phase = maiav1alpha1.InstancePhaseUpdating
			r.setCondition(ctx, instance, maiav1alpha1.ConditionTypeProgressing, metav1.ConditionTrue,
				"DeploymentUpdating", "Deployment is updating")
		}
	}

	// Set endpoint
	instance.Status.Endpoint = fmt.Sprintf("http://%s.%s.svc:%d",
		instance.Name, instance.Namespace, defaultHTTPPort)

	return r.Status().Update(ctx, instance)
}

func (r *MaiaInstanceReconciler) setCondition(ctx context.Context, instance *maiav1alpha1.MaiaInstance,
	conditionType string, status metav1.ConditionStatus, reason, message string) {
	meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
		Type:               conditionType,
		Status:             status,
		Reason:             reason,
		Message:            message,
		ObservedGeneration: instance.Generation,
	})
}

// SetupWithManager sets up the controller with the Manager.
func (r *MaiaInstanceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&maiav1alpha1.MaiaInstance{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Service{}).
		Owns(&corev1.ConfigMap{}).
		Owns(&corev1.PersistentVolumeClaim{}).
		Owns(&networkingv1.Ingress{}).
		Owns(&batchv1.CronJob{}).
		Owns(&monitoringv1.ServiceMonitor{}).
		Named("maiainstance").
		Complete(r)
}

// Helper functions

func ptr[T any](v T) *T {
	return &v
}

func getImage(spec maiav1alpha1.ImageSpec) string {
	repo := "ghcr.io/ar4mirez/maia"
	if spec.Repository != "" {
		repo = spec.Repository
	}
	// Default to "latest" only if no tag is specified.
	// Users should specify a semantic version (e.g., "v1.0.0") in production.
	tag := "latest"
	if spec.Tag != "" {
		tag = spec.Tag
	}
	return fmt.Sprintf("%s:%s", repo, tag)
}

func getImagePullPolicy(spec maiav1alpha1.ImageSpec) corev1.PullPolicy {
	if spec.PullPolicy != "" {
		return spec.PullPolicy
	}
	return corev1.PullIfNotPresent
}

func getStorageSize(spec maiav1alpha1.StorageSpec) string {
	if spec.Size != "" {
		return spec.Size
	}
	return "10Gi"
}

func getStorageDataDir(spec maiav1alpha1.StorageSpec) string {
	if spec.DataDir != "" {
		return spec.DataDir
	}
	return "/data"
}

func getGCInterval(spec maiav1alpha1.StorageSpec) string {
	if spec.GCInterval != "" {
		return spec.GCInterval
	}
	return "5m"
}

func getLoggingLevel(spec maiav1alpha1.LoggingSpec) string {
	if spec.Level != "" {
		return spec.Level
	}
	return "info"
}

func getLoggingFormat(spec maiav1alpha1.LoggingSpec) string {
	if spec.Format != "" {
		return spec.Format
	}
	return "json"
}

func getEmbeddingModel(spec maiav1alpha1.EmbeddingSpec) string {
	if spec.Model != "" {
		return spec.Model
	}
	return "local"
}

func getRateLimitRPS(spec maiav1alpha1.RateLimitSpec) int {
	if spec.RequestsPerSecond > 0 {
		return spec.RequestsPerSecond
	}
	return 100
}

func getRateLimitBurst(spec maiav1alpha1.RateLimitSpec) int {
	if spec.Burst > 0 {
		return spec.Burst
	}
	return 200
}

func buildResourceRequirements(spec maiav1alpha1.ResourcesSpec) corev1.ResourceRequirements {
	resources := corev1.ResourceRequirements{
		Limits:   corev1.ResourceList{},
		Requests: corev1.ResourceList{},
	}

	// Limits
	if spec.Limits.CPU != "" {
		resources.Limits[corev1.ResourceCPU] = resource.MustParse(spec.Limits.CPU)
	} else {
		resources.Limits[corev1.ResourceCPU] = resource.MustParse("1000m")
	}
	if spec.Limits.Memory != "" {
		resources.Limits[corev1.ResourceMemory] = resource.MustParse(spec.Limits.Memory)
	} else {
		resources.Limits[corev1.ResourceMemory] = resource.MustParse("1Gi")
	}

	// Requests
	if spec.Requests.CPU != "" {
		resources.Requests[corev1.ResourceCPU] = resource.MustParse(spec.Requests.CPU)
	} else {
		resources.Requests[corev1.ResourceCPU] = resource.MustParse("100m")
	}
	if spec.Requests.Memory != "" {
		resources.Requests[corev1.ResourceMemory] = resource.MustParse(spec.Requests.Memory)
	} else {
		resources.Requests[corev1.ResourceMemory] = resource.MustParse("256Mi")
	}

	return resources
}
