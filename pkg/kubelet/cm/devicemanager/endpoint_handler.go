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
	"sync"
	"time"

	pluginapi "k8s.io/kubernetes/pkg/kubelet/apis/deviceplugin/v1alpha"
	pluginregistration "k8s.io/kubernetes/pkg/kubelet/apis/pluginregistration/v1beta"
)

// endpointHandler handles all operations related to endpoints
// namely creating the endpoints, keeping a map of them and stoping them
type endpointHandler interface {
	NewEndpoint(socketPath, domain string) (endpoint, error)

	Devices() map[string][]pluginapi.Device
	Endpoint(resourceName string) (endpoint, bool)

	Stop() error

	Store() endpointStore
	SetStore(endpointStore)

	SetCallback(f managerCallback)
}

type endpointStore interface {
	Endpoint(resourceName string) (endpoint, bool)

	SwapEndpoint(e endpoint) (endpoint, bool)
	DeleteEndpoint(resourceName string) error

	Range(func(string, endpoint))
}

type endpointStoreImpl struct {
	sync.Mutex
	endpoints map[string]endpoint
}

type endpointHandlerImpl struct {
	wg sync.WaitGroup

	store    endpointStore
	callback managerCallback
}

func newEndpointHandlerImpl(f managerCallback) *endpointHandlerImpl {
	return &endpointHandlerImpl{
		store:    newEndpointStoreImpl(),
		callback: f,
	}
}

func (h *endpointHandlerImpl) NewEndpoint(socketPath, domain string) (endpoint, error) {
	v := pluginregistration.NewValidatorImpl(pluginapi.Version, domain)
	c, err := v.Connect(socketPath, time.Second)
	if err != nil {
		return nil, err
	}

	rName, err := v.ValidateEndpoint(c)
	v.NotifyRegistrationStatus(c, err)
	if err != nil {
		return nil, err
	}

	e, err := newEndpoint(c, rName)
	if err != nil {
		return nil, err
	}

	// If an endpoint with this resourceName is already present
	var devStore deviceStore = newDeviceStoreImpl(h.callback)
	if old, ok := h.store.Endpoint(e.ResourceName()); ok && old != nil {
		devStore = old.Store()
	}

	// set a new store or the old store
	e.SetStore(devStore)

	// Add the endpoint to the store (and get the old one)
	old, ok := h.store.SwapEndpoint(e)

	h.wg.Add(1)
	go h.trackEnpoint(e)

	if ok && old != nil {
		// Prevent it to issue a Callback to the manager before stopping it
		old.SetStore(newAlwaysEmptyDeviceStore())

		if err = old.Stop(); err != nil {
			return nil, err
		}
	}

	return e, nil
}

// This function has two code path:
//  - The endpoint was stopped to be removed
//     - It's still in the store and has the same value
//     - Remove it from the store
//  - The endpoint was stopped to be replaced
//     - It's still in the store but was already replaced
//     - Do nothing and return
func (h *endpointHandlerImpl) trackEnpoint(e endpoint) {
	defer h.wg.Done()

	rName := e.ResourceName()
	e.Run()

	curr, ok := h.store.Endpoint(rName)
	if !ok || curr != e {
		return
	}

	h.store.DeleteEndpoint(rName)
}

func (h *endpointHandlerImpl) Devices() map[string][]pluginapi.Device {
	devs := make(map[string][]pluginapi.Device)

	h.store.Range(func(k string, e endpoint) {
		devs[k] = e.Store().Devices()
	})

	return devs
}

func (h *endpointHandlerImpl) Endpoint(resourceName string) (endpoint, bool) {
	return h.store.Endpoint(resourceName)
}

func (h *endpointHandlerImpl) Stop() error {
	var endpoints []endpoint
	h.store.Range(func(k string, e endpoint) {
		endpoints = append(endpoints, e)
	})

	for _, e := range endpoints {
		if err := e.Stop(); err != nil {
			return err
		}
	}

	if err := h.waitTimeout(); err != nil {
		return err
	}

	return nil
}

func (h *endpointHandlerImpl) waitTimeout() error {
	c := make(chan interface{})
	go func() {
		defer close(c)
		h.wg.Wait()
	}()

	select {
	case <-c:
		return nil
	case <-time.After(time.Second):
		return fmt.Errorf("Timeout while stopping plugin watcher")
	}

	return nil
}

func (h *endpointHandlerImpl) Store() endpointStore {
	return h.store
}

func (h *endpointHandlerImpl) SetStore(s endpointStore) {
	h.store = s
}

func (h *endpointHandlerImpl) SetCallback(f managerCallback) {
	h.callback = f

	h.store.Range(func(k string, e endpoint) {
		e.Store().SetCallback(f)
	})
}

func newEndpointStoreImpl() *endpointStoreImpl {
	return &endpointStoreImpl{
		endpoints: make(map[string]endpoint),
	}
}

func (s *endpointStoreImpl) Endpoint(resourceName string) (endpoint, bool) {
	s.Lock()
	defer s.Unlock()

	e, ok := s.endpoints[resourceName]
	return e, ok
}

func (s *endpointStoreImpl) SwapEndpoint(e endpoint) (endpoint, bool) {
	s.Lock()
	defer s.Unlock()

	old, ok := s.endpoints[e.ResourceName()]
	s.endpoints[e.ResourceName()] = e

	return old, ok
}

func (s *endpointStoreImpl) DeleteEndpoint(resourceName string) error {
	s.Lock()
	defer s.Unlock()

	_, ok := s.endpoints[resourceName]
	if !ok {
		return fmt.Errorf("Endpoint %s does not exist", resourceName)
	}

	delete(s.endpoints, resourceName)
	return nil
}

func (s *endpointStoreImpl) Range(f func(string, endpoint)) {
	s.Lock()
	defer s.Unlock()

	for k, v := range s.endpoints {
		f(k, v)
	}
}
