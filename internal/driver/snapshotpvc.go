package driver

import (
	"context"
	"fmt"

	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

const snapshotPVCName = "etcd-snapshots"

// SnapshotPVCProvisioner ensures a dedicated PVC exists for storing etcd snapshots.
type SnapshotPVCProvisioner struct {
	k8sClient    kubernetes.Interface
	storageClass string
	pvcSize      string
	logger       *zap.SugaredLogger
}

// NewSnapshotPVCProvisioner creates a new SnapshotPVCProvisioner.
func NewSnapshotPVCProvisioner(k8sClient kubernetes.Interface, storageClass, pvcSize string, logger *zap.SugaredLogger) *SnapshotPVCProvisioner {
	return &SnapshotPVCProvisioner{
		k8sClient:    k8sClient,
		storageClass: storageClass,
		pvcSize:      pvcSize,
		logger:       logger,
	}
}

// EnsureSnapshotPVC ensures a dedicated snapshot PVC exists in the given namespace,
// creating it if necessary. Returns the PVC name.
func (p *SnapshotPVCProvisioner) EnsureSnapshotPVC(ctx context.Context, namespace string) (string, error) {
	// Try to get existing PVC
	_, err := p.k8sClient.CoreV1().PersistentVolumeClaims(namespace).Get(ctx, snapshotPVCName, metav1.GetOptions{})
	if err == nil {
		p.logger.Debugw("Snapshot PVC already exists",
			"pvc_name", snapshotPVCName,
			"namespace", namespace,
		)
		return snapshotPVCName, nil
	}

	if !errors.IsNotFound(err) {
		return "", fmt.Errorf("failed to check for existing snapshot PVC: %w", err)
	}

	// Parse the requested size
	quantity, err := resource.ParseQuantity(p.pvcSize)
	if err != nil {
		return "", fmt.Errorf("invalid snapshot PVC size %q: %w", p.pvcSize, err)
	}

	// Build the PVC spec
	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      snapshotPVCName,
			Namespace: namespace,
			Labels: map[string]string{
				"app": "etcd-snapshot-driver",
			},
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{
				corev1.ReadWriteOnce,
			},
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: quantity,
				},
			},
		},
	}

	// Set storage class if specified (empty string means use cluster default)
	if p.storageClass != "" {
		pvc.Spec.StorageClassName = &p.storageClass
	}

	p.logger.Infow("Creating dedicated snapshot PVC",
		"pvc_name", snapshotPVCName,
		"namespace", namespace,
		"size", p.pvcSize,
		"storage_class", p.storageClass,
	)

	_, err = p.k8sClient.CoreV1().PersistentVolumeClaims(namespace).Create(ctx, pvc, metav1.CreateOptions{})
	if err != nil {
		// Handle race condition where another request created the PVC concurrently
		if errors.IsAlreadyExists(err) {
			p.logger.Debugw("Snapshot PVC created concurrently by another request",
				"pvc_name", snapshotPVCName,
				"namespace", namespace,
			)
			return snapshotPVCName, nil
		}
		return "", fmt.Errorf("failed to create snapshot PVC: %w", err)
	}

	p.logger.Infow("Dedicated snapshot PVC created successfully",
		"pvc_name", snapshotPVCName,
		"namespace", namespace,
	)

	return snapshotPVCName, nil
}
