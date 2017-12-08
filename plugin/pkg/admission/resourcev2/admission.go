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

package resourcev2

import (
	"fmt"
	"io"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apiserver/pkg/admission"
	"k8s.io/kubernetes/pkg/apis/core"

	"k8s.io/apimachinery/pkg/util/uuid"
)

// Register is called by the apiserver to register the plugin factory.
func Register(plugins *admission.Plugins) {
	plugins.Register("ResourceV2", func(config io.Reader) (admission.Interface, error) {
		return newResourceV2(), nil
	})
}

type plugin struct {
	*admission.Handler
}

// Make sure we are implementing the interface.
var _ admission.MutationInterface = &plugin{}

func newResourceV2() *plugin {
	return &plugin{
		Handler: admission.NewHandler(admission.Create, admission.Update),
	}
}

func (p *plugin) Admit(attributes admission.Attributes) error {
	// Ignore all calls to subresources or resources other than pods.
	if len(attributes.GetSubresource()) != 0 || attributes.GetResource().GroupResource() != core.Resource("pods") {
		return nil
	}

	pod, ok := attributes.GetObject().(*core.Pod)
	if !ok {
		return errors.NewBadRequest(fmt.Sprintf("expected *core.Pod but got %T", attributes.GetObject()))
	}

	for i, container := range pod.Spec.InitContainers {
		for resourceName, val := range container.Resources.Limits {
			if resourceName != core.ResourceName("nvidia.com/gpu") {
				continue
			}

			name, eRes := newExtendedResource(resourceName, val)
			pod.Spec.InitContainers[i].ExtendedResourceRequests = []string{name}
			pod.Spec.ExtendedResources = append(pod.Spec.ExtendedResources, eRes)

			deleteRes(&pod.Spec.InitContainers[i].Resources, resourceName)
		}
	}

	for i, container := range pod.Spec.Containers {
		for resourceName, val := range container.Resources.Limits {

			if resourceName != core.ResourceName("nvidia.com/gpu") {
				continue
			}

			name, eRes := newExtendedResource(resourceName, val)
			pod.Spec.Containers[i].ExtendedResourceRequests = []string{name}
			pod.Spec.ExtendedResources = append(pod.Spec.ExtendedResources, eRes)

			deleteRes(&pod.Spec.Containers[i].Resources, resourceName)
		}
	}

	return nil
}

func newExtendedResource(rName core.ResourceName, val resource.Quantity) (string, core.PodExtendedResource) {
	name := string(uuid.NewUUID())

	return name, core.PodExtendedResource{
		Name: name,
		Resources: core.ResourceRequirements {
			Limits: core.ResourceList {
				rName: val,
			},
			Requests: core.ResourceList {
				rName: val,
			},
		},
	}
}

func deleteRes(resources *core.ResourceRequirements, rName core.ResourceName) {
	if _, ok := resources.Requests[rName]; ok {
		delete(resources.Requests, rName)
	}

	if _, ok := resources.Limits[rName]; ok {
		delete(resources.Limits, rName)
	}
}
