package descheduler

import (
	"context"
	"fmt"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

type deploymentDescheduler struct {
	deployments []string
	Descheduler
}

func (d *deploymentDescheduler) restartDeployment(ctx context.Context, rs *appsv1.ReplicaSet) error {
	ownerDeployment := &v1.Deployment{}
	rolloutPatchTemplate := `{
		"spec": {
			"template": {
				"metadata": {
					"annotations": {
						"kubectl.kubernetes.io/restartedAt": "%s"
					}
				}
			}
		}
	}`

	patchOpts := metav1.PatchOptions{}
	if d.dryRun {
		patchOpts.DryRun = []string{"All"}
	}
	for _, owner := range rs.OwnerReferences {
		if owner.Kind == "Deployment" && owner.Name != "" {
			od, err := d.client.AppsV1().Deployments(rs.Namespace).Get(ctx, owner.Name, metav1.GetOptions{})
			if err != nil {
				return err
			}
			ownerDeployment = od
		}
	}
	d.logger.Infoln("Deployment ROLLOUT", ownerDeployment.Name, "in ns", ownerDeployment.Namespace)
	_, err := d.client.AppsV1().Deployments(ownerDeployment.Namespace).Patch(
		ctx,
		ownerDeployment.Name,
		types.StrategicMergePatchType,
		[]byte(fmt.Sprintf(
			rolloutPatchTemplate,
			time.Now().Format(time.RFC3339),
		)),
		patchOpts,
	)
	if err != nil {
		return err
	}
	return nil
}
