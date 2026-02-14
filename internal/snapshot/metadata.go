package snapshot

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

type SnapshotMetadata struct {
	SnapshotID     string    `json:"snapshot_id"`
	SourceVolumeID string    `json:"source_volume_id"`
	ClusterName    string    `json:"cluster_name"`
	CreationTime   time.Time `json:"creation_time"`
	Size           int64     `json:"size"`
	ChecksumSHA256 string    `json:"checksum_sha256"`
	ReadyToUse     bool      `json:"ready_to_use"`
	PVCName        string    `json:"pvc_name"`
	Namespace      string    `json:"namespace"`
}

type GroupSnapshotMetadata struct {
	GroupSnapshotID      string    `json:"group_snapshot_id"`
	SnapshotID           string    `json:"snapshot_id"`
	SourceVolumeIDs      []string  `json:"source_volume_ids"`
	ClusterName          string    `json:"cluster_name"`
	SnapshotPVCName      string    `json:"snapshot_pvc_name"`
	SnapshotPVCNamespace string    `json:"snapshot_pvc_namespace"`
	CreationTime         time.Time `json:"creation_time"`
	ReadyToUse           bool      `json:"ready_to_use"`
}

type Manager struct {
	k8sClient kubernetes.Interface
	logger    *zap.SugaredLogger
	namespace string // kube-system
}

func NewManager(k8sClient kubernetes.Interface, logger *zap.SugaredLogger, namespace string) *Manager {
	return &Manager{
		k8sClient: k8sClient,
		logger:    logger,
		namespace: namespace,
	}
}

// StoreSnapshotMetadata saves snapshot metadata to a ConfigMap
func (m *Manager) StoreSnapshotMetadata(ctx context.Context, metadata *SnapshotMetadata) error {
	m.logger.Debugw("Storing snapshot metadata",
		"snapshot_id", metadata.SnapshotID,
	)

	data, err := json.Marshal(metadata)
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}

	configMapName := "etcd-snapshot-metadata"

	// Get or create ConfigMap
	cm, err := m.k8sClient.CoreV1().ConfigMaps(m.namespace).Get(ctx, configMapName, metav1.GetOptions{})
	if err != nil {
		// Create new ConfigMap
		cm = &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      configMapName,
				Namespace: m.namespace,
				Labels: map[string]string{
					"app": "etcd-snapshot-driver",
				},
			},
			Data: map[string]string{
				metadata.SnapshotID: string(data),
			},
		}

		if _, err := m.k8sClient.CoreV1().ConfigMaps(m.namespace).Create(ctx, cm, metav1.CreateOptions{}); err != nil {
			return fmt.Errorf("failed to create ConfigMap: %w", err)
		}
		m.logger.Infow("Created ConfigMap for snapshot metadata",
			"configmap_name", configMapName,
			"snapshot_id", metadata.SnapshotID,
		)
		return nil
	}

	// Update existing ConfigMap
	if cm.Data == nil {
		cm.Data = make(map[string]string)
	}
	cm.Data[metadata.SnapshotID] = string(data)

	if _, err := m.k8sClient.CoreV1().ConfigMaps(m.namespace).Update(ctx, cm, metav1.UpdateOptions{}); err != nil {
		return fmt.Errorf("failed to update ConfigMap: %w", err)
	}

	m.logger.Infow("Stored snapshot metadata",
		"snapshot_id", metadata.SnapshotID,
	)
	return nil
}

// RetrieveSnapshotMetadata retrieves snapshot metadata from ConfigMap
func (m *Manager) RetrieveSnapshotMetadata(ctx context.Context, snapshotID string) (*SnapshotMetadata, error) {
	m.logger.Debugw("Retrieving snapshot metadata",
		"snapshot_id", snapshotID,
	)

	configMapName := "etcd-snapshot-metadata"
	cm, err := m.k8sClient.CoreV1().ConfigMaps(m.namespace).Get(ctx, configMapName, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get ConfigMap: %w", err)
	}

	if data, ok := cm.Data[snapshotID]; ok {
		var metadata SnapshotMetadata
		if err := json.Unmarshal([]byte(data), &metadata); err != nil {
			return nil, fmt.Errorf("failed to unmarshal metadata: %w", err)
		}
		return &metadata, nil
	}

	return nil, fmt.Errorf("snapshot metadata not found: %s", snapshotID)
}

// DeleteSnapshotMetadata removes snapshot metadata from ConfigMap
func (m *Manager) DeleteSnapshotMetadata(ctx context.Context, snapshotID string) error {
	m.logger.Debugw("Deleting snapshot metadata",
		"snapshot_id", snapshotID,
	)

	configMapName := "etcd-snapshot-metadata"
	cm, err := m.k8sClient.CoreV1().ConfigMaps(m.namespace).Get(ctx, configMapName, metav1.GetOptions{})
	if err != nil {
		// ConfigMap doesn't exist, which is fine for deletion
		m.logger.Debugw("ConfigMap not found, nothing to delete")
		return nil
	}

	if _, ok := cm.Data[snapshotID]; ok {
		delete(cm.Data, snapshotID)
		if _, err := m.k8sClient.CoreV1().ConfigMaps(m.namespace).Update(ctx, cm, metav1.UpdateOptions{}); err != nil {
			return fmt.Errorf("failed to update ConfigMap: %w", err)
		}
		m.logger.Infow("Deleted snapshot metadata",
			"snapshot_id", snapshotID,
		)
	}

	return nil
}

// StoreGroupSnapshotMetadata saves group snapshot metadata to a ConfigMap
func (m *Manager) StoreGroupSnapshotMetadata(ctx context.Context, metadata *GroupSnapshotMetadata) error {
	m.logger.Debugw("Storing group snapshot metadata",
		"group_snapshot_id", metadata.GroupSnapshotID,
	)

	data, err := json.Marshal(metadata)
	if err != nil {
		return fmt.Errorf("failed to marshal group metadata: %w", err)
	}

	configMapName := "etcd-group-snapshot-metadata"

	// Get or create ConfigMap
	cm, err := m.k8sClient.CoreV1().ConfigMaps(m.namespace).Get(ctx, configMapName, metav1.GetOptions{})
	if err != nil {
		// Create new ConfigMap
		cm = &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      configMapName,
				Namespace: m.namespace,
				Labels: map[string]string{
					"app": "etcd-snapshot-driver",
				},
			},
			Data: map[string]string{
				metadata.GroupSnapshotID: string(data),
			},
		}

		if _, err := m.k8sClient.CoreV1().ConfigMaps(m.namespace).Create(ctx, cm, metav1.CreateOptions{}); err != nil {
			return fmt.Errorf("failed to create group snapshot ConfigMap: %w", err)
		}
		m.logger.Infow("Created ConfigMap for group snapshot metadata",
			"configmap_name", configMapName,
			"group_snapshot_id", metadata.GroupSnapshotID,
		)
		return nil
	}

	// Update existing ConfigMap
	if cm.Data == nil {
		cm.Data = make(map[string]string)
	}
	cm.Data[metadata.GroupSnapshotID] = string(data)

	if _, err := m.k8sClient.CoreV1().ConfigMaps(m.namespace).Update(ctx, cm, metav1.UpdateOptions{}); err != nil {
		return fmt.Errorf("failed to update group snapshot ConfigMap: %w", err)
	}

	m.logger.Infow("Stored group snapshot metadata",
		"group_snapshot_id", metadata.GroupSnapshotID,
	)
	return nil
}

// RetrieveGroupSnapshotMetadata retrieves group snapshot metadata from ConfigMap
func (m *Manager) RetrieveGroupSnapshotMetadata(ctx context.Context, groupSnapshotID string) (*GroupSnapshotMetadata, error) {
	m.logger.Debugw("Retrieving group snapshot metadata",
		"group_snapshot_id", groupSnapshotID,
	)

	configMapName := "etcd-group-snapshot-metadata"
	cm, err := m.k8sClient.CoreV1().ConfigMaps(m.namespace).Get(ctx, configMapName, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get group snapshot ConfigMap: %w", err)
	}

	if data, ok := cm.Data[groupSnapshotID]; ok {
		var metadata GroupSnapshotMetadata
		if err := json.Unmarshal([]byte(data), &metadata); err != nil {
			return nil, fmt.Errorf("failed to unmarshal group metadata: %w", err)
		}
		return &metadata, nil
	}

	return nil, fmt.Errorf("group snapshot metadata not found: %s", groupSnapshotID)
}

// DeleteGroupSnapshotMetadata removes group snapshot metadata from ConfigMap
func (m *Manager) DeleteGroupSnapshotMetadata(ctx context.Context, groupSnapshotID string) error {
	m.logger.Debugw("Deleting group snapshot metadata",
		"group_snapshot_id", groupSnapshotID,
	)

	configMapName := "etcd-group-snapshot-metadata"
	cm, err := m.k8sClient.CoreV1().ConfigMaps(m.namespace).Get(ctx, configMapName, metav1.GetOptions{})
	if err != nil {
		// ConfigMap doesn't exist, which is fine for deletion
		m.logger.Debugw("Group snapshot ConfigMap not found, nothing to delete")
		return nil
	}

	if _, ok := cm.Data[groupSnapshotID]; ok {
		delete(cm.Data, groupSnapshotID)
		if _, err := m.k8sClient.CoreV1().ConfigMaps(m.namespace).Update(ctx, cm, metav1.UpdateOptions{}); err != nil {
			return fmt.Errorf("failed to update group snapshot ConfigMap: %w", err)
		}
		m.logger.Infow("Deleted group snapshot metadata",
			"group_snapshot_id", groupSnapshotID,
		)
	}

	return nil
}
