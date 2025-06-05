package cmd

import (
	"bytes"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"k8s.io/cli-runtime/pkg/genericclioptions"
)

func TestNewCmdDescheduler(t *testing.T) {
	tests := []struct {
		name        string
		args        []string
		wantErr     bool
		errMessage  string
		validateCmd func(*testing.T, *cobra.Command, *DeschedulerOptions)
	}{
		{
			name:       "no args and no labels",
			args:       []string{},
			wantErr:    true,
			errMessage: "must provide a node to be descheduled or labels to match nodes",
		},
		{
			name:       "invalid grace period",
			args:       []string{"node1", "--grace-period", "-1"},
			wantErr:    true,
			errMessage: "deletion grace period must be non-negative",
		},
		{
			name:       "invalid log level",
			args:       []string{"node1", "--log-level", "invalid"},
			wantErr:    true,
			errMessage: "invalid log level",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := NewCmdDescheduler()
			buf := new(bytes.Buffer)
			cmd.SetOut(buf)
			cmd.SetErr(buf)
			cmd.SetArgs(tt.args)

			err := cmd.Execute()
			if tt.wantErr {
				assert.Error(t, err)
				if tt.errMessage != "" {
					assert.Contains(t, err.Error(), tt.errMessage)
				}
				return
			}

			assert.NoError(t, err)
			if tt.validateCmd != nil {
				dOpts := &DeschedulerOptions{
					configFlags: genericclioptions.NewConfigFlags(true),
				}
				tt.validateCmd(t, cmd, dOpts)
			}
		})
	}
}

func TestDeschedulerOptions_Complete(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(*DeschedulerOptions)
		wantErr bool
	}{
		{
			name: "valid config",
			setup: func(opts *DeschedulerOptions) {
				opts.configFlags = genericclioptions.NewConfigFlags(true)
				opts.configFlags.KubeConfig = stringPtr("") // Use default kubeconfig
			},
			wantErr: false,
		},
		{
			name: "invalid kubeconfig",
			setup: func(opts *DeschedulerOptions) {
				opts.configFlags = genericclioptions.NewConfigFlags(true)
				opts.configFlags.KubeConfig = stringPtr("/nonexistent/config")
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := &DeschedulerOptions{}
			if tt.setup != nil {
				tt.setup(opts)
			}

			err := opts.Complete()
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			assert.NotNil(t, opts.client)
		})
	}
}

func stringPtr(s string) *string {
	return &s
}
