/*
Copyright 2014 The Kubernetes Authors.

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

package core

import (
	"testing"

	"github.com/stretchr/testify/require"

	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/kubernetes/plugin/pkg/scheduler/schedulercache"
)

const (
	memoryAttr  = "nvidia.com/memory"
	computeAttr = "nvidia.com/compute-capabilities"
	eccAttr     = "nvidia.com/ecc"
)

func newRes(id, health, memory, compute, ecc string) v1.ExtendedResource {
	return v1.ExtendedResource{
		ID:     id,
		Health: health,
		Attributes: map[string]string{
			memoryAttr:  memory,
			computeAttr: compute,
			eccAttr:     ecc,
		},
	}
}

func newResourceSelector(key string, op v1.NodeSelectorOperator, values ...string) v1.ResourceSelector {
	return v1.ResourceSelector{
		Key:      key,
		Operator: op,
		Values:   values,
	}
}

func newResourceList(rName v1.ResourceName, val int) v1.ResourceList {
	return v1.ResourceList{rName: *resource.NewQuantity(int64(val), resource.DecimalSI)}
}

func TestRemoveFromAvailable(t *testing.T) {
	name := "foo"
	rName := v1.ResourceName(name)

	require.Error(t, removeFromAvailable(v1.ExtendedResourceMap{}, rName, []string{}))
	require.Error(t, removeFromAvailable(v1.ExtendedResourceMap{
		rName: v1.ExtendedResourceDomain{},
	}, rName, []string{"someId1"}))

	available := v1.ExtendedResourceMap{
		rName: v1.ExtendedResourceDomain{
			Resources: map[string]v1.ExtendedResource{
				"someId1": {},
				"someId2": {},
				"someId3": {},
			},
		},
	}

	require.NoError(t, removeFromAvailable(available, rName, []string{"someId1", "someId2"}))
	require.Len(t, available, 1)
	require.Contains(t, available, rName)
	require.Contains(t, available[rName].Resources, "someId3")
}

func TestIsDeviceAMatch(t *testing.T) {
	res := newRes("someId1", v1.ExtendedResourcesHealthy, "4096", "4.2", "true")

	ok, err := isDeviceAMatch(res, v1.ExtendedResourceAffinity{
		Required: []v1.ResourceSelector{
			newResourceSelector(memoryAttr, v1.NodeSelectorOpGt, "4095"),
		},
	})

	require.NoError(t, err)
	require.True(t, ok)

	ok, err = isDeviceAMatch(res, v1.ExtendedResourceAffinity{
		Required: []v1.ResourceSelector{
			newResourceSelector(memoryAttr, v1.NodeSelectorOpLt, "4095"),
		},
	})

	require.NoError(t, err)
	require.False(t, ok)

	ok, err = isDeviceAMatch(res, v1.ExtendedResourceAffinity{
		Required: []v1.ResourceSelector{
			newResourceSelector(memoryAttr, v1.NodeSelectorOpGt, "4095"),
			newResourceSelector(eccAttr, v1.NodeSelectorOpIn, "true"),
		},
	})

	require.NoError(t, err)
	require.True(t, ok)

	ok, err = isDeviceAMatch(res, v1.ExtendedResourceAffinity{
		Required: []v1.ResourceSelector{
			newResourceSelector(memoryAttr, v1.NodeSelectorOpGt, "4095"),
			newResourceSelector(eccAttr, v1.NodeSelectorOpIn, "false"),
		},
	})

	require.NoError(t, err)
	require.False(t, ok)

	ok, err = isDeviceAMatch(res, v1.ExtendedResourceAffinity{
		Required: []v1.ResourceSelector{
			newResourceSelector(memoryAttr, v1.NodeSelectorOpGt, "4095"),
			newResourceSelector(eccAttr, v1.NodeSelectorOpGt, "false", "true"),
		},
	})

	require.Error(t, err)

	ok, err = isDeviceAMatch(res, v1.ExtendedResourceAffinity{})
	require.NoError(t, err)
	require.True(t, ok)
}

func TestAllocateResources(t *testing.T) {
	rName := v1.ResourceName("foo")
	notRName := v1.ResourceName("bar")

	affinity := v1.ExtendedResourceAffinity{
		Required: []v1.ResourceSelector{
			newResourceSelector(memoryAttr, v1.NodeSelectorOpGt, "4095"),
		},
	}

	res1 := newRes("someId1", v1.ExtendedResourcesHealthy, "4096", "4.2", "true")
	res2 := newRes("someId2", v1.ExtendedResourcesHealthy, "4096", "4.2", "true")
	unfitRes := newRes("someId3", v1.ExtendedResourcesHealthy, "2048", "4.2", "true")

	available := v1.ExtendedResourceMap{notRName: v1.ExtendedResourceDomain{}}
	_, failure := allocateResources(rName, int64(1), affinity, available)
	require.NotNil(t, failure)

	available = v1.ExtendedResourceMap{
		rName: v1.ExtendedResourceDomain{
			Resources: map[string]v1.ExtendedResource{
				unfitRes.ID: unfitRes,
				res1.ID:     res1,
				res2.ID:     res2,
			},
		},
	}

	selected, failure := allocateResources(rName, int64(1), affinity, available)
	require.Nil(t, failure)
	require.Len(t, selected, 1)
	require.NotContains(t, selected, "unfitResId1")

	selected, failure = allocateResources(rName, int64(2), affinity, available)
	require.Nil(t, failure)
	require.Len(t, selected, 2)
	require.Contains(t, selected, res1.ID)
	require.Contains(t, selected, res2.ID)

	affinity = v1.ExtendedResourceAffinity{}
	selected, failure = allocateResources(rName, int64(1), affinity, available)
	require.Nil(t, failure)
	require.Len(t, selected, 1)
}

func NewPodExtendedResource(name string, res v1.ResourceList, affinity v1.ExtendedResourceAffinity) v1.PodExtendedResource {
	return v1.PodExtendedResource{
		Name: name,
		Resources: v1.ResourceRequirements{Limits: res},
		Affinity: affinity,
	}

}

func TestHasExtendedResources(t *testing.T) {
	res1 := newRes("someId1", v1.ExtendedResourcesHealthy, "4096", "4.2", "true")
	res2 := newRes("someId2", v1.ExtendedResourcesHealthy, "4096", "4.2", "true")
	unfitRes1 := newRes("someId3", v1.ExtendedResourcesHealthy, "2048", "4.2", "true")
	unfitRes2 := newRes("someId4", v1.ExtendedResourcesHealthy, "2048", "4.2", "true")

	rName := v1.ResourceName("nvidia.com/foo")
	rName2 := v1.ResourceName("nvidia.com/bar")
	notRName := v1.ResourceName("nvidia.com/baz")

	emptyAffinity := v1.ExtendedResourceAffinity{}
	affinity := v1.ExtendedResourceAffinity{
		Required: []v1.ResourceSelector{
			newResourceSelector(memoryAttr, v1.NodeSelectorOpGt, "4095"),
		},
	}

	available := v1.ExtendedResourceMap{
		rName: v1.ExtendedResourceDomain{
			Resources: map[string]v1.ExtendedResource{
				unfitRes1.ID: unfitRes1,
				res1.ID:     res1,
				res2.ID:     res2,
			},
		},
		rName2: v1.ExtendedResourceDomain{
			Resources: map[string]v1.ExtendedResource{
				unfitRes1.ID: unfitRes1,
				unfitRes2.ID: unfitRes2,
				res1.ID:     res1,
			},
		},
	}

	p := &v1.Pod{Spec: v1.PodSpec{}}
	nI := schedulercache.NewNodeInfo()
	nI.SetAvailable(available)

	// An extended resource with no requests
	p.Spec.ExtendedResources = []v1.PodExtendedResource{NewPodExtendedResource("", v1.ResourceList{}, emptyAffinity)}
	_, _, failed := hasExtendedResources(p, nI)
	require.Len(t, failed, 1)


	// An extended resource not available
	requests := newResourceList(notRName, 2)
	p.Spec.ExtendedResources = []v1.PodExtendedResource{NewPodExtendedResource("", requests, emptyAffinity)}

	_, _, failed = hasExtendedResources(p, nI)
	require.Len(t, failed, 1)

	// An extended resource with not enough resources
	requests = newResourceList(rName, len(available[rName].Resources)+1)
	p.Spec.ExtendedResources = []v1.PodExtendedResource{NewPodExtendedResource("", requests, emptyAffinity)}

	_, _, failed = hasExtendedResources(p, nI)
	require.Len(t, failed, 1)

	// Successful request with no affinity
	requests = newResourceList(rName, len(available[rName].Resources))
	p.Spec.ExtendedResources = []v1.PodExtendedResource{NewPodExtendedResource("foo", requests, emptyAffinity)}

	ok, binding, failed := hasExtendedResources(p, nI)
	require.Len(t, failed, 0)
	require.True(t, ok)
	require.Len(t, binding, 1)
	require.Contains(t, binding, "foo")
	require.Len(t, binding["foo"].Resources, 3)

	// Successful request with affinity
	requests = newResourceList(rName, len(available[rName].Resources) - 1)
	p.Spec.ExtendedResources = []v1.PodExtendedResource{NewPodExtendedResource("foo", requests, affinity)}

	ok, binding, failed = hasExtendedResources(p, nI)
	require.Len(t, failed, 0)
	require.True(t, ok)
	require.Len(t, binding, 1)
	require.Contains(t, binding, "foo")
	require.Len(t, binding["foo"].Resources, 2)
	require.NotContains(t, binding["foo"].Resources, unfitRes1.ID)

	// Successful complex request with affinity
	requests1 := newResourceList(rName, 2)
	requests2 := newResourceList(rName2, 1)
	p.Spec.ExtendedResources = []v1.PodExtendedResource{
		NewPodExtendedResource("foo", requests1, affinity),
		NewPodExtendedResource("bar", requests2, affinity),
	}

	ok, binding, failed = hasExtendedResources(p, nI)
	require.Len(t, failed, 0)
	require.True(t, ok)

	require.Len(t, binding, 2)
	require.Contains(t, binding, "foo")
	require.Contains(t, binding, "bar")
	require.Len(t, binding["foo"].Resources, 2)
	require.Len(t, binding["bar"].Resources, 1)
	require.NotContains(t, binding["foo"].Resources, unfitRes1.ID)
	require.NotContains(t, binding["bar"].Resources, unfitRes1.ID)
	require.NotContains(t, binding["bar"].Resources, unfitRes2.ID)
}
