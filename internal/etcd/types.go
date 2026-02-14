package etcd

import "time"

type ClusterInfo struct {
	Name         string
	Namespace    string
	Endpoints    []string
	Members      []MemberInfo
	Version      string
	HasQuorum    bool
}

type MemberInfo struct {
	Name       string
	ID         string
	ClientURLs []string
	PeerURLs   []string
}

type DiscoveryResult struct {
	ClusterInfo *ClusterInfo
	Source      string // label, annotation, statefulset
	Timestamp   time.Time
}
