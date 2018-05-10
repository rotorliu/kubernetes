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
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/golang/glog"
	"github.com/stretchr/testify/require"

	"k8s.io/api/core/v1"
	pluginapi "k8s.io/kubernetes/pkg/kubelet/apis/deviceplugin/v1alpha"
)

const (
	pluginDir        = "/tmp/device_plugin/plugins/"
	testResourceName = "fake-domain.com/resource"
)

var (
	testPluginDir    = ""
	pluginSocketName = ""
)

func init() {
	var logLevel string

	flag.Set("alsologtostderr", fmt.Sprintf("%t", true))
	flag.StringVar(&logLevel, "logLevel", "6", "test")
	flag.Lookup("v").Value.Set(logLevel)
}

func TestMain(m *testing.M) {
	var err error

	testPluginDir, err = ioutil.TempDir("", "")
	if err != nil {
		glog.Errorf("could not create temp dir with error: %v", err)
		os.Exit(1)
	}
	pluginSocketName = filepath.Join(testPluginDir, "fake-domain.com/resource.sock")
	stub_socket_dir = testPluginDir

	ret := m.Run()

	err = os.RemoveAll(testPluginDir)
	if err != nil {
		glog.Errorf("could not create temp dir with error: %v", err)
		os.Exit(1)
	}

	os.Exit(ret)
}

func cleanupPlugins(pluginDir string) error {
	glog.V(2).Infof("Cleaning up path %s", pluginDir)

	return filepath.Walk(pluginDir, func(path string, f os.FileInfo, err error) error {
		if path == pluginDir {
			return nil
		}

		return os.RemoveAll(path)
	})
}

func TestManagerPluginHandling(t *testing.T) {
	defer func() { require.NoError(t, cleanupPlugins(testPluginDir)) }()

	mgr, err := NewManagerImplWithPluginDir(testPluginDir)
	require.NoError(t, err)

	deviceStoreShim := newManagerStoreShim(mgr.deviceStore)
	mgr.setStore(deviceStoreShim)

	require.NoError(t, mgr.Start(activePodsStub, &sourcesReadyStub{}))
	defer func() { require.NoError(t, mgr.Stop()) }()

	resourceName1 := "nvidia.com/gpu"
	resourceName2 := "nvidia.com/tesla"
	devs1 := []*pluginapi.Device{
		{ID: "Device1", Health: pluginapi.Healthy},
		{ID: "Device2", Health: pluginapi.Healthy},
		{ID: "Device3", Health: pluginapi.Unhealthy},
	}
	devs2 := []*pluginapi.Device{
		{ID: "Device4", Health: pluginapi.Healthy},
		{ID: "Device5", Health: pluginapi.Unhealthy},
		{ID: "Device6", Health: pluginapi.Healthy},
		{ID: "Device7", Health: pluginapi.Unhealthy},
	}

	plugin1 := NewDevicePluginStub(filepath.Join(testPluginDir, resourceName1), resourceName1, devs1)
	plugin2 := NewDevicePluginStub(filepath.Join(testPluginDir, resourceName2), resourceName2, devs2)

	glog.V(2).Infof("Adding and starting Endpoints")
	require.NoError(t, plugin1.Start())
	require.NoError(t, plugin1.WaitForRegistration(time.Second))
	require.NoError(t, deviceStoreShim.WaitForUpdate(time.Second))

	require.NoError(t, plugin2.Start())
	require.NoError(t, plugin2.WaitForRegistration(time.Second))
	require.NoError(t, deviceStoreShim.WaitForUpdate(time.Second))

	glog.V(2).Infof("Checking Capacity")
	res, removed := mgr.GetCapacity()
	require.Len(t, res, 2)
	require.Len(t, removed, 0)

	rName1 := v1.ResourceName(plugin1.Name())
	rName2 := v1.ResourceName(plugin2.Name())
	require.Contains(t, res, rName1)
	require.Contains(t, res, rName2)
	require.Len(t, res[rName1].Resources, len(plugin1.Devs))
	require.Len(t, res[rName2].Resources, len(plugin2.Devs))

	glog.V(2).Infof("Stoping Endpoint 1 should change capacity")
	require.NoError(t, plugin1.Stop())
	require.NoError(t, deviceStoreShim.WaitForUpdate(time.Second))

	res, removed = mgr.GetCapacity()
	require.Len(t, res, 1)
	require.Contains(t, res, rName2)
	require.Len(t, res[rName2].Resources, len(plugin2.Devs))

	require.Len(t, removed, 1)
	require.Contains(t, removed, plugin1.Name())

	glog.V(2).Infof("Updating Endpoint2's devices")
	glog.V(2).Infof("Deleting an unhealthy device should change capacity")
	plugin2.Update(plugin2.Devs[:2])
	require.NoError(t, deviceStoreShim.WaitForUpdate(time.Second))
	res, removed = mgr.GetCapacity()
	require.Len(t, res, 1)
	require.Len(t, removed, 0)
	require.Contains(t, res, rName2)
	require.Len(t, res[rName2].Resources, 2)

	glog.V(2).Infof("Deleting a healthy device should change capacity")
	plugin2.Update(plugin2.Devs[:1])
	require.NoError(t, deviceStoreShim.WaitForUpdate(time.Second))
	res, removed = mgr.GetCapacity()
	require.Len(t, res, 1)
	require.Len(t, removed, 0)
	require.Contains(t, res, rName2)
	require.Len(t, res[rName2].Resources, 1)

	glog.V(2).Infof("Cleaning up")
	mgr.deviceStore = deviceStoreShim.store
	require.NoError(t, plugin2.Stop())
}

func TestManagerReRegistration(t *testing.T) {
	defer func() { require.NoError(t, cleanupPlugins(testPluginDir)) }()

	mgr, err := NewManagerImplWithPluginDir(testPluginDir)
	require.NoError(t, err)

	deviceStoreShim := newManagerStoreShim(mgr.deviceStore)
	mgr.setStore(deviceStoreShim)

	require.NoError(t, mgr.Start(activePodsStub, &sourcesReadyStub{}))
	defer func() { require.NoError(t, mgr.Stop()) }()

	resourceName1 := "nvidia.com/gpu"
	resourceName2 := "nvidia.com/tesla"
	devs1 := []*pluginapi.Device{
		{ID: "Device1", Health: pluginapi.Healthy},
		{ID: "Device2", Health: pluginapi.Healthy},
		{ID: "Device3", Health: pluginapi.Unhealthy},
	}
	devs2 := []*pluginapi.Device{
		{ID: "Device4", Health: pluginapi.Healthy},
		{ID: "Device5", Health: pluginapi.Unhealthy},
		{ID: "Device6", Health: pluginapi.Healthy},
		{ID: "Device7", Health: pluginapi.Unhealthy},
	}

	plugin1 := NewDevicePluginStub(filepath.Join(testPluginDir, resourceName1), resourceName1, devs1)
	plugin2 := NewDevicePluginStub(filepath.Join(testPluginDir, resourceName2), resourceName1, devs2)

	glog.V(2).Infof("Adding and starting Endpoints")
	require.NoError(t, plugin1.Start())
	require.NoError(t, plugin1.WaitForRegistration(time.Second))
	require.NoError(t, deviceStoreShim.WaitForUpdate(time.Second))

	require.NoError(t, plugin2.Start())
	require.NoError(t, plugin2.WaitForRegistration(time.Second))
	require.NoError(t, deviceStoreShim.WaitForUpdate(time.Second))

	glog.V(2).Infof("Making sure that only one Update call was issued during re-registration and it did not delete the devices")
	rName1 := v1.ResourceName(plugin1.Name())
	res, removed := mgr.GetCapacity()
	require.Len(t, res, 1)
	require.Len(t, removed, 0)
	require.Contains(t, res, rName1)
	require.Len(t, res[rName1].Resources, len(devs2))

	glog.V(2).Infof("Cleaning up")
	mgr.deviceStore = deviceStoreShim.store
	require.NoError(t, plugin2.Stop())
	require.NoError(t, plugin1.Stop())
}

type ManagerStoreShim struct {
	store  managerStore
	update chan interface{}
}

func newManagerStoreShim(store managerStore) *ManagerStoreShim {
	return &ManagerStoreShim{
		store:  store,
		update: make(chan interface{}, 25), // prevent another error when stopping because of another error
	}
}
func (s *ManagerStoreShim) UpdateCapacity(res string, a, u, d []pluginapi.Device) {
	s.store.UpdateCapacity(res, a, u, d)
	s.update <- true
}
func (s *ManagerStoreShim) GetCapacity() (capacity v1.ExtendedResourceMap, removed []string) {
	return s.store.GetCapacity()
}
func (s *ManagerStoreShim) HasDevices(res string, ids []string) error {
	return s.store.HasDevices(res, ids)
}
func (s *ManagerStoreShim) WaitForUpdate(t time.Duration) error {
	c := make(chan interface{})
	go func() {
		defer close(c)
		<-s.update
	}()

	select {
	case <-c:
		return nil
	case <-time.After(t):
		return fmt.Errorf("timeout while waiting for manager store update")
	}

	return nil
}

func constructInitResponse(devices, mounts, envs map[string]string) *InitResponse {
	resp := &InitResponse{}

	for k, v := range devices {
		resp.Spec.Devices = append(resp.Spec.Devices, &pluginapi.DeviceSpec{
			HostPath:      k,
			ContainerPath: v,
			Permissions:   "mrw",
		})
	}

	for k, v := range mounts {
		resp.Spec.Mounts = append(resp.Spec.Mounts, &pluginapi.Mount{
			ContainerPath: k,
			HostPath:      v,
			ReadOnly:      true,
		})
	}

	resp.Spec.Envs = make(map[string]string)
	for k, v := range envs {
		resp.Spec.Envs[k] = v
	}

	return resp
}

type mockEndpoint struct {
	resourceName string

	InitContainerFunc func(req *InitRequest) (*InitResponse, error)
	AdmitPodFunc      func(req *AdmitRequest) error
}

func (m *mockEndpoint) Run()        {}
func (m *mockEndpoint) Stop() error { return nil }

func (m *mockEndpoint) Store() deviceStore     { return nil }
func (m *mockEndpoint) SetStore(d deviceStore) {}
func (m *mockEndpoint) ResourceName() string {
	return m.resourceName
}

func (m *mockEndpoint) AdmitPod(req *AdmitRequest) error {
	if m.AdmitPodFunc != nil {
		return m.AdmitPodFunc(req)
	}

	return nil
}

func (m *mockEndpoint) InitContainer(req *InitRequest) (*InitResponse, error) {
	if m.InitContainerFunc != nil {
		return m.InitContainerFunc(req)
	}

	return nil, nil
}

func newDeviceSpec(containerPath, hostPath, permissions string) *pluginapi.DeviceSpec {
	return &pluginapi.DeviceSpec{
		ContainerPath: containerPath,
		HostPath:      hostPath,
		Permissions:   permissions,
	}
}

func newMount(containerPath, hostPath string, readonly bool) *pluginapi.Mount {
	return &pluginapi.Mount{
		ContainerPath: containerPath,
		HostPath:      hostPath,
		ReadOnly:      readonly,
	}
}

func newDevices(ids ...string) (devs []*pluginapi.Device) {
	for _, id := range ids {
		devs = append(devs, &pluginapi.Device{
			ID:     id,
			Health: pluginapi.Healthy,
		})
	}

	return devs
}
