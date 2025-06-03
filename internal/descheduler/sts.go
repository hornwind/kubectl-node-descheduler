package descheduler

import (
	"context"
	"fmt"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

type stsDescheduler struct {
	statefulsets []string
	Descheduler
}

func (d *stsDescheduler) restartStatefulset(ctx context.Context, sts *appsv1.StatefulSet) error {
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
	d.logger.Infoln("StatefulSet ROLLOUT", sts.Name, "in ns", sts.Namespace)
	_, err := d.client.AppsV1().StatefulSets(sts.Namespace).Patch(
		ctx,
		sts.Name,
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
