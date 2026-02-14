package etcd_test

import (
	"context"
	"testing"

	"github.com/Ajpantuso/etcd-snapshot-driver/internal/etcd"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
	"go.uber.org/zap"
)

func TestDiscoverClusterByLabel(t *testing.T) {
	logger, _ := zap.NewProduction()
	defer logger.Sync()

	k8sClient := fake.NewSimpleClientset()

	// Create test PVC with cluster label
	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "etcd-pvc",
			Namespace: "default",
			Labels: map[string]string{
				"etcd.io/cluster": "test-cluster",
			},
		},
	}
	k8sClient.CoreV1().PersistentVolumeClaims("default").Create(context.Background(), pvc, metav1.CreateOptions{})

	// Create test ETCD pods
	for i := 0; i < 3; i++ {
		pod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "etcd-" + string(rune(i)),
				Namespace: "default",
				Labels: map[string]string{
					"etcd.io/cluster": "test-cluster",
				},
			},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{
					{
						Name:  "etcd",
						Image: "quay.io/coreos/etcd:v3.5.0",
					},
				},
			},
		}
		k8sClient.CoreV1().Pods("default").Create(context.Background(), pod, metav1.CreateOptions{})
	}

	discovery := etcd.NewDiscovery(k8sClient, logger.Sugar(), "etcd.io/cluster")

	clusterInfo, err := discovery.DiscoverCluster(context.Background(), "default", "etcd-pvc")
	if err != nil {
		t.Fatalf("DiscoverCluster failed: %v", err)
	}

	if clusterInfo.Name != "test-cluster" {
		t.Errorf("Expected cluster name 'test-cluster', got '%s'", clusterInfo.Name)
	}

	if len(clusterInfo.Endpoints) != 3 {
		t.Errorf("Expected 3 endpoints, got %d", len(clusterInfo.Endpoints))
	}

	if !clusterInfo.HasQuorum {
		t.Error("Expected cluster to have quorum with 3 members")
	}
}
