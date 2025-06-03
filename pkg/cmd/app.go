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

type DeschedulerOptions struct {
	args                []string
	nodeNames           []string
	skipNamespaces      []string
	nodeLabels          []string
	client              *kubernetes.Clientset
	configFlags         *genericclioptions.ConfigFlags
	deletionGracePeriod int64
	logLevel            string
	dryRun              bool
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
		Short: "Deschedule worcloads from node gracefully",
		PreRunE: func(cmd *cobra.Command, args []string) error {
			o.args = args
			if len(o.args) < 1 {
				if len(o.nodeLabels) == 0 {
					return fmt.Errorf("must provide a node to be descheduled or labels to match nodes")
				}
				return nil
			}
			o.nodeNames = o.args
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

	cmd.Flags().StringArrayVarP(&o.nodeLabels, "label", "", nil, "Match labels on nodes to be descheduled")
	cmd.Flags().StringArrayVarP(&o.skipNamespaces, "skip-namespace", "", nil, "Namespaces to be skipped")
	cmd.Flags().Int64VarP(&o.deletionGracePeriod, "deletion-grace-period", "", 30, "Deletion grace period in seconds")
	cmd.Flags().StringVarP(&o.logLevel, "log-level", "", "info", "Log level")
	cmd.Flags().BoolVarP(&o.dryRun, "dry-run", "", false, "Dry run mode")
	cmd.PersistentFlags().BoolP("help", "", false, "Show help for command")

	o.configFlags.AddFlags(cmd.Flags())

	return cmd
}

func (d *DeschedulerOptions) Complete() error {
	restConfig, err := d.configFlags.ToRawKubeConfigLoader().ClientConfig()
	if err != nil {
		return err
	}
	d.client, err = kubernetes.NewForConfig(restConfig)
	if err != nil {
		return err
	}
	return nil
}

func (d *DeschedulerOptions) Run() error {
	ctx := context.Background()
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
	logger.SetLogLevel(d.logLevel)
	logger.Infoln("Dry-run mode is", d.dryRun)
	logger.Infof("Deletion grace period is %ds", d.deletionGracePeriod)
	logger.Infoln("Skip namespaces are", d.skipNamespaces)
	if len(d.nodeLabels) > 0 {
		logger.Infoln("Matching nodes with labels", d.nodeLabels)
	}
	if len(d.nodeNames) > 0 {
		logger.Infoln("Node names are", d.nodeNames)
	}

	return descheduler.Run(ctx)
}
