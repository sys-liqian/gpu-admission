/*
Copyright 2021.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/
package plugin

import (
	"context"
	"encoding/json"
	"fmt"
	"gpu-admission/pkg/algorithm"
	"gpu-admission/pkg/device"
	"gpu-admission/pkg/util"
	"strings"
	"time"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8stypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"
	"k8s.io/kubernetes/pkg/scheduler/framework"
)

const (
	PluginName  = "gpu-admission"
	waitTimeout = 10 * time.Second
)

var _ framework.FilterPlugin = &VGPUPlugin{}

type VGPUPlugin struct {
	handle framework.Handle
	client kubernetes.Interface
}

func (v *VGPUPlugin) Name() string {
	return PluginName
}

func New(_ runtime.Object, f framework.Handle) (framework.Plugin, error) {
	return &VGPUPlugin{handle: f, client: f.ClientSet()}, nil
}

func (v *VGPUPlugin) Filter(ctx context.Context, _ *framework.CycleState, pod *v1.Pod, node *framework.NodeInfo) *framework.Status {
	klog.V(3).Infof("filter pod: %s, node: %s\n", pod.Name, node.Node().Name)

	if !util.IsGPURequiredPod(pod) {
		return framework.NewStatus(framework.Success, fmt.Sprintf("pod %s do not require gpus", pod.Name))
	}

	for k := range pod.Annotations {
		if strings.Contains(k, util.GPUAssigned) ||
			strings.Contains(k, util.PredicateTimeAnnotation) ||
			strings.Contains(k, util.PredicateGPUIndexPrefix) {
			return framework.NewStatus(framework.Skip, fmt.Sprintf("pod %s had been predicated!", pod.Name))
		}
	}

	if !util.IsGPUEnabledNode(node.Node()) {
		return framework.NewStatus(framework.Skip, fmt.Sprintf("node %s no GPU device", node.Node().Name))
	}

	pods, err := v.ListPodsOnNode(node.Node())
	if err != nil {
		return framework.NewStatus(framework.Error, err.Error())
	}

	nodeInfo := device.NewNodeInfo(node.Node(), pods)
	alloc := algorithm.NewAllocator(nodeInfo)
	newPod, err := alloc.Allocate(pod)
	if err != nil {
		return framework.NewStatus(framework.Error, err.Error())
	}

	annotationMap := make(map[string]string)
	for k, v := range newPod.Annotations {
		if strings.Contains(k, util.GPUAssigned) ||
			strings.Contains(k, util.PredicateTimeAnnotation) ||
			strings.Contains(k, util.PredicateGPUIndexPrefix) ||
			strings.Contains(k, util.PredicateNode) {
			annotationMap[k] = v
		}
	}

	err = v.patchPodWithAnnotations(newPod, annotationMap)
	if err != nil {
		return framework.NewStatus(framework.Error, err.Error())
	}

	return framework.NewStatus(framework.Success, "")
}

func (v *VGPUPlugin) ListPodsOnNode(node *v1.Node) ([]*v1.Pod, error) {
	// #lizard forgives
	pods, err := v.client.CoreV1().Pods(v1.NamespaceAll).List(context.Background(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	var ret []*v1.Pod
	for _, pod := range pods.Items {
		klog.V(9).Infof("List pod %s", pod.Name)
		var predicateNode string
		if pod.Spec.NodeName == "" && pod.Annotations != nil {
			if v, ok := pod.Annotations[util.PredicateNode]; ok {
				predicateNode = v
			}
		}
		if (pod.Spec.NodeName == node.Name || predicateNode == node.Name) &&
			pod.Status.Phase != v1.PodSucceeded &&
			pod.Status.Phase != v1.PodFailed {
			ret = append(ret, &pod)
			klog.V(9).Infof("get pod %s on node %s", pod.UID, node.Name)
		}
	}
	return ret, nil
}

func (v *VGPUPlugin) patchPodWithAnnotations(
	pod *v1.Pod, annotationMap map[string]string) error {
	// update annotations by patching to the pod
	type patchMetadata struct {
		Annotations map[string]string `json:"annotations"`
	}
	type patchPod struct {
		Metadata patchMetadata `json:"metadata"`
	}
	payload := patchPod{
		Metadata: patchMetadata{
			Annotations: annotationMap,
		},
	}

	payloadBytes, _ := json.Marshal(payload)
	err := wait.PollImmediate(time.Second, waitTimeout, func() (bool, error) {
		_, err := v.client.CoreV1().Pods(pod.Namespace).
			Patch(context.Background(), pod.Name, k8stypes.StrategicMergePatchType, payloadBytes, metav1.PatchOptions{})
		if err == nil {
			return true, nil
		}
		if util.ShouldRetry(err) {
			return false, nil
		}

		return false, err
	})
	if err != nil {
		msg := fmt.Sprintf("failed to add annotation %v to pod %s due to %s",
			annotationMap, pod.UID, err.Error())
		klog.V(3).Info(msg)
		return fmt.Errorf(msg)
	}
	return nil
}
