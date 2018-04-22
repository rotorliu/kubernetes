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
	"k8s.io/kubernetes/pkg/kubelet/lifecycle"
	"k8s.io/kubernetes/plugin/pkg/scheduler/schedulercache"

	pluginapi "k8s.io/kubernetes/pkg/kubelet/apis/deviceplugin/v1alpha"
	pluginregistration "k8s.io/kubernetes/pkg/kubelet/apis/pluginregistration/v1beta"
	kubecontainer "k8s.io/kubernetes/pkg/kubelet/container"
)

// Manager manages all the Device Plugins running on a node.
type Manager interface {
	// Start starts device plugin registration service.
	Start(activePods ActivePodsFunc, sourcesReady config.SourcesReady) error

	AdmitPod(node *schedulercache.NodeInfo, attrs *lifecycle.PodAdmitAttributes) error
	InitContainer(pod *v1.Pod, container *v1.Container) (*kubecontainer.RunContainerOptions, error)

	GetCapacity() (v1.ExtendedResourceMap, []string)

	PodResources(p *v1.Pod) *kubecontainer.RunPodOptions

	// Stop stops the manager.
	Stop() error
}

// ActivePodsFunc is a function that returns a list of pods to reconcile.
type ActivePodsFunc func() []*v1.Pod

type InitResponse = pluginapi.InitContainerResponse
type InitRequest = pluginapi.InitContainerRequest

type AdmitRequest = pluginapi.AdmitPodRequest
type AdmitResponse = pluginapi.AdmitPodResponse

type VersionsRequest = pluginregistration.GetSupportedVersionsRequest
type VersionsResponse = pluginregistration.GetSupportedVersionsResponse

type IdentityRequest = pluginregistration.GetPluginIdentityRequest
type IdentityResponse = pluginregistration.GetPluginIdentityResponse

type InfoRequest = pluginapi.GetPluginInfoRequest
type InfoResponse = pluginapi.GetPluginInfoResponse

type ListAndWatchStream = pluginapi.DevicePlugin_ListAndWatchServer
