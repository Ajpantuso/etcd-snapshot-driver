package driver

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestEnsureSnapshotPVC_CreatesNew(t *testing.T) {
	fakeClient := fake.NewSimpleClientset()
	logger := zap.NewNop().Sugar()

	provisioner := NewSnapshotPVCProvisioner(fakeClient, "gp2", "10Gi", logger)

	ctx := context.Background()
	name, err := provisioner.EnsureSnapshotPVC(ctx, "test-ns")
	require.NoError(t, err)
	assert.Equal(t, snapshotPVCName, name)

	// Verify PVC was created with correct spec
	pvc, err := fakeClient.CoreV1().PersistentVolumeClaims("test-ns").Get(ctx, snapshotPVCName, metav1.GetOptions{})
	require.NoError(t, err)

	assert.Equal(t, "etcd-snapshot-driver", pvc.Labels["app"])
	assert.Equal(t, []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce}, pvc.Spec.AccessModes)

	expectedSize := resource.MustParse("10Gi")
	actualSize := pvc.Spec.Resources.Requests[corev1.ResourceStorage]
	assert.True(t, expectedSize.Equal(actualSize), "expected size %s, got %s", expectedSize.String(), actualSize.String())

	require.NotNil(t, pvc.Spec.StorageClassName)
	assert.Equal(t, "gp2", *pvc.Spec.StorageClassName)
}

func TestEnsureSnapshotPVC_ReusesExisting(t *testing.T) {
	existingPVC := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      snapshotPVCName,
			Namespace: "test-ns",
		},
		Status: corev1.PersistentVolumeClaimStatus{
			Phase: corev1.ClaimBound,
		},
	}

	fakeClient := fake.NewSimpleClientset(existingPVC)
	logger := zap.NewNop().Sugar()

	provisioner := NewSnapshotPVCProvisioner(fakeClient, "gp2", "10Gi", logger)

	ctx := context.Background()
	name, err := provisioner.EnsureSnapshotPVC(ctx, "test-ns")
	require.NoError(t, err)
	assert.Equal(t, snapshotPVCName, name)
}

func TestEnsureSnapshotPVC_PendingPVC(t *testing.T) {
	existingPVC := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      snapshotPVCName,
			Namespace: "test-ns",
		},
		Status: corev1.PersistentVolumeClaimStatus{
			Phase: corev1.ClaimPending,
		},
	}

	fakeClient := fake.NewSimpleClientset(existingPVC)
	logger := zap.NewNop().Sugar()

	provisioner := NewSnapshotPVCProvisioner(fakeClient, "gp2", "10Gi", logger)

	ctx := context.Background()
	name, err := provisioner.EnsureSnapshotPVC(ctx, "test-ns")
	require.NoError(t, err)
	assert.Equal(t, snapshotPVCName, name)
}

func TestEnsureSnapshotPVC_EmptyStorageClass(t *testing.T) {
	fakeClient := fake.NewSimpleClientset()
	logger := zap.NewNop().Sugar()

	provisioner := NewSnapshotPVCProvisioner(fakeClient, "", "10Gi", logger)

	ctx := context.Background()
	name, err := provisioner.EnsureSnapshotPVC(ctx, "test-ns")
	require.NoError(t, err)
	assert.Equal(t, snapshotPVCName, name)

	// Verify StorageClassName is nil (cluster default)
	pvc, err := fakeClient.CoreV1().PersistentVolumeClaims("test-ns").Get(ctx, snapshotPVCName, metav1.GetOptions{})
	require.NoError(t, err)
	assert.Nil(t, pvc.Spec.StorageClassName)
}

func TestEnsureSnapshotPVC_InvalidSize(t *testing.T) {
	fakeClient := fake.NewSimpleClientset()
	logger := zap.NewNop().Sugar()

	provisioner := NewSnapshotPVCProvisioner(fakeClient, "gp2", "not-a-size", logger)

	ctx := context.Background()
	_, err := provisioner.EnsureSnapshotPVC(ctx, "test-ns")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid snapshot PVC size")
}
