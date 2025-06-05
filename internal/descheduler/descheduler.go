package descheduler

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/hornwind/kubectl-node-descheduler/pkg/logging"
	apiv1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

const (
	evictionInterval = 30 * time.Second
)

type Descheduler struct {
	// skipLabels          []string
	skipNamespaces      []string
	nodes               []string
	nodeLabels          []string
	logLevel            string
	client              kubernetes.Interface
	logger              *logging.Logger
	cancel              context.CancelFunc
	deletionGracePeriod int64
	evictJobs           bool
	dryRun              bool
}

func NewDescheduler(client kubernetes.Interface, skipNamespaces, nodes, nodeLabels []string, logLevel string, deletionGracePeriod int64, evictJobs, dryRun bool) *Descheduler {
	logger := logging.GetLogger()
	logger.SetLogLevel(logLevel)

	return &Descheduler{
		// skipLabels:     skipLabels,
		skipNamespaces:      skipNamespaces,
		nodes:               nodes,
		nodeLabels:          nodeLabels,
		logLevel:            logLevel,
		client:              client,
		logger:              logger,
		deletionGracePeriod: deletionGracePeriod,
		evictJobs:           evictJobs,
		dryRun:              dryRun,
	}
}

func (d *Descheduler) Run(ctx context.Context) error {
	ctx, d.cancel = context.WithCancel(ctx)

	err := d.combineNodes(ctx)
	if err != nil {
		return err
	}

	err = d.CordonNodes(ctx, d.nodes)
	if err != nil {
		return err
	}

	d.updateSkipNamespaces()
	d.logger.Infoln("Skipped namespaces:", d.skipNamespaces)

	ticker := time.NewTicker(evictionInterval)
	defer ticker.Stop()

	firstTick := false

	// create a wrapper of the ticker that ticks the first time immediately
	tickerChan := func() <-chan time.Time {
		if !firstTick {
			firstTick = true
			c := make(chan time.Time, 1)
			c <- time.Now()
			return c
		}
		return ticker.C
	}

	for {
		select {
		case <-tickerChan():
			if err := d.drainIteration(ctx); err != nil {
				d.logger.Errorln("Error draining:", err)
				d.cancel()
				return err
			}
		case <-ctx.Done():
			d.logger.Infoln("Descheduler stopped")
			return nil
		}
	}
}

// combine nodes by labels and names
func (d *Descheduler) combineNodes(ctx context.Context) error {
	if len(d.nodeLabels) == 0 && len(d.nodes) == 0 {
		return fmt.Errorf("must provide a node to be descheduled or labels to match nodes")
	} else if len(d.nodeLabels) == 0 {
		return nil
	}

	labelsConcat := strings.Join(d.nodeLabels, ",")
	d.logger.Debugln("Matching nodes with LabelSelector", labelsConcat)
	nodes, err := d.client.CoreV1().Nodes().List(ctx, metav1.ListOptions{
		LabelSelector: labelsConcat,
	})
	if err != nil {
		return err
	}

	for _, node := range nodes.Items {
		if slices.Contains(d.nodes, node.Name) {
			continue
		}
		d.nodes = append(d.nodes, node.Name)
	}
	d.logger.Infoln("Nodes selected to deschedule:", d.nodes)
	return nil
}

func (d *Descheduler) drainIteration(ctx context.Context) error {
	pods, err := d.getPods(ctx)
	if err != nil {
		return err
	}

	err = d.scheduleEvictions(ctx, pods)
	if err != nil {
		return err
	}

	select {
	case <-ctx.Done():
		return nil
	default:
		if err := d.showVolumeMounts(ctx); err != nil {
			d.logger.Errorln("Error showing mounted volumes:", err)
			return nil
		}
	}

	return nil
}

// avoid typos and mistakes in the skipNamespaces flag
func (d *Descheduler) updateSkipNamespaces() {
	d.logger.Debugln("skipNamespaces before parsing:", d.skipNamespaces)
	ns := strings.Join(d.skipNamespaces, ",")
	d.logger.Debugln("Namespaces after joining:", ns)
	s := strings.Split(ns, ",")
	d.logger.Debugln("Namespaces after splitting:", s)
	for i := range s {
		s[i] = strings.TrimSpace(s[i])
	}
	d.logger.Debugln("Namespaces after trimming and formatting:", s)
	d.skipNamespaces = s
}

func (d *Descheduler) getPods(ctx context.Context) (*apiv1.PodList, error) {
	podList := &apiv1.PodList{}

	for _, node := range d.nodes {
		d.logger.Debugln("Gathering pods on", node)
		p, err := d.client.CoreV1().Pods(metav1.NamespaceAll).List(ctx, metav1.ListOptions{
			FieldSelector: fmt.Sprintf("spec.nodeName=%s", node),
		})
		if err != nil {
			return nil, err
		}
		podList.Items = append(podList.Items, p.Items...)
	}

	// filter pods
	result := &apiv1.PodList{}
	for _, pod := range podList.Items {
		if pod.Status.Phase != apiv1.PodRunning {
			continue
		}
		if slices.Contains(d.skipNamespaces, pod.Namespace) {
			continue
		}
		// Skip pods without owner references (?)
		if len(pod.OwnerReferences) == 0 {
			continue
		}
		for _, ownerRef := range pod.OwnerReferences {
			if ownerRef.Kind == "ReplicaSet" || ownerRef.Kind == "StatefulSet" {
				result.Items = append(result.Items, pod)
				continue
			}
			if d.evictJobs && ownerRef.Kind == "Job" {
				result.Items = append(result.Items, pod)
			}
		}
	}

	d.logger.Infoln("Pods for eviction:", len(result.Items))
	if len(result.Items) == 0 {
		d.logger.Infoln("No pods to evict, stopping descheduler")
		d.cancel()
	}

	// filter pods that are not in the skipLabels
	// skippedPods = &apiv1.PodList{}
	// for _, pod := range runningPods.Items {
	// 	if !slices.Contains(d.skipLabels, pod.Labels[d.skipLabels]) {
	// 		skippedPods.Items = append(skippedPods.Items, pod)
	// 	}
	// }

	return result, nil
}

// pod eviction func
func (d *Descheduler) evictPod(ctx context.Context, pod *apiv1.Pod) error {
	eviction := &policyv1.Eviction{
		ObjectMeta: metav1.ObjectMeta{
			Name:      pod.Name,
			Namespace: pod.Namespace,
		},
		DeleteOptions: &metav1.DeleteOptions{
			GracePeriodSeconds: &d.deletionGracePeriod,
		},
	}

	if d.dryRun {
		eviction.DeleteOptions.DryRun = []string{"All"}
	}
	d.logger.Infoln("Evicting pod", pod.Name, "in ns", pod.Namespace)
	return d.client.CoreV1().Pods(pod.Namespace).EvictV1(ctx, eviction)
}

// Show mounted volumes by node
func (d *Descheduler) showVolumeMounts(ctx context.Context) error {
	volumes, err := d.client.StorageV1().VolumeAttachments().List(ctx, metav1.ListOptions{})
	if err != nil {
		return err
	}

	for _, node := range d.nodes {
		d.logger.Debugln("Mounted volumes for node", node)
		for _, volume := range volumes.Items {
			d.logger.Debugln("Check VolumeAttachment", volume.Name)
			if volume.Spec.NodeName == node && volume.Status.Attached {
				d.logger.Infoln("Volume", volume.Name, "attached to", node)
			}
		}
	}
	return nil
}

// schedule evictions
func (d *Descheduler) scheduleEvictions(ctx context.Context, pods *apiv1.PodList) error {
	ids := make(map[string]struct{})

	for _, pod := range pods.Items {
		for _, ownerRef := range pod.OwnerReferences {
			if ownerRef.Kind == "ReplicaSet" {
				_, ok := ids[string(ownerRef.UID)]
				if ok {
					d.logger.Infoln("Skipping pod", pod.Name, "in ns", pod.Namespace, "already seen and eviction postponed")
					continue
				}

				rs, err := d.client.AppsV1().ReplicaSets(pod.Namespace).Get(ctx, ownerRef.Name, metav1.GetOptions{})
				if err != nil {
					d.logger.Errorln("Error getting ReplicaSet", ownerRef.Name, "in ns", pod.Namespace, ":", err)
					continue
				}

				if rs.Status.Replicas == 0 && rs.Status.ReadyReplicas == 0 && rs.Status.AvailableReplicas == 0 {
					d.logger.Infoln("ReplicaSet", ownerRef.Name, "in ns", pod.Namespace, "has no replicas, skipping")
					continue
				}

				if rs.Status.ReadyReplicas < *rs.Spec.Replicas {
					d.logger.Infoln(
						"ReplicaSet", ownerRef.Name,
						"in ns", pod.Namespace,
						"has", rs.Status.ReadyReplicas,
						"ready replicas less than", *rs.Spec.Replicas,
						"desired replicas, skipping",
					)
					continue
				}

				err = d.evictPod(ctx, &pod)
				if err != nil {
					d.logger.Errorln("Error evicting pod", pod.Name, "in ns", pod.Namespace, ":", err)
				}
				ids[string(ownerRef.UID)] = struct{}{}
			}

			if ownerRef.Kind == "StatefulSet" {
				_, ok := ids[string(ownerRef.UID)]
				if ok {
					d.logger.Infoln("Skipping pod", pod.Name, "in ns", pod.Namespace, "already seen and eviction postponed")
					continue
				}

				sts, err := d.client.AppsV1().StatefulSets(pod.Namespace).Get(ctx, ownerRef.Name, metav1.GetOptions{})
				if err != nil {
					d.logger.Errorln("Error getting StatefulSet", ownerRef.Name, "in ns", pod.Namespace, ":", err)
					continue
				}

				if sts.Status.Replicas == 0 && sts.Status.ReadyReplicas == 0 && sts.Status.AvailableReplicas == 0 {
					d.logger.Infoln("StatefulSet", ownerRef.Name, "in ns", pod.Namespace, "has no replicas, skipping")
					continue
				}

				if sts.Status.ReadyReplicas < *sts.Spec.Replicas {
					d.logger.Infoln(
						"StatefulSet", ownerRef.Name,
						"in ns", pod.Namespace,
						"has", sts.Status.ReadyReplicas,
						"ready replicas less than", *sts.Spec.Replicas,
						"desired replicas, skipping",
					)
					continue
				}

				err = d.evictPod(ctx, &pod)
				if err != nil {
					d.logger.Errorln("Error evicting pod", pod.Name, "in ns", pod.Namespace, ":", err)
				}
				ids[string(ownerRef.UID)] = struct{}{}
			}
		}
	}
	return nil
}
