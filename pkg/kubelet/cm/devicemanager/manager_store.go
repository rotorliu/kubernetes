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

	"github.com/golang/glog"
	"k8s.io/api/core/v1"

	pluginapi "k8s.io/kubernetes/pkg/kubelet/apis/deviceplugin/v1alpha"
)

type managerStore interface {
	UpdateCapacity(res string, added, updated, deleted []pluginapi.Device)
	GetCapacity() (capacity v1.ExtendedResourceMap, removed []string)
	HasDevices(res string, ids []string) error
}

type ManagerStoreImpl struct {
	m         sync.Mutex
	resources v1.ExtendedResourceMap
	removed   []string
}

func newManagerStoreImpl() *ManagerStoreImpl {
	return &ManagerStoreImpl{
		resources: v1.ExtendedResourceMap{},
	}
}

func (s *ManagerStoreImpl) UpdateCapacity(res string, a, u, d []pluginapi.Device) {
	glog.V(2).Infof("%s: UpdateCapacity added: %v, updated: %v, deleted: %v", res, a, u, d)

	s.m.Lock()
	defer s.m.Unlock()

	rName := v1.ResourceName(res)

	if _, ok := s.resources[rName]; !ok {
		s.resources[rName] = v1.ExtendedResourceDomain{
			Resources: make(map[string]v1.ExtendedResource),
		}
	}

	domain := s.resources[rName]

	for _, d := range a {
		domain.Resources[d.ID] = PluginAPIDeviceToV1(d)
	}

	for _, d := range u {
		domain.Resources[d.ID] = PluginAPIDeviceToV1(d)
	}

	for _, d := range d {
		if _, ok := domain.Resources[d.ID]; !ok {
			continue
		}

		delete(domain.Resources, d.ID)
	}

	if len(domain.Resources) == 0 {
		delete(s.resources, rName)
		s.removed = append(s.removed, res)
	}
}

func (s *ManagerStoreImpl) GetCapacity() (v1.ExtendedResourceMap, []string) {
	s.m.Lock()
	defer s.m.Unlock()

	clone := v1.ExtendedResourceMap{}
	for k, v := range s.resources {
		clone[k] = *v.DeepCopy()
	}

	removed := s.removed
	s.removed = nil

	return clone, removed
}

func (s *ManagerStoreImpl) HasDevices(res string, ids []string) error {
	s.m.Lock()
	defer s.m.Unlock()

	rName := v1.ResourceName(res)
	domain, ok := s.resources[rName]
	if !ok {
		return fmt.Errorf("resource %s not available on this node", rName)
	}

	for _, id := range ids {
		r, ok := domain.Resources[id]
		if !ok {
			return fmt.Errorf("resource %s/%s not available on this node", rName, id)
		}

		if r.Health != pluginapi.Healthy {
			return fmt.Errorf("resource %s/%s is unhealthy on this node", rName, id)
		}
	}

	return nil
}

func PluginAPIDeviceToV1(d pluginapi.Device) v1.ExtendedResource {
	return v1.ExtendedResource{
		ID:         d.ID,
		Health:     d.Health,
		Attributes: d.Attributes,
	}
}
