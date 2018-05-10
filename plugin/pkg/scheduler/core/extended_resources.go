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
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/golang/glog"

	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/util/workqueue"

	"k8s.io/kubernetes/pkg/apis/core/v1/helper"
	"k8s.io/kubernetes/plugin/pkg/scheduler/algorithm"
	"k8s.io/kubernetes/plugin/pkg/scheduler/algorithm/predicates"
	"k8s.io/kubernetes/plugin/pkg/scheduler/schedulercache"

	v1helper "k8s.io/kubernetes/pkg/apis/core/v1/helper"
)

// TODO We only need to check if there is a scalar resource
// We need to support both path:
// - The limits field per container (old way to request nvidia.com/gpu)
// - The new ExtendedResources field per pod
func GetExtendedResources(pod *v1.Pod, filtered []*v1.Node,
	nodeNameToInfo map[string]*schedulercache.NodeInfo,
	failedPredicateMap FailedPredicateMap) ([]*v1.Node,
	map[string]v1.ExtendedResourceBinding) {

	if len(pod.Spec.ExtendedResources) == 0 {
		return filtered, map[string]v1.ExtendedResourceBinding{}
	}

	var predicateResultLock sync.Mutex
	var resLock sync.Mutex
	var filteredLen int32
	refiltered := make([]*v1.Node, len(filtered))

	extendedResources := map[string]v1.ExtendedResourceBinding{}

	checkNode := func(i int) {
		nodeName := filtered[i].Name
		ni := nodeNameToInfo[nodeName]

		fits, binding, failed := hasExtendedResources(pod, ni)

		if fits {
			refiltered[atomic.AddInt32(&filteredLen, 1)-1] = filtered[i]

			resLock.Lock()
			extendedResources[nodeName] = binding
			resLock.Unlock()
		} else {
			predicateResultLock.Lock()
			failedPredicateMap[nodeName] = failed
			predicateResultLock.Unlock()
		}
	}

	workqueue.Parallelize(16, len(filtered), checkNode)
	refiltered = refiltered[:filteredLen]

	return refiltered, extendedResources
}

func hasExtendedResources(p *v1.Pod, nI *schedulercache.NodeInfo) (bool,
	v1.ExtendedResourceBinding, []algorithm.PredicateFailureReason) {

	available := nI.Available()

	binding := v1.ExtendedResourceBinding{}

	for _, pRes := range p.Spec.ExtendedResources {
		rName, err := v1helper.PodExtendedResourceName(&pRes)
		if err != nil {
			return false, binding, []algorithm.PredicateFailureReason{
				predicates.NewFailureReason(fmt.Sprintf("%v", err)),
			}
		}

		val := pRes.Resources.Limits[rName]
		res, f := allocateResources(rName, int64(val.Value()),
			pRes.Affinity, available)

		if len(f) != 0 {
			return false, v1.ExtendedResourceBinding{}, f
		}

		binding[pRes.Name] = v1.ExtendedResourceList{Resources: res}
		removeFromAvailable(available, rName, res)
	}

	return true, binding, nil
}

func allocateResources(rName v1.ResourceName, num int64, affinity v1.ExtendedResourceAffinity,
	available v1.ExtendedResourceMap) ([]string, []algorithm.PredicateFailureReason) {

	var selected []string
	domain, ok := available[rName]
	if !ok {
		return selected, []algorithm.PredicateFailureReason{
			predicates.NewFailureReason(fmt.Sprintf("Insufficient %v", rName))}
	}

	glog.V(5).Infof("Requesting %d of extended resource %v", num, rName)
	glog.V(5).Infof("Available %v", domain)

	for id, res := range domain.Resources {
		if int64(len(selected)) == num {
			break
		}

		ok, err := isDeviceAMatch(res, affinity)
		if err != nil {
			return selected, []algorithm.PredicateFailureReason{predicates.NewFailureReason(err.Error())}
		}

		if ok {
			selected = append(selected, id)
			continue
		}

		glog.V(5).Infof("Resource %v did not fit for affinity %+v", res, affinity)
	}

	if int64(len(selected)) != num {
		return selected, []algorithm.PredicateFailureReason{
			predicates.NewFailureReason(fmt.Sprintf("Insufficient %v", rName))}
	}

	return selected, nil
}

func isDeviceAMatch(res v1.ExtendedResource, affinity v1.ExtendedResourceAffinity) (bool, error) {
	if len(affinity.Required) == 0 {
		return true, nil
	}

	req, err := helper.ExtendedRequirementsAsSelector(affinity.Required)
	if err != nil {
		return false, err
	}

	if !req.Matches(labels.Set(res.Attributes)) {
		return false, nil
	}

	return true, nil
}

func removeFromAvailable(available v1.ExtendedResourceMap, rName v1.ResourceName, ids []string) error {
	if _, ok := available[rName]; !ok {
		return fmt.Errorf("resource %s is not available", rName)
	}

	for _, id := range ids {
		if _, ok := available[rName].Resources[id]; !ok {
			return fmt.Errorf("resource %s/%s is not available", rName, id)
		}

		delete(available[rName].Resources, id)
	}

	return nil
}
