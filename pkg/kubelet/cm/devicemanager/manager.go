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
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/golang/glog"

	"k8s.io/api/core/v1"
	"k8s.io/kubernetes/pkg/kubelet/config"
	"k8s.io/kubernetes/pkg/kubelet/lifecycle"
	"k8s.io/kubernetes/pkg/kubelet/metrics"
	"k8s.io/kubernetes/plugin/pkg/scheduler/schedulercache"

	v1helper "k8s.io/kubernetes/pkg/apis/core/v1/helper"
	pluginapi "k8s.io/kubernetes/pkg/kubelet/apis/deviceplugin/v1alpha"
	pluginregistration "k8s.io/kubernetes/pkg/kubelet/apis/pluginregistration/v1beta"
	kubecontainer "k8s.io/kubernetes/pkg/kubelet/container"
)

// managerCallback is the function called when a device's health state changes,
// or new devices are reported, or old devices are deleted.
// Updated contains the most recent state of the Device.
type managerCallback func(resourceName string, added, updated, deleted []pluginapi.Device)

// ManagerImpl is the structure in charge of managing Device Plugins.
type ManagerImpl struct {
	pluginDir     string
	checkpointDir string

	// interface handling all Endpoint operations
	endpointHandler endpointHandler
	pluginWatcher   pluginregistration.PluginWatcher
	deviceStore     managerStore
	podCache        cache

	// activePods is a method for listing active pods on the node
	// so the amount of pluginResources requested by existing pods
	// could be counted when updating allocated devices
	activePods ActivePodsFunc

	// sourcesReady provides the readiness of kubelet configuration sources such as apiserver update readiness.
	// We use it to determine when we can purge inactive pods from checkpointed state.
	sourcesReady config.SourcesReady

	stopChan chan interface{}
	wg       sync.WaitGroup
}

func NewManagerImpl() (*ManagerImpl, error) {
	return NewManagerImplWithPluginDir(pluginapi.DevicePluginsPath)
}

// NewManagerImpl creates a new manager.
func NewManagerImplWithPluginDir(pluginDir string) (*ManagerImpl, error) {
	watcher, err := pluginregistration.NewPluginWatcherImpl(pluginDir)
	if err != nil {
		return nil, err
	}

	deviceStore := newManagerStoreImpl()

	return &ManagerImpl{
		pluginDir: pluginDir,

		pluginWatcher:   watcher,
		deviceStore:     deviceStore,
		endpointHandler: newEndpointHandlerImpl(deviceStore.UpdateCapacity),
		podCache:        newCacheImpl(),

		stopChan: make(chan interface{}),
	}, nil
}

// Start starts the Device Plugin Manager amd start initialization of
// podDevices and allocatedDevices information from checkpoint-ed state and
// starts device plugin registration service.
func (m *ManagerImpl) Start(activePods ActivePodsFunc, sourcesReady config.SourcesReady) error {
	glog.V(2).Infof("Starting Device Plugin manager")

	m.activePods = activePods
	m.sourcesReady = sourcesReady

	if err := m.pluginWatcher.Start(); err != nil {
		return fmt.Errorf("failed to start Plugin Watcher with err: %+v", err)
	}

	m.wg.Add(1)
	go m.run()

	glog.V(2).Infof("Device Manager is now running")

	return nil
}

func (m *ManagerImpl) run() {
	defer m.wg.Done()

	endpointsAdded := m.pluginWatcher.Added()
	endpointsRemoved := m.pluginWatcher.Removed()

	for {
		select {
		case endpoint := <-endpointsAdded:
			if endpoint == "" {
				continue
			}

			glog.V(4).Infof("Got endpoint %s", endpoint)
			domain := strings.TrimPrefix(filepath.Dir(endpoint), m.pluginDir)
			domain = strings.TrimPrefix(domain, "/")

			e, err := m.endpointHandler.NewEndpoint(endpoint, domain)
			if err != nil {
				glog.Errorf("could not register Endpoint %s: %+v", endpoint, err)
				continue
			}

			glog.V(2).Infof("Successfully added Device plugin %s", e.ResourceName())

		case removed := <-endpointsRemoved:
			glog.V(2).Infof("Got Remove event %s", removed)
			continue

		case <-m.stopChan:
			return
		}
	}
}

// Allocate is the call that you can use to allocate a set of devices
// from the registered device plugins.
func (m *ManagerImpl) AdmitPod(node *schedulercache.NodeInfo, attrs *lifecycle.PodAdmitAttributes) error {
	m.lazyPodDelete(m.activePods())
	var rNames []v1.ResourceName

	for _, pRes := range attrs.Pod.Spec.ExtendedResources {
		rName, err := v1helper.PodExtendedResourceName(&pRes)
		if err != nil {
			return err
		}

		if err := m.deviceStore.HasDevices(string(rName), pRes.Assigned); err != nil {
			return err
		}

		rNames = append(rNames, rName)
	}

	for _, rName := range rNames {
		if err := m.admitPod(attrs.Pod, rName); err != nil {
			return err
		}
	}

	return nil
}

func (m *ManagerImpl) DeletePod(uid string) {
	m.podCache.DeletePod(uid)
}

// GetCapacity is expected to be called when Kubelet updates its node status.
// The first returned variable contains the registered device plugin resource capacity.

// The second returned variable contains previously registered resources that are no longer active.
// Kubelet uses this information to update resource capacity/allocatable in its node status.
func (m *ManagerImpl) GetCapacity() (v1.ExtendedResourceMap, []string) {
	capacity, removed := m.deviceStore.GetCapacity()
	return capacity, removed
}

func (m *ManagerImpl) admitPod(p *v1.Pod, rName v1.ResourceName) error {
	request := &pluginapi.AdmitPodRequest{
		PodName:        p.Name,
		InitContainers: map[string]*pluginapi.Container{},
		Containers:     map[string]*pluginapi.Container{},
	}

	for _, container := range p.Spec.InitContainers {
		devs, err := v1helper.PodExtendedResourceAssigned(rName, &container, p)
		if err != nil {
			return err
		}

		request.InitContainers[container.Name] = &pluginapi.Container{
			Name:    container.Name,
			Devices: devs,
		}
	}

	for _, container := range p.Spec.Containers {
		devs, err := v1helper.PodExtendedResourceAssigned(rName, &container, p)
		if err != nil {
			return err
		}

		request.Containers[container.Name] = &pluginapi.Container{
			Name:    container.Name,
			Devices: devs,
		}
	}

	resource := string(rName)
	e, ok := m.endpointHandler.Endpoint(resource)
	if !ok {
		return fmt.Errorf("could not find Endpoint for %s", resource)
	}

	startRPCTime := time.Now()
	resp, err := e.AdmitPod(request)
	metrics.DevicePluginAllocationLatency.WithLabelValues(resource).Observe(metrics.SinceInMicroseconds(startRPCTime))

	m.podCache.CachePodResources(p, resp)

	return err
}

func (m *ManagerImpl) PodResources(p *v1.Pod) *kubecontainer.RunPodOptions {
	return m.podCache.PodResources(p)
}

// Iterate over all the resources requested by the container
// Execute n calls to InitContainer with n being the number of different device
//         plugin resources requested
func (m *ManagerImpl) InitContainer(p *v1.Pod, c *v1.Container) (*kubecontainer.RunContainerOptions, error) {
	podResources := p.Spec.ExtendedResources
	requests := map[string]*InitRequest{}

	for _, r := range c.ExtendedResourceRequests {
		i, err := v1helper.PodExtendedResource(r, podResources)
		if err != nil {
			return nil, err
		}

		rName, err := v1helper.PodExtendedResourceName(&podResources[i])
		if err != nil {
			return nil, err
		}

		res := string(rName)

		if _, ok := requests[res]; !ok {
			requests[res] = &InitRequest{
				Container: &pluginapi.Container{
					Name:    c.Name,
					Devices: []string{},
				},
			}
		}

		devices := podResources[i].Assigned
		requests[res].Container.Devices = append(requests[res].Container.Devices, devices...)
	}

	// foreach device plugin
	var responses []*InitResponse
	for res, req := range requests {
		e, ok := m.endpointHandler.Endpoint(res)
		if !ok {
			return nil, fmt.Errorf("could not find Endpoint for %s", res)
		}

		resp, err := e.InitContainer(req)
		if err != nil {
			return nil, err
		}
		responses = append(responses, resp)
	}

	return DeviceRunContainerOptionsFromInitResponses(responses), nil
}

func (m *ManagerImpl) lazyPodDelete(activePods []*v1.Pod) {
	if !m.sourcesReady.AllReady() {
		return
	}

	cachedPods := m.podCache.ListPods()
	for _, p := range activePods {
		if _, ok := cachedPods[string(p.UID)]; !ok {
			continue
		}

		delete(cachedPods, string(p.UID))
	}

	for uid := range cachedPods {
		m.DeletePod(uid)
	}
}

func (m *ManagerImpl) setStore(store managerStore) {
	m.deviceStore = store
	m.endpointHandler.SetCallback(store.UpdateCapacity)
}

// Stop is the function that can stop the gRPC server.
func (m *ManagerImpl) Stop() error {
	glog.V(2).Infof("Stopping the manager")

	// always stop subcomponents before stoping manager (e.g pluginwatcher)
	// Stop endpoints
	if err := m.endpointHandler.Stop(); err != nil {
		return err
	}

	// stop watcher
	if err := m.pluginWatcher.Stop(); err != nil {
		return err
	}

	close(m.stopChan)

	if err := m.waitTimeout(); err != nil {
		return err
	}

	return nil
}

func (m *ManagerImpl) waitTimeout() error {
	c := make(chan interface{})
	go func() {
		defer close(c)
		m.wg.Wait()
	}()

	select {
	case <-c:
		return nil
	case <-time.After(time.Second * 5):
		return fmt.Errorf("timeout while stopping device plugin manager")
	}

	return nil
}
