package etcd

import (
	"context"
	"fmt"

	"go.uber.org/zap"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/kubernetes"
)

type Discovery struct {
	k8sClient        kubernetes.Interface
	logger           *zap.SugaredLogger
	clusterLabelKey  string
	healthValidator  *HealthValidator
}

func NewDiscovery(k8sClient kubernetes.Interface, logger *zap.SugaredLogger, clusterLabelKey string) *Discovery {
	return &Discovery{
		k8sClient:        k8sClient,
		logger:           logger,
		clusterLabelKey:  clusterLabelKey,
		healthValidator:  NewHealthValidator(logger, nil), // No TLS for now, will be added later
	}
}

// DiscoverCluster discovers ETCD cluster information using label-based discovery.
// The cluster label key is configurable and specified during initialization.
func (d *Discovery) DiscoverCluster(ctx context.Context, pvcNamespace, pvcName string) (*ClusterInfo, error) {
	d.logger.Infow("Discovering ETCD cluster",
		"pvc_namespace", pvcNamespace,
		"pvc_name", pvcName,
	)

	// Get the PVC to check for cluster label
	pvc, err := d.k8sClient.CoreV1().PersistentVolumeClaims(pvcNamespace).Get(ctx, pvcName, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get PVC: %w", err)
	}

	// Check for cluster label
	clusterName, ok := pvc.Labels[d.clusterLabelKey]
	if !ok {
		return nil, fmt.Errorf("PVC %s/%s missing required label %s", pvcNamespace, pvcName, d.clusterLabelKey)
	}

	return d.discoverByLabel(ctx, pvcNamespace, clusterName)
}

func (d *Discovery) discoverByLabel(ctx context.Context, namespace, clusterName string) (*ClusterInfo, error) {
	d.logger.Debugw("Attempting label-based discovery",
		"namespace", namespace,
		"cluster_name", clusterName,
		"label_key", d.clusterLabelKey,
	)

	// Find pods with matching cluster label
	labelSelector := labels.SelectorFromSet(map[string]string{
		d.clusterLabelKey: clusterName,
	})

	pods, err := d.k8sClient.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: labelSelector.String(),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list pods by label: %w", err)
	}

	if len(pods.Items) == 0 {
		return nil, fmt.Errorf("no ETCD pods found with label %s=%s", d.clusterLabelKey, clusterName)
	}

	endpoints := make([]string, 0)
	for _, pod := range pods.Items {
		// Assume ETCD API on port 2379
		endpoint := fmt.Sprintf("https://%s.%s.svc.cluster.local:2379", pod.Name, namespace)
		endpoints = append(endpoints, endpoint)
	}

	d.logger.Infow("Discovered ETCD cluster via labels",
		"cluster_name", clusterName,
		"endpoints", endpoints,
		"pod_count", len(pods.Items),
	)

	return &ClusterInfo{
		Name:      clusterName,
		Namespace: namespace,
		Endpoints: endpoints,
		HasQuorum: len(pods.Items) >= 3,
	}, nil
}

// ValidateClusterHealth checks if the cluster is healthy using ETCD client
func (d *Discovery) ValidateClusterHealth(ctx context.Context, cluster *ClusterInfo) error {
	if d.healthValidator == nil {
		// Fallback to quorum check if no health validator
		if !cluster.HasQuorum {
			return fmt.Errorf("cluster %s does not have quorum", cluster.Name)
		}
		return nil
	}

	// Use ETCD client to validate health
	return d.healthValidator.ValidateHealth(ctx, cluster.Endpoints)
}
