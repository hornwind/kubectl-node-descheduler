package descheduler

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (d *Descheduler) cordonNode(ctx context.Context, nodeName string) error {
	node, err := d.client.CoreV1().Nodes().Get(ctx, nodeName, metav1.GetOptions{})
	if err != nil {
		return err
	}

	node.Spec.Unschedulable = true
	updateOpts := metav1.UpdateOptions{}
	if d.dryRun {
		updateOpts.DryRun = []string{"All"}
	}
	_, err = d.client.CoreV1().Nodes().Update(ctx, node, updateOpts)
	if err != nil {
		return err
	}

	return nil
}

func (d *Descheduler) CordonNodes(ctx context.Context, nodes []string) error {
	for _, node := range nodes {
		err := d.cordonNode(ctx, node)
		if err != nil {
			return err
		}
	}
	return nil
}
