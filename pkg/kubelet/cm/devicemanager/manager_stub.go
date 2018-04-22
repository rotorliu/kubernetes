/*
Copyright 2017 The Kubernetes Authors.

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

package devicemanager

import (
	"k8s.io/api/core/v1"
	"k8s.io/kubernetes/pkg/kubelet/config"
	kubecontainer "k8s.io/kubernetes/pkg/kubelet/container"
	"k8s.io/kubernetes/pkg/kubelet/lifecycle"
	"k8s.io/kubernetes/plugin/pkg/scheduler/schedulercache"
)

// ManagerStub provides a simple stub implementation for the Device Manager.
type ManagerStub struct{}

var activePodsStub = func() []*v1.Pod {
	return []*v1.Pod{}
}

type sourcesReadyStub struct{}

func (s *sourcesReadyStub) AddSource(source string) {}
func (s *sourcesReadyStub) AllReady() bool          { return true }

// NewManagerStub creates a ManagerStub.
func NewManagerStub() (*ManagerStub, error) {
	return &ManagerStub{}, nil
}

// Start simply returns nil.
func (h *ManagerStub) Start(activePods ActivePodsFunc, sourcesReady config.SourcesReady) error {
	return nil
}

// Stop simply returns nil.
func (h *ManagerStub) Stop() error {
	return nil
}

// Allocate simply returns nil.
func (h *ManagerStub) AdmitPod(node *schedulercache.NodeInfo, attrs *lifecycle.PodAdmitAttributes) error {
	return nil
}

// GetDeviceRunContainerOptions simply returns nil.
func (h *ManagerStub) InitContainer(p *v1.Pod, c *v1.Container) (*kubecontainer.RunContainerOptions, error) {
	return nil, nil
}

// GetCapacity simply returns nil capacity and empty removed resource list.
func (h *ManagerStub) GetCapacity() (v1.ExtendedResourceMap, []string) {
	return nil, []string{}
}

func (h *ManagerStub) PodResources(p *v1.Pod) *kubecontainer.RunPodOptions {
	return nil
}
