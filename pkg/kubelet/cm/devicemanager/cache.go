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
	"sync"

	"k8s.io/api/core/v1"
	kubecontainer "k8s.io/kubernetes/pkg/kubelet/container"
)

type cache interface {
	CachePodResources(p *v1.Pod, r *AdmitResponse)
	PodResources(p *v1.Pod) *kubecontainer.RunPodOptions
	ListPods() map[string]struct{}

	// Deletes the pod
	DeletePod(uid string) error
}

type cacheImpl struct {
	// pod UID to RunPodOptions
	podResources map[string]*kubecontainer.RunPodOptions
	m            sync.Mutex
}

func newCacheImpl() *cacheImpl {
	return &cacheImpl{
		podResources: map[string]*kubecontainer.RunPodOptions{},
	}
}

func (c *cacheImpl) CachePodResources(p *v1.Pod, r *AdmitResponse) {
	runOptions := &kubecontainer.RunPodOptions{}

	for k, v := range r.Pod.Annotations {
		runOptions.Annotations = append(runOptions.Annotations,
			kubecontainer.Annotation{
				Name:  k,
				Value: v,
			},
		)
	}

	c.m.Lock()
	defer c.m.Unlock()

	c.podResources[string(p.UID)] = runOptions

}

func (c *cacheImpl) PodResources(p *v1.Pod) *kubecontainer.RunPodOptions {
	c.m.Lock()
	defer c.m.Unlock()

	runOptions, ok := c.podResources[string(p.UID)]
	if !ok {
		return nil
	}

	return runOptions
}

func (c *cacheImpl) ListPods() map[string]struct{} {
	pods := map[string]struct{}{}

	c.m.Lock()
	defer c.m.Unlock()

	for k := range c.podResources {
		pods[k] = struct{}{}
	}

	return pods
}

func (c *cacheImpl) DeletePod(uid string) error {
	c.m.Lock()
	defer c.m.Unlock()

	if _, ok := c.podResources[uid]; !ok {
		return nil
	}

	delete(c.podResources, uid)
	return nil
}
