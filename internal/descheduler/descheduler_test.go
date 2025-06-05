package descheduler

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	appsv1 "k8s.io/api/apps/v1"
	apiv1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func createTestNode(name string, labels map[string]string) *apiv1.Node {
	return &apiv1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:   name,
			Labels: labels,
		},
	}
}

func createTestPod(name, namespace, nodeName string, phase apiv1.PodPhase, ownerKind string) *apiv1.Pod {
	pod := &apiv1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels: map[string]string{
				"app": "test",
			},
		},
		Spec: apiv1.PodSpec{
			NodeName: nodeName,
		},
		Status: apiv1.PodStatus{
			Phase: phase,
		},
	}

	if ownerKind != "" {
		pod.OwnerReferences = []metav1.OwnerReference{
			{
				APIVersion: "apps/v1",
				Kind:       ownerKind,
				Name:       "test-owner",
				UID:        "test-uid",
			},
		}
	}

	return pod
}

func createTestReplicaSet(name, namespace string) *appsv1.ReplicaSet {
	return &appsv1.ReplicaSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			UID:       "test-uid",
		},
		Spec: appsv1.ReplicaSetSpec{
			Replicas: int32Ptr(2),
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": "test",
				},
			},
			Template: apiv1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app": "test",
					},
				},
				Spec: apiv1.PodSpec{
					Containers: []apiv1.Container{
						{
							Name:  "test",
							Image: "test",
						},
					},
				},
			},
		},
		Status: appsv1.ReplicaSetStatus{
			Replicas:      2,
			ReadyReplicas: 2,
		},
	}
}

func int32Ptr(i int32) *int32 {
	return &i
}

func TestDescheduler_CombineNodes(t *testing.T) {
	tests := []struct {
		name          string
		nodes         []string
		nodeLabels    []string
		existingNodes []*apiv1.Node
		expectedNodes []string
		expectedError bool
		errorContains string
	}{
		{
			name:          "no nodes or labels",
			expectedError: true,
			errorContains: "must provide a node to be descheduled or labels to match nodes",
		},
		{
			name:          "nodes only",
			nodes:         []string{"node1", "node2"},
			expectedNodes: []string{"node1", "node2"},
		},
		{
			name:       "labels only",
			nodeLabels: []string{"role=worker"},
			existingNodes: []*apiv1.Node{
				createTestNode("node1", map[string]string{"role": "worker"}),
				createTestNode("node2", map[string]string{"role": "worker"}),
				createTestNode("node3", map[string]string{"role": "master"}),
			},
			expectedNodes: []string{"node1", "node2"},
		},
		{
			name:       "combine nodes and labels",
			nodes:      []string{"node1"},
			nodeLabels: []string{"role=worker"},
			existingNodes: []*apiv1.Node{
				createTestNode("node1", map[string]string{"role": "worker"}),
				createTestNode("node2", map[string]string{"role": "worker"}),
			},
			expectedNodes: []string{"node1", "node2"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clientset := fake.NewSimpleClientset()

			// Create test nodes in the fake clientset
			for _, node := range tt.existingNodes {
				_, err := clientset.CoreV1().Nodes().Create(context.TODO(), node, metav1.CreateOptions{})
				assert.NoError(t, err)
			}

			d := NewDescheduler(clientset, nil, tt.nodes, tt.nodeLabels, "info", 30, false, false)
			err := d.combineNodes(context.TODO())

			if tt.expectedError {
				assert.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
				return
			}

			assert.NoError(t, err)
			assert.ElementsMatch(t, tt.expectedNodes, d.nodes)
		})
	}
}

func TestDescheduler_GetPods(t *testing.T) {
	tests := []struct {
		name           string
		nodes          []string
		skipNamespaces []string
		existingPods   []*apiv1.Pod
		expectedPods   int
	}{
		{
			// https://github.com/kubernetes/client-go/issues/326
			name:  "single node with running pods",
			nodes: []string{"node1"},
			existingPods: []*apiv1.Pod{
				createTestPod("pod1", "default", "node1", apiv1.PodRunning, "ReplicaSet"),
				createTestPod("pod2", "default", "node1", apiv1.PodRunning, "ReplicaSet"),
				createTestPod("pod4", "default", "node1", apiv1.PodRunning, ""), // Pod without owner reference
			},
			expectedPods: 2,
		},
		{
			name:           "skip namespace",
			nodes:          []string{"node1"},
			skipNamespaces: []string{"kube-system"},
			existingPods: []*apiv1.Pod{
				createTestPod("pod1", "default", "node1", apiv1.PodRunning, "ReplicaSet"),
				createTestPod("pod2", "kube-system", "node1", apiv1.PodRunning, "ReplicaSet"),
			},
			expectedPods: 1,
		},
		{
			name:  "filter daemonsets",
			nodes: []string{"node1"},
			existingPods: []*apiv1.Pod{
				createTestPod("pod1", "default", "node1", apiv1.PodRunning, "ReplicaSet"),
				createTestPod("pod2", "default", "node1", apiv1.PodRunning, "DaemonSet"),
			},
			expectedPods: 1,
		},
		{
			name:  "filter non-running pods",
			nodes: []string{"node1"},
			existingPods: []*apiv1.Pod{
				createTestPod("pod1", "default", "node1", apiv1.PodRunning, "ReplicaSet"),
				createTestPod("pod2", "default", "node1", apiv1.PodPending, "ReplicaSet"),
				createTestPod("pod3", "default", "node1", apiv1.PodSucceeded, "ReplicaSet"),
			},
			expectedPods: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clientset := fake.NewClientset()

			// Create test nodes
			testNodes := []string{"node1", "node2"}
			for _, node := range testNodes {
				n := createTestNode(node, nil)
				_, err := clientset.CoreV1().Nodes().Create(context.TODO(), n, metav1.CreateOptions{})
				assert.NoError(t, err)
			}

			// Create ReplicaSet first
			rs := createTestReplicaSet("test-owner", "default")
			_, err := clientset.AppsV1().ReplicaSets("default").Create(context.TODO(), rs, metav1.CreateOptions{})
			assert.NoError(t, err)

			// Create test pods
			for _, pod := range tt.existingPods {
				_, err := clientset.CoreV1().Pods(pod.Namespace).Create(context.TODO(), pod, metav1.CreateOptions{})
				assert.NoError(t, err)
			}
			d := NewDescheduler(clientset, tt.skipNamespaces, tt.nodes, nil, "info", 30, false, false)
			d.updateSkipNamespaces()

			pods, err := d.getPods(context.TODO())
			assert.NoError(t, err)

			assert.Equal(t, tt.expectedPods, len(pods.Items))
		})
	}
}

func TestDescheduler_Run(t *testing.T) {
	tests := []struct {
		name           string
		nodes          []string
		skipNamespaces []string
		existingPods   []*apiv1.Pod
		dryRun         bool
		timeout        time.Duration
		checkFunc      func(*testing.T, *fake.Clientset)
	}{
		{
			name:    "cordon node",
			nodes:   []string{"node1"},
			dryRun:  false,
			timeout: 1 * time.Second,
			checkFunc: func(t *testing.T, clientset *fake.Clientset) {
				node, err := clientset.CoreV1().Nodes().Get(context.TODO(), "node1", metav1.GetOptions{})
				assert.NoError(t, err)
				assert.True(t, node.Spec.Unschedulable)
			},
		},
		// will not work because pod eviction is not implemented in fake clientset
		// {
		// 	name:   "evict pods",
		// 	nodes:  []string{"node1"},
		// 	dryRun: false,
		// 	existingPods: []*apiv1.Pod{
		// 		createTestPod("pod1", "default", "node1", apiv1.PodRunning, "ReplicaSet"),
		// 		createTestPod("pod2", "default", "node1", apiv1.PodRunning, "ReplicaSet"),
		// 	},
		// 	timeout: 60 * time.Second,
		// 	checkFunc: func(t *testing.T, clientset *fake.Clientset) {
		// 		// Wait for eviction to complete
		// 		time.Sleep(1 * time.Second)
		// 		pods, err := clientset.CoreV1().Pods("default").List(context.TODO(), metav1.ListOptions{})
		// 		assert.NoError(t, err)
		// 		assert.Empty(t, pods.Items)
		// 	},
		// },
		{
			name:   "dry run mode",
			nodes:  []string{"node1"},
			dryRun: true,
			existingPods: []*apiv1.Pod{
				createTestPod("pod1", "default", "node1", apiv1.PodRunning, "ReplicaSet"),
			},
			timeout: 1 * time.Second,
			checkFunc: func(t *testing.T, clientset *fake.Clientset) {
				pods, err := clientset.CoreV1().Pods("default").List(context.TODO(), metav1.ListOptions{})
				assert.NoError(t, err)
				assert.Len(t, pods.Items, 1)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clientset := fake.NewSimpleClientset()

			// Create test node
			node := createTestNode("node1", nil)
			_, err := clientset.CoreV1().Nodes().Create(context.TODO(), node, metav1.CreateOptions{})
			assert.NoError(t, err)

			// Create ReplicaSet first
			rs := createTestReplicaSet("test-owner", "default")
			_, err = clientset.AppsV1().ReplicaSets("default").Create(context.TODO(), rs, metav1.CreateOptions{})
			assert.NoError(t, err)

			// Create test pods
			for _, pod := range tt.existingPods {
				_, err := clientset.CoreV1().Pods(pod.Namespace).Create(context.TODO(), pod, metav1.CreateOptions{})
				assert.NoError(t, err)
			}

			d := NewDescheduler(clientset, tt.skipNamespaces, tt.nodes, nil, "info", 30, false, tt.dryRun)

			ctx, cancel := context.WithTimeout(context.Background(), tt.timeout)
			defer cancel()

			err = d.Run(ctx)
			assert.NoError(t, err)

			if tt.checkFunc != nil {
				tt.checkFunc(t, clientset)
			}
		})
	}
}

func TestDescheduler_UpdateSkipNamespaces(t *testing.T) {
	tests := []struct {
		name           string
		skipNamespaces []string
		expected       []string
	}{
		{
			name:           "trim spaces",
			skipNamespaces: []string{" kube-system ", "monitoring "},
			expected:       []string{"kube-system", "monitoring"},
		},
		{
			name:           "handle empty strings",
			skipNamespaces: []string{"", "kube-system", "  "},
			expected:       []string{"", "kube-system", ""},
		},
		{
			name:           "handle commas",
			skipNamespaces: []string{"kube-system,monitoring"},
			expected:       []string{"kube-system", "monitoring"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := NewDescheduler(nil, tt.skipNamespaces, nil, nil, "info", 30, false, false)
			d.updateSkipNamespaces()
			assert.ElementsMatch(t, tt.expected, d.skipNamespaces)
		})
	}
}
