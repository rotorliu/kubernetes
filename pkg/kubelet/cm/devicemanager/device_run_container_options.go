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
	"github.com/golang/glog"
	kubecontainer "k8s.io/kubernetes/pkg/kubelet/container"
)

func DeviceRunContainerOptionsFromInitResponses(responses []*InitResponse) *kubecontainer.RunContainerOptions {
	opts := &kubecontainer.RunContainerOptions{}

	// Maps to detect duplicate settings.
	devsMap := make(map[string]string)
	mountsMap := make(map[string]string)
	envsMap := make(map[string]string)
	annotationsMap := make(map[string]string)

	// Loops through AllocationResponses of all cached device resources.
	for _, resp := range responses {
		for k, v := range resp.Spec.Envs {
			if e, ok := envsMap[k]; ok {
				glog.V(4).Infof("skip existing env %s %s", k, v)
				if e != v {
					glog.Errorf("Environment variable %s has conflicting setting: %s and %s", k, e, v)
				}

				continue
			}

			glog.V(4).Infof("add env %s %s", k, v)
			envsMap[k] = v
			opts.Envs = append(opts.Envs, kubecontainer.EnvVar{Name: k, Value: v})
		}

		// Updates RunContainerOptions.Devices.
		for _, dev := range resp.Spec.Devices {
			if d, ok := devsMap[dev.ContainerPath]; ok {
				glog.V(4).Infof("skip existing device %s %s", dev.ContainerPath, dev.HostPath)
				if d != dev.HostPath {
					glog.Errorf("Container device %s has conflicting mapping host devices: %s and %s",
						dev.ContainerPath, d, dev.HostPath)
				}

				continue
			}

			glog.V(4).Infof("add device %s %s", dev.ContainerPath, dev.HostPath)
			devsMap[dev.ContainerPath] = dev.HostPath
			opts.Devices = append(opts.Devices, kubecontainer.DeviceInfo{
				PathOnHost:      dev.HostPath,
				PathInContainer: dev.ContainerPath,
				Permissions:     dev.Permissions,
			})
		}

		// Updates RunContainerOptions.Mounts.
		for _, mount := range resp.Spec.Mounts {
			if m, ok := mountsMap[mount.ContainerPath]; ok {
				glog.V(4).Infof("skip existing mount %s %s", mount.ContainerPath, mount.HostPath)
				if m != mount.HostPath {
					glog.Errorf("Container mount %s has conflicting mapping host mounts: %s and %s",
						mount.ContainerPath, m, mount.HostPath)
				}

				continue
			}

			glog.V(4).Infof("add mount %s %s", mount.ContainerPath, mount.HostPath)
			mountsMap[mount.ContainerPath] = mount.HostPath
			opts.Mounts = append(opts.Mounts, kubecontainer.Mount{
				Name:          mount.ContainerPath,
				ContainerPath: mount.ContainerPath,
				HostPath:      mount.HostPath,
				ReadOnly:      mount.ReadOnly,
				// TODO: This may need to be part of Device plugin API.
				SELinuxRelabel: false,
			})
		}

		// Updates for Annotations
		for k, v := range resp.Spec.Annotations {
			if e, ok := annotationsMap[k]; ok {
				glog.V(4).Infof("skip existing annotation %s %s", k, v)
				if e != v {
					glog.Errorf("Annotation %s has conflicting setting: %s and %s", k, e, v)
				}

				continue
			}

			glog.V(4).Infof("add annotation %s %s", k, v)
			annotationsMap[k] = v
			opts.Annotations = append(opts.Annotations, kubecontainer.Annotation{Name: k, Value: v})
		}
	}

	return opts
}
