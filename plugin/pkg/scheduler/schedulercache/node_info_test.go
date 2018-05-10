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
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

func newRes(id, health string) v1.ExtendedResource {
	return v1.ExtendedResource{
		ID:     id,
		Health: health,
	}
}

func TestSetNode(t *testing.T) {
	ni := NewNodeInfo()

	rName := v1.ResourceName("nvidia.com/gpu")
	rName2 := v1.ResourceName("nvidia.com/bar")
	dev1 := newRes("someId1", v1.ExtendedResourcesHealthy)
	dev2 := newRes("someId2", v1.ExtendedResourcesHealthy)

	n := v1.Node{
		Status: v1.NodeStatus{
			ExtendedResources: v1.ExtendedResourceMap{
				rName: v1.ExtendedResourceDomain{
					Resources: map[string]v1.ExtendedResource{
						"someId1": dev1,
						"someId2": dev2,
					},
				},
			},
		},
	}

	err := ni.SetNode(&n)
	require.NoError(t, err)

	require.Len(t, ni.Available(), 1)
	require.Contains(t, ni.Available(), rName)
	require.Len(t, ni.Available()[rName].Resources, 2)

	n = v1.Node{
		Status: v1.NodeStatus{
			ExtendedResources: v1.ExtendedResourceMap{
				rName: v1.ExtendedResourceDomain{
					Resources: map[string]v1.ExtendedResource{
						"someId2": dev2,
						"someId3": dev2,
					},
				},
				rName2: v1.ExtendedResourceDomain{
					Resources: map[string]v1.ExtendedResource{
						"someId1": dev1,
					},
				},
			},
		},
	}
	err = ni.SetNode(&n)
	require.NoError(t, err)

	require.Len(t, ni.Available(), 2)
	require.Contains(t, ni.Available(), rName)
	require.Contains(t, ni.Available(), rName2)
	require.Len(t, ni.Available()[rName].Resources, 2)
	require.NotContains(t, ni.Available()[rName].Resources, "someId1")
	require.Len(t, ni.Available()[rName2].Resources, 1)

	n = v1.Node{
		Status: v1.NodeStatus{
			ExtendedResources: v1.ExtendedResourceMap{
				rName2: v1.ExtendedResourceDomain{
					Resources: map[string]v1.ExtendedResource{
						"someId1": dev1,
					},
				},
			},
		},
	}
	err = ni.SetNode(&n)
	require.NoError(t, err)

	require.Contains(t, ni.Available(), rName)
	require.Len(t, ni.Available()[rName].Resources, 0)
	require.Contains(t, ni.Available(), rName2)
	require.Len(t, ni.Available()[rName2].Resources, 1)
}

func TestAddAndRemovePodExtendedResources(t *testing.T) {
	ni := NewNodeInfo()

	rName1 := v1.ResourceName("nvidia.com/gpu")
	rName2 := v1.ResourceName("nvidia.com/bar")
	dev := newRes("someId1", v1.ExtendedResourcesHealthy)

	resMap := v1.ExtendedResourceMap{
		rName1: v1.ExtendedResourceDomain{Resources: map[string]v1.ExtendedResource{}},
		rName2: v1.ExtendedResourceDomain{Resources: map[string]v1.ExtendedResource{}},
	}

	for i := 0; i < 32; i++ {
		resMap[rName1].Resources[fmt.Sprintf("someId%d", i)] = dev
		resMap[rName2].Resources[fmt.Sprintf("someId%d", i)] = dev
	}

	err := ni.SetNode(&v1.Node{
		Status: v1.NodeStatus{
			ExtendedResources: resMap,
		},
	})
	require.NoError(t, err)

	var pods []v1.Pod
	for i := 0; i < 16; i++ {
		p := v1.Pod{
			Spec: v1.PodSpec{
				Containers: []v1.Container{
					{ExtendedResourceRequests: []string{"foo"}},
				},
				ExtendedResources: []v1.PodExtendedResource{
					{
						Name: "foo",
						Resources: v1.ResourceRequirements{
							Limits: v1.ResourceList{
								rName1: resource.MustParse("1"),
							},
						},
						Assigned: []string{fmt.Sprintf("someId%d", i)},
					},
				},
			},
		}

		ni.AddPod(&p)
		pods = append(pods, p)
	}

	for i := 16; i < 24; i++ {
		p := v1.Pod{
			Spec: v1.PodSpec{
				Containers: []v1.Container{
					{ExtendedResourceRequests: []string{"foo"}},
				},
				ExtendedResources: []v1.PodExtendedResource{
					{
						Name: "foo",
						Resources: v1.ResourceRequirements{
							Limits: v1.ResourceList{
								rName1: resource.MustParse("2"),
							},
						},
						Assigned: []string{
							fmt.Sprintf("someId%d", i),
							fmt.Sprintf("someId%d", i+8),
						},
					},
				},
			},
		}

		ni.AddPod(&p)
		pods = append(pods, p)
	}

	for _, p := range pods {
		ni.RemovePod(&p)
	}

	require.Len(t, ni.Available(), 2)
	require.Contains(t, ni.Available(), rName1)
	require.Contains(t, ni.Available(), rName2)
	require.Len(t, ni.Available()[rName1].Resources, 32)
	require.Len(t, ni.Available()[rName2].Resources, 32)
}
