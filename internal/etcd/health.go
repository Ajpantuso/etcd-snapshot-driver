package etcd

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io/ioutil"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
	"go.uber.org/zap"
	"google.golang.org/grpc"
)

// HealthValidator performs client-based health checks on ETCD clusters
type HealthValidator struct {
	logger        *zap.SugaredLogger
	tlsConfig     *tls.Config
	healthTimeout time.Duration
}

// NewHealthValidator creates a new health validator
// tlsConfig can be nil for non-TLS connections
func NewHealthValidator(logger *zap.SugaredLogger, tlsConfig *tls.Config) *HealthValidator {
	return &HealthValidator{
		logger:        logger,
		tlsConfig:     tlsConfig,
		healthTimeout: 5 * time.Second,
	}
}

// ValidateHealth checks if an ETCD cluster is healthy
// Returns nil if healthy, error if unhealthy
func (hv *HealthValidator) ValidateHealth(ctx context.Context, endpoints []string) error {
	if len(endpoints) == 0 {
		return fmt.Errorf("no endpoints provided")
	}

	// Create ETCD client
	cfg := clientv3.Config{
		Endpoints:   endpoints,
		DialTimeout: hv.healthTimeout,
		TLS:         hv.tlsConfig,
		DialOptions: []grpc.DialOption{
			grpc.WithBlock(),
		},
	}

	client, err := clientv3.New(cfg)
	if err != nil {
		return fmt.Errorf("failed to create ETCD client: %w", err)
	}
	defer client.Close()

	// Get member list to validate cluster state
	memberCtx, cancel := context.WithTimeout(ctx, hv.healthTimeout)
	defer cancel()

	memberList, err := client.MemberList(memberCtx)
	if err != nil {
		return fmt.Errorf("failed to get member list: %w", err)
	}

	if len(memberList.Members) == 0 {
		return fmt.Errorf("cluster has no members")
	}

	// Calculate quorum requirement (majority of voting members)
	votingMembers := 0
	for _, member := range memberList.Members {
		if !member.IsLearner {
			votingMembers++
		}
	}

	if votingMembers == 0 {
		return fmt.Errorf("cluster has no voting members")
	}

	quorumRequired := (votingMembers / 2) + 1

	// Check each member's health
	healthyMembers := 0
	hasLeader := false

	for _, member := range memberList.Members {
		if member.IsLearner {
			continue // Skip learner members in quorum calculation
		}

		// Check if member is the leader
		if memberList.Header.GetMemberId() == member.ID {
			hasLeader = true
		}

		// Try to ping the member
		memberEndpoint := member.ClientURLs[0] // Use first client URL
		memberCtx, cancel := context.WithTimeout(ctx, hv.healthTimeout)

		memberCfg := clientv3.Config{
			Endpoints:   []string{memberEndpoint},
			DialTimeout: hv.healthTimeout,
			TLS:         hv.tlsConfig,
			DialOptions: []grpc.DialOption{
				grpc.WithBlock(),
			},
		}

		memberClient, err := clientv3.New(memberCfg)
		cancel()

		if err != nil {
			hv.logger.Debugw("Member unhealthy",
				"member_id", fmt.Sprintf("%x", member.ID),
				"endpoint", memberEndpoint,
				"error", err,
			)
			memberClient.Close()
			continue
		}

		memberCtx, cancel = context.WithTimeout(ctx, hv.healthTimeout)
		_, err = memberClient.Get(memberCtx, "health")
		cancel()
		memberClient.Close()

		if err == nil {
			healthyMembers++
			hv.logger.Debugw("Member healthy",
				"member_id", fmt.Sprintf("%x", member.ID),
				"endpoint", memberEndpoint,
			)
		}
	}

	// Validate quorum
	if healthyMembers < quorumRequired {
		return fmt.Errorf("insufficient healthy members: %d/%d required %d",
			healthyMembers, votingMembers, quorumRequired)
	}

	// Validate leader exists
	if !hasLeader {
		return fmt.Errorf("cluster has no leader")
	}

	hv.logger.Debugw("Cluster health validated",
		"healthy_members", healthyMembers,
		"voting_members", votingMembers,
		"quorum_required", quorumRequired,
		"has_leader", hasLeader,
	)

	return nil
}

// LoadTLSConfig loads TLS certificates from files
func LoadTLSConfig(certPath, keyPath, caPath string) (*tls.Config, error) {
	// Load client certificate and key
	cert, err := tls.LoadX509KeyPair(certPath, keyPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load client certificates: %w", err)
	}

	// Load CA certificate
	caCert, err := ioutil.ReadFile(caPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read CA certificate: %w", err)
	}

	caCertPool := x509.NewCertPool()
	if !caCertPool.AppendCertsFromPEM(caCert) {
		return nil, fmt.Errorf("failed to parse CA certificate")
	}

	// Create TLS configuration
	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
		RootCAs:      caCertPool,
	}

	return tlsConfig, nil
}
