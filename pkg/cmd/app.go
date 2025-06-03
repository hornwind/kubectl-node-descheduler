package cmd

import (
	"context"
	"fmt"

	"github.com/hornwind/kubectl-node-descheduler/internal/descheduler"
	"github.com/hornwind/kubectl-node-descheduler/pkg/logging"
	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/kubernetes"
)

const helpText = `
Deschedule workloads from one or more nodes gracefully.

You can specify nodes either by name as arguments or by labels using the --label flag.
The tool will cordon the specified nodes and evict pods according to the configured parameters.

Examples:
  # Deschedule a single node
  kubectl node-descheduler deschedule node1

  # Deschedule multiple nodes
  kubectl node-descheduler deschedule node1 node2

  # Deschedule nodes by label
  kubectl node-descheduler deschedule --label node-role.kubernetes.io/worker=true

  # Skip certain namespaces
  kubectl node-descheduler deschedule node1 --skip-namespace kube-system --skip-namespace monitoring
`

// DeschedulerOptions holds the configuration options for the descheduler
type DeschedulerOptions struct {
	// args holds the raw command line arguments
	args []string

	// nodeNames is the list of node names to deschedule
	nodeNames []string

	// skipNamespaces is the list of namespaces to skip during descheduling
	skipNamespaces []string

	// nodeLabels is the list of labels to match nodes for descheduling
	nodeLabels []string

	// client is the Kubernetes client for interacting with the cluster
	client *kubernetes.Clientset

	// configFlags holds the Kubernetes client configuration flags
	configFlags *genericclioptions.ConfigFlags

	// deletionGracePeriod is the grace period in seconds for pod deletion
	deletionGracePeriod int64

	// logLevel sets the logging verbosity
	logLevel string

	// dryRun enables running without making actual changes
	dryRun bool
}

func NewDeschedulerOptions() *DeschedulerOptions {
	return &DeschedulerOptions{
		configFlags: genericclioptions.NewConfigFlags(true),
	}
}

// NewCmdDescheduler creates a new cobra command for the descheduler
func NewCmdDescheduler() *cobra.Command {
	o := NewDeschedulerOptions()

	cmd := &cobra.Command{
		Use:   "deschedule [node name] [flags]",
		Short: "Deschedule workloads from node gracefully",
		Long:  helpText,
		PreRunE: func(cmd *cobra.Command, args []string) error {
			o.args = args
			if len(o.args) < 1 && len(o.nodeLabels) == 0 {
				return fmt.Errorf("must provide a node to be descheduled or labels to match nodes")
			}
			o.nodeNames = o.args

			// Validate log level
			if err := logging.ValidateLogLevel(o.logLevel); err != nil {
				return fmt.Errorf("invalid log level: %w", err)
			}

			// Validate grace period
			if o.deletionGracePeriod < 0 {
				return fmt.Errorf("deletion grace period must be non-negative")
			}

			return nil
		},
		RunE: func(c *cobra.Command, args []string) error {
			if err := o.Complete(); err != nil {
				return err
			}

			if err := o.Run(); err != nil {
				return err
			}
			return nil
		},
	}

	cmd.Flags().StringArrayVarP(&o.nodeLabels, "label", "l", nil, "Match labels on nodes to be descheduled")
	cmd.Flags().StringArrayVarP(&o.skipNamespaces, "skip-namespace", "S", nil, "Namespaces to be skipped")
	cmd.Flags().Int64VarP(&o.deletionGracePeriod, "grace-period", "g", 30, "Deletion grace period in seconds")
	cmd.Flags().StringVarP(&o.logLevel, "log-level", "", "info", "Log level (debug, info, warn, error)")
	cmd.Flags().BoolVarP(&o.dryRun, "dry-run", "", false, "Dry run mode")
	cmd.PersistentFlags().BoolP("help", "h", false, "Show help for command")

	o.configFlags.AddFlags(cmd.Flags())

	return cmd
}

// Complete the setup of the command
func (d *DeschedulerOptions) Complete() error {
	restConfig, err := d.configFlags.ToRawKubeConfigLoader().ClientConfig()
	if err != nil {
		return fmt.Errorf("failed to get REST config: %w", err)
	}
	d.client, err = kubernetes.NewForConfig(restConfig)
	if err != nil {
		return fmt.Errorf("failed to create Kubernetes client: %w", err)
	}
	return nil
}

// Run executes the descheduler command
func (d *DeschedulerOptions) Run() error {
	ctx := context.Background()

	// // Create context with timeout
	// ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	// defer cancel()

	descheduler := descheduler.NewDescheduler(
		d.client,
		d.skipNamespaces,
		d.nodeNames,
		d.nodeLabels,
		d.logLevel,
		d.deletionGracePeriod,
		d.dryRun,
	)

	logger := logging.GetLogger()
	if err := logger.SetLogLevel(d.logLevel); err != nil {
		return fmt.Errorf("failed to set log level: %w", err)
	}

	logger.Infoln("Dry-run mode is", d.dryRun)
	logger.Infof("Deletion grace period is %ds", d.deletionGracePeriod)
	logger.Debugln("Received skip namespaces are", d.skipNamespaces)
	if len(d.nodeLabels) > 0 {
		logger.Infoln("Matching nodes with labels", d.nodeLabels)
	}
	if len(d.nodeNames) > 0 {
		logger.Infoln("Node names are", d.nodeNames)
	}

	return descheduler.Run(ctx)
}
