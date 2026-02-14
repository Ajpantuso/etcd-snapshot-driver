package main

import (
	"os"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
)

func TestFlagDefaults(t *testing.T) {
	// Create a command and register flags
	cmd := &cobra.Command{}
	v, err := SetupViper(cmd)
	if err != nil {
		t.Fatalf("Failed to setup viper: %v", err)
	}

	// Define test cases for each flag and its expected default value
	tests := []struct {
		name    string
		flag    string
		want    interface{}
		getFunc func(*viper.Viper, string) interface{}
	}{
		{
			name:    "csi-endpoint default",
			flag:    "csi-endpoint",
			want:    "unix:///var/lib/kubelet/plugins/etcd-snapshot-driver/csi.sock",
			getFunc: func(vv *viper.Viper, k string) interface{} { return vv.GetString(k) },
		},
		{
			name:    "driver-name default",
			flag:    "driver-name",
			want:    "etcd-snapshot-driver",
			getFunc: func(vv *viper.Viper, k string) interface{} { return vv.GetString(k) },
		},
		{
			name:    "etcd-namespace default",
			flag:    "etcd-namespace",
			want:    "etcd",
			getFunc: func(vv *viper.Viper, k string) interface{} { return vv.GetString(k) },
		},
		{
			name:    "cluster-label-key default",
			flag:    "cluster-label-key",
			want:    "etcd.io/cluster",
			getFunc: func(vv *viper.Viper, k string) interface{} { return vv.GetString(k) },
		},
		{
			name:    "snapshot-timeout default",
			flag:    "snapshot-timeout",
			want:    5 * time.Minute,
			getFunc: func(vv *viper.Viper, k string) interface{} { return vv.GetDuration(k) },
		},
		{
			name:    "job-backoff-limit default",
			flag:    "job-backoff-limit",
			want:    int32(3),
			getFunc: func(vv *viper.Viper, k string) interface{} { return vv.GetInt32(k) },
		},
		{
			name:    "job-active-deadline default",
			flag:    "job-active-deadline",
			want:    int64(600),
			getFunc: func(vv *viper.Viper, k string) interface{} { return vv.GetInt64(k) },
		},
		{
			name:    "etcd-tls-enabled default",
			flag:    "etcd-tls-enabled",
			want:    true,
			getFunc: func(vv *viper.Viper, k string) interface{} { return vv.GetBool(k) },
		},
		{
			name:    "leader-elect default",
			flag:    "leader-elect",
			want:    false,
			getFunc: func(vv *viper.Viper, k string) interface{} { return vv.GetBool(k) },
		},
		{
			name:    "log-level default",
			flag:    "log-level",
			want:    "info",
			getFunc: func(vv *viper.Viper, k string) interface{} { return vv.GetString(k) },
		},
		{
			name:    "log-format default",
			flag:    "log-format",
			want:    "json",
			getFunc: func(vv *viper.Viper, k string) interface{} { return vv.GetString(k) },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.getFunc(v, tt.flag)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestEnvironmentVariableBinding(t *testing.T) {
	cmd := &cobra.Command{}
	v, err := SetupViper(cmd)
	if err != nil {
		t.Fatalf("Failed to setup viper: %v", err)
	}

	// Setup environment variable binding once
	LoadOptions(v)

	tests := []struct {
		name    string
		envVar  string
		envVal  string
		flag    string
		want    interface{}
		getFunc func(*viper.Viper, string) interface{}
	}{
		{
			name:    "CLUSTER_LABEL_KEY binding",
			envVar:  "CLUSTER_LABEL_KEY",
			envVal:  "my-custom-label",
			flag:    "cluster-label-key",
			want:    "my-custom-label",
			getFunc: func(vv *viper.Viper, k string) interface{} { return vv.GetString(k) },
		},
		{
			name:    "CSI_ENDPOINT binding",
			envVar:  "CSI_ENDPOINT",
			envVal:  "/tmp/custom.sock",
			flag:    "csi-endpoint",
			want:    "/tmp/custom.sock",
			getFunc: func(vv *viper.Viper, k string) interface{} { return vv.GetString(k) },
		},
		{
			name:    "ETCD_NAMESPACE binding",
			envVar:  "ETCD_NAMESPACE",
			envVal:  "custom-ns",
			flag:    "etcd-namespace",
			want:    "custom-ns",
			getFunc: func(vv *viper.Viper, k string) interface{} { return vv.GetString(k) },
		},
		{
			name:    "LOG_LEVEL binding",
			envVar:  "LOG_LEVEL",
			envVal:  "debug",
			flag:    "log-level",
			want:    "debug",
			getFunc: func(vv *viper.Viper, k string) interface{} { return vv.GetString(k) },
		},
		{
			name:    "ETCD_TLS_ENABLED binding",
			envVar:  "ETCD_TLS_ENABLED",
			envVal:  "false",
			flag:    "etcd-tls-enabled",
			want:    false,
			getFunc: func(vv *viper.Viper, k string) interface{} { return vv.GetBool(k) },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup environment variable
			os.Setenv(tt.envVar, tt.envVal)
			defer os.Unsetenv(tt.envVar)

			// Test
			got := tt.getFunc(v, tt.flag)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestParseLogLevel(t *testing.T) {
	tests := []struct {
		level    string
		expected string
	}{
		{"debug", "debug"},
		{"info", "info"},
		{"warn", "warn"},
		{"error", "error"},
		{"unknown", "info"}, // defaults to info
	}

	for _, tt := range tests {
		t.Run(tt.level, func(t *testing.T) {
			result := parseLogLevel(tt.level)
			expected := parseLogLevel(tt.expected)
			assert.Equal(t, expected, result)
		})
	}
}
