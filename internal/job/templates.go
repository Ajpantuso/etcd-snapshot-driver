package job

import (
	"fmt"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type JobConfig struct {
	SnapshotID            string
	Namespace             string
	ETCDEndpoints         []string
	SnapshotPVCName       string
	SnapshotPVCNamespace  string
	Timeout               int32
	BackoffLimit          int32
	ActiveDeadlineSeconds int64
	Operation             string // save, delete, restore

	// TLS Configuration
	TLSEnabled     bool
	TLSSecretName  string
	ClientCertPath string
	ClientKeyPath  string
	CAPath         string

	// Container Images
	ETCDImage    string
	BusyboxImage string
}

// GenerateSnapshotSaveJob creates a Kubernetes Job for snapshot save operation
// GenerateSnapshotSaveJob creates a Kubernetes Job for snapshot save operation
// buildSnapshotCommand creates a shell command for snapshot save with TLS and metadata output
func buildSnapshotCommand(cfg *JobConfig) string {
	// Build the etcdutl command with TLS flags if needed
	cmdParts := []string{
		"etcdutl",
		"--endpoints", fmt.Sprintf("%v", cfg.ETCDEndpoints),
	}

	if cfg.TLSEnabled {
		cmdParts = append(cmdParts,
			"--cacert", cfg.CAPath,
			"--cert", cfg.ClientCertPath,
			"--key", cfg.ClientKeyPath,
		)
	}

	cmdParts = append(cmdParts,
		"snapshot",
		"save",
		fmt.Sprintf("/snapshots/%s.db", cfg.SnapshotID),
	)

	return fmt.Sprintf("set -e\n%s\netcdutl snapshot status /snapshots/%s.db -w json\n",
		fmt.Sprintf("etcdutl --endpoints '%v'", cfg.ETCDEndpoints) + conditionalTLSFlags(cfg) +
			fmt.Sprintf(" snapshot save /snapshots/%s.db", cfg.SnapshotID),
		cfg.SnapshotID,
	)
}

// conditionalTLSFlags returns TLS flags if enabled
func conditionalTLSFlags(cfg *JobConfig) string {
	if !cfg.TLSEnabled {
		return ""
	}
	return fmt.Sprintf(" --cacert '%s' --cert '%s' --key '%s'",
		cfg.CAPath,
		cfg.ClientCertPath,
		cfg.ClientKeyPath,
	)
}

func GenerateSnapshotSaveJob(cfg *JobConfig) *batchv1.Job {
	jobName := fmt.Sprintf("etcd-snapshot-save-%s", cfg.SnapshotID)
	ttlSecondsAfterFinished := int32(3600) // 1 hour

	// Build command with TLS flags and metadata output
	command := []string{
		"sh",
		"-c",
		buildSnapshotCommand(cfg),
	}

	// Build volume mounts
	volumeMounts := []corev1.VolumeMount{
		{
			Name:      "snapshot-pvc",
			MountPath: "/snapshots",
		},
		{
			Name:      "tmp",
			MountPath: "/tmp",
		},
	}

	// Add TLS volume mounts if enabled
	if cfg.TLSEnabled {
		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      "etcd-client-tls",
			MountPath: "/etc/etcd/tls/client",
			ReadOnly:  true,
		})
		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      "etcd-ca",
			MountPath: "/etc/etcd/tls/etcd-ca",
			ReadOnly:  true,
		})
	}

	// Build volumes
	volumes := []corev1.Volume{
		{
			Name: "snapshot-pvc",
			VolumeSource: corev1.VolumeSource{
				PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
					ClaimName: cfg.SnapshotPVCName,
					ReadOnly:  false,
				},
			},
		},
		{
			Name: "tmp",
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		},
	}

	// Add TLS volumes if enabled
	if cfg.TLSEnabled {
		volumes = append(volumes, corev1.Volume{
			Name: "etcd-client-tls",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: cfg.TLSSecretName,
					Items: []corev1.KeyToPath{
						{
							Key:  "etcd-client.crt",
							Path: "etcd-client.crt",
						},
						{
							Key:  "etcd-client.key",
							Path: "etcd-client.key",
						},
					},
				},
			},
		})
		volumes = append(volumes, corev1.Volume{
			Name: "etcd-ca",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: cfg.TLSSecretName,
					Items: []corev1.KeyToPath{
						{
							Key:  "etcd-client-ca.crt",
							Path: "ca.crt",
						},
					},
				},
			},
		})
	}

	// Determine image to use
	image := cfg.ETCDImage
	if image == "" {
		image = "quay.io/coreos/etcd:v3.5.0"
	}

	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      jobName,
			Namespace: cfg.Namespace,
			Labels: map[string]string{
				"app":         "etcd-snapshot-driver",
				"operation":   "snapshot-save",
				"snapshot-id": cfg.SnapshotID,
			},
		},
		Spec: batchv1.JobSpec{
			TTLSecondsAfterFinished: &ttlSecondsAfterFinished,
			BackoffLimit:            &cfg.BackoffLimit,
			ActiveDeadlineSeconds:   &cfg.ActiveDeadlineSeconds,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app":         "etcd-snapshot-driver",
						"snapshot-id": cfg.SnapshotID,
					},
				},
				Spec: corev1.PodSpec{
					ServiceAccountName: "etcd-snapshot-executor",
					RestartPolicy:      corev1.RestartPolicyNever,
					SecurityContext: &corev1.PodSecurityContext{
						RunAsNonRoot: boolPtr(true),
						RunAsUser:    int64Ptr(65534),
						FSGroup:      int64Ptr(65534),
						SeccompProfile: &corev1.SeccompProfile{
							Type: corev1.SeccompProfileTypeRuntimeDefault,
						},
					},
					Containers: []corev1.Container{
						{
							Name:         "etcd-snapshot",
							Image:        image,
							Command:      command,
							VolumeMounts: volumeMounts,
							SecurityContext: &corev1.SecurityContext{
								AllowPrivilegeEscalation: boolPtr(false),
								Capabilities: &corev1.Capabilities{
									Drop: []corev1.Capability{"ALL"},
								},
								ReadOnlyRootFilesystem: boolPtr(true),
							},
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceMemory: mustParseQuantity("256Mi"),
									corev1.ResourceCPU:    mustParseQuantity("100m"),
								},
								Limits: corev1.ResourceList{
									corev1.ResourceMemory: mustParseQuantity("512Mi"),
									corev1.ResourceCPU:    mustParseQuantity("500m"),
								},
							},
						},
					},
					Volumes: volumes,
				},
			},
		},
	}

	return job
}

// GenerateSnapshotDeleteJob creates a Kubernetes Job for snapshot deletion
// GenerateSnapshotDeleteJob creates a Kubernetes Job for snapshot deletion
func GenerateSnapshotDeleteJob(cfg *JobConfig) *batchv1.Job {
	jobName := fmt.Sprintf("etcd-snapshot-delete-%s", cfg.SnapshotID)
	ttlSecondsAfterFinished := int32(3600)

	// Determine image to use
	image := cfg.BusyboxImage
	if image == "" {
		image = "busybox:1.35"
	}

	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      jobName,
			Namespace: cfg.Namespace,
			Labels: map[string]string{
				"app":         "etcd-snapshot-driver",
				"operation":   "snapshot-delete",
				"snapshot-id": cfg.SnapshotID,
			},
		},
		Spec: batchv1.JobSpec{
			TTLSecondsAfterFinished: &ttlSecondsAfterFinished,
			BackoffLimit:            &cfg.BackoffLimit,
			ActiveDeadlineSeconds:   &cfg.ActiveDeadlineSeconds,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app":         "etcd-snapshot-driver",
						"snapshot-id": cfg.SnapshotID,
					},
				},
				Spec: corev1.PodSpec{
					ServiceAccountName: "etcd-snapshot-executor",
					RestartPolicy:      corev1.RestartPolicyNever,
					SecurityContext: &corev1.PodSecurityContext{
						RunAsNonRoot: boolPtr(true),
						RunAsUser:    int64Ptr(65534),
						FSGroup:      int64Ptr(65534),
						SeccompProfile: &corev1.SeccompProfile{
							Type: corev1.SeccompProfileTypeRuntimeDefault,
						},
					},
					Containers: []corev1.Container{
						{
							Name:    "rm",
							Image:   image,
							Command: []string{"rm", "-f", fmt.Sprintf("/snapshots/%s.db", cfg.SnapshotID)},
							SecurityContext: &corev1.SecurityContext{
								AllowPrivilegeEscalation: boolPtr(false),
								Capabilities: &corev1.Capabilities{
									Drop: []corev1.Capability{"ALL"},
								},
								ReadOnlyRootFilesystem: boolPtr(true),
							},
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceMemory: mustParseQuantity("32Mi"),
									corev1.ResourceCPU:    mustParseQuantity("50m"),
								},
								Limits: corev1.ResourceList{
									corev1.ResourceMemory: mustParseQuantity("128Mi"),
									corev1.ResourceCPU:    mustParseQuantity("100m"),
								},
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "snapshot-pvc",
									MountPath: "/snapshots",
								},
								{
									Name:      "tmp",
									MountPath: "/tmp",
								},
							},
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: "snapshot-pvc",
							VolumeSource: corev1.VolumeSource{
								PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
									ClaimName: cfg.SnapshotPVCName,
									ReadOnly:  false,
								},
							},
						},
						{
							Name: "tmp",
							VolumeSource: corev1.VolumeSource{
								EmptyDir: &corev1.EmptyDirVolumeSource{},
							},
						},
					},
				},
			},
		},
	}

	return job
}

// Helper functions
func boolPtr(b bool) *bool {
	return &b
}

func int64Ptr(i int64) *int64 {
	return &i
}

func mustParseQuantity(s string) resource.Quantity {
	quantity, err := resource.ParseQuantity(s)
	if err != nil {
		panic(fmt.Sprintf("invalid quantity: %s", s))
	}
	return quantity
}
