/*
Copyright 2015 The Kubernetes Authors.

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

package schedulercache

import (
	"github.com/golang/glog"
	"k8s.io/api/core/v1"
	v1helper "k8s.io/kubernetes/pkg/apis/core/v1/helper"
)

type ExtendedResourceCacheManager interface {
	Clone() ExtendedResourceCacheManager
	Available() v1.ExtendedResourceMap

	AddPod(pod *v1.Pod)
	RemovePod(pod *v1.Pod)

	SetNode(node *v1.Node)
	RemoveNode(node *v1.Node)

	// Used for testing
	SetAvailable(v1.ExtendedResourceMap)
}

type ExtendedResourceCacheManagerImpl struct {
	allocatable v1.ExtendedResourceMap
	available   v1.ExtendedResourceMap
	used        v1.ExtendedResourceMap
}

func NewExtendedResourceCacheManagerImpl() *ExtendedResourceCacheManagerImpl {
	return &ExtendedResourceCacheManagerImpl{
		allocatable: v1.ExtendedResourceMap{},
		available:   v1.ExtendedResourceMap{},
		used:        v1.ExtendedResourceMap{},
	}
}

func (m *ExtendedResourceCacheManagerImpl) Clone() ExtendedResourceCacheManager {
	clone := NewExtendedResourceCacheManagerImpl()

	for k, v := range m.allocatable {
		clone.allocatable[k] = *v.DeepCopy()
	}

	for k, v := range m.available {
		clone.available[k] = *v.DeepCopy()
	}

	for k, v := range m.used {
		clone.used[k] = *v.DeepCopy()
	}

	return clone
}

func (m *ExtendedResourceCacheManagerImpl) Available() v1.ExtendedResourceMap {
	available := v1.ExtendedResourceMap{}
	for k, v := range m.available {
		available[k] = *v.DeepCopy()
	}

	return available
}

func (m *ExtendedResourceCacheManagerImpl) SetAvailable(av v1.ExtendedResourceMap) {
	m.available = av
}

// Adds the pod resources to requests
// Remove the pod resources from available
func (m *ExtendedResourceCacheManagerImpl) AddPod(pod *v1.Pod) {
	for _, pRes := range pod.Spec.ExtendedResources {
		resourceName, err := v1helper.PodExtendedResourceName(&pRes)
		if err != nil {
			continue
		}

		// Pod added with resource we don't know about ignore?
		nRes, ok := m.available[resourceName]
		if !ok {
			continue
		}

		for _, id := range pRes.Assigned {
			// Pod added with resource we don't know about ignore?
			if _, ok := nRes.Resources[id]; !ok {
				continue
			}

			// Add to used Remove from Available
			m.used[resourceName].Resources[id] = nRes.Resources[id]
			delete(nRes.Resources, id)
		}
	}
}

// Adds the pod resources to available if it is in allocatable
// Remove the pod resources from requests if they are in requests
func (m *ExtendedResourceCacheManagerImpl) RemovePod(pod *v1.Pod) {
	for _, pRes := range pod.Spec.ExtendedResources {
		resourceName, err := v1helper.PodExtendedResourceName(&pRes)
		if err != nil {
			continue
		}

		// That's an error, ignore?
		uRes, ok := m.used[resourceName]
		if !ok {
			continue
		}

		allocRes, isAllocated := m.allocatable[resourceName]

		availableRes := m.available[resourceName]
		for _, id := range pRes.Assigned {
			// That's an error, ignore?
			if _, ok := uRes.Resources[id]; !ok {
				continue
			}

			delete(uRes.Resources, id)

			// Unknown resource or recently removed, Don't add it back
			if !isAllocated {
				continue
			}

			// Unknown resource or recently removed, Don't add it back
			r, ok := allocRes.Resources[id]
			if !ok {
				continue
			}

			availableRes.Resources[id] = r
		}
	}
}

func (m *ExtendedResourceCacheManagerImpl) SetNode(node *v1.Node) {
	// Resources Removed
	for resourceName, allocRes := range m.allocatable {
		nRes, ok := node.Status.ExtendedResources[resourceName]
		if !ok {
			m.available[resourceName] = v1.ExtendedResourceDomain{
				Resources: map[string]v1.ExtendedResource{},
			}
			continue
		}

		// That's an error
		avRes, ok := m.available[resourceName]
		if !ok {
			continue
		}

		for id := range allocRes.Resources {
			if _, ok := nRes.Resources[id]; ok {
				continue
			}

			delete(avRes.Resources, id)
		}
	}

	// Resources Added
	for resourceName, nRes := range node.Status.ExtendedResources {
		_, ok := m.allocatable[resourceName]
		if !ok {
			m.available[resourceName] = *nRes.DeepCopy()
			m.allocatable[resourceName] = *nRes.DeepCopy()

			m.used[resourceName] = v1.ExtendedResourceDomain{
				Resources: map[string]v1.ExtendedResource{},
			}

			continue
		}

		// If it's not in use then we re-create it
		for id, res := range nRes.Resources {
			if _, ok := m.used[resourceName].Resources[id]; ok {
				continue
			}

			m.available[resourceName].Resources[id] = res
			m.allocatable[resourceName].Resources[id] = res
		}
	}

	glog.V(6).Infof("Allocatable: %+v", m.allocatable)
	glog.V(6).Infof("Available: %+v", m.available)
}

func (m *ExtendedResourceCacheManagerImpl) RemoveNode(node *v1.Node) {
}
