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
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/golang/glog"
	"golang.org/x/net/context"
	"google.golang.org/grpc"

	pluginapi "k8s.io/kubernetes/pkg/kubelet/apis/deviceplugin/v1alpha"
	pluginregistration "k8s.io/kubernetes/pkg/kubelet/apis/pluginregistration/v1beta"
)

type InitStubFunc func(r *InitRequest) *InitResponse

var stub_socket_dir = ""

// Stub implementation for DevicePlugin.
type DevicePluginStub struct {
	Devs   []*pluginapi.Device
	socket string
	rName  string

	stop       chan interface{}
	registered chan interface{}
	update     chan []*pluginapi.Device
	wg         sync.WaitGroup

	initFunc InitStubFunc

	server *grpc.Server
}

// NewDevicePluginStub returns an initialized DevicePlugin Stub.
func NewDevicePluginStub(socketName, rName string, devs []*pluginapi.Device) *DevicePluginStub {
	return NewDevicePluginStubWithInit(socketName, rName, devs, nil)
}

// NewDevicePluginStubWithInit returns an initialized DevicePlugin Stub.
func NewDevicePluginStubWithInit(socketName, rName string, devs []*pluginapi.Device, init InitStubFunc) *DevicePluginStub {
	return &DevicePluginStub{
		Devs:   devs,
		socket: socketName,
		rName:  rName,

		stop:       make(chan interface{}),
		registered: make(chan interface{}),
		update:     make(chan []*pluginapi.Device),

		initFunc: init,
	}
}

func (m *DevicePluginStub) Name() string {
	return m.rName
}

func (m *DevicePluginStub) SocketName() string {
	name := strings.TrimPrefix(m.socket, stub_socket_dir)
	return strings.TrimPrefix(name, "/")
}

// Start starts the gRPC server of the device plugin
func (m *DevicePluginStub) Start() error {
	os.MkdirAll(filepath.Dir(m.socket), 0755)

	if err := m.cleanup(); err != nil {
		return err
	}

	sock, err := net.Listen("unix", m.socket)
	if err != nil {
		return err
	}

	m.server = grpc.NewServer([]grpc.ServerOption{}...)
	pluginapi.RegisterDevicePluginServer(m.server, m)
	pluginregistration.RegisterIdentityServer(m.server, m)

	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		m.server.Serve(sock)
	}()

	// Wait for server to start by launching a blocking connexion
	c, err := dial(m.socket, 5*time.Second)
	if err != nil {
		return err
	}
	c.Close()

	glog.V(2).Infof("Starting to serve on %s", m.socket)

	return nil
}

// Stop stops the gRPC server
func (m *DevicePluginStub) Stop() error {
	glog.V(2).Infof("Stopping server %s", m.SocketName())

	m.server.Stop()
	close(m.stop)

	if err := m.waitTimeout(); err != nil {
		return err
	}

	return m.cleanup()
}

func (m *DevicePluginStub) GetSupportedVersions(ctx context.Context, in *VersionsRequest) (*VersionsResponse, error) {
	glog.V(2).Infof("%s: GetSupportedVersions", m.SocketName())

	return &VersionsResponse{
		SupportedVersions: []string{pluginapi.Version},
	}, nil
}

func (m *DevicePluginStub) GetPluginIdentity(ctx context.Context, in *IdentityRequest) (*IdentityResponse, error) {
	glog.V(2).Infof("%s: GetPluginIdentity", m.SocketName())

	return &IdentityResponse{
		ResourceName: m.rName,
	}, nil
}

func (m *DevicePluginStub) PluginRegistrationStatus(ctx context.Context, in *pluginregistration.RegistrationStatus) (*pluginregistration.Empty, error) {
	glog.V(2).Infof("%s: PluginRegistrationStatus was %v", m.SocketName(), in)
	if m.registered != nil {
		close(m.registered)
		m.registered = nil
	}

	return &pluginregistration.Empty{}, nil
}

func (m *DevicePluginStub) WaitForRegistration(timeout time.Duration) error {
	c := make(chan interface{})
	go func() {
		defer close(c)
		<-m.registered
	}()

	select {
	case <-c:
		return nil
	case <-time.After(timeout):
		return fmt.Errorf("timeout while waiting for plugin registration")
	}

	return nil
}

func (m *DevicePluginStub) GetPluginInfo(ctx context.Context, in *InfoRequest) (*InfoResponse, error) {
	glog.V(2).Infof("%s: GetPluginInfo", m.SocketName())

	return &InfoResponse{
		InitTimeout: 1,
		Labels:      map[string]string{},
	}, nil
}

// ListAndWatch lists devices and update that list according to the Update call
func (m *DevicePluginStub) ListAndWatch(e *pluginapi.ListAndWatchRequest, s ListAndWatchStream) error {
	glog.V(2).Infof("%s: ListAndWatch", m.SocketName())

	s.Send(&pluginapi.ListAndWatchResponse{Devices: m.Devs})

	for {
		select {
		case <-m.stop:
			return nil
		case updated := <-m.update:
			m.Devs = updated
			s.Send(&pluginapi.ListAndWatchResponse{Devices: m.Devs})
		}
	}
}

// Allocate does a mock allocation
func (m *DevicePluginStub) InitContainer(ctx context.Context, r *InitRequest) (*InitResponse, error) {
	glog.V(2).Infof("%s: InitContainer, %+v", m.SocketName(), r)

	response := &InitResponse{}
	if m.initFunc != nil {
		response = m.initFunc(r)
	}

	return response, nil
}

func (m *DevicePluginStub) AdmitPod(ctx context.Context, in *pluginapi.AdmitPodRequest) (*pluginapi.AdmitPodResponse, error) {
	return nil, nil
}

// Update allows the device plugin to send new devices through ListAndWatch
func (m *DevicePluginStub) Update(devs []*pluginapi.Device) {
	m.update <- devs
}

func (m *DevicePluginStub) cleanup() error {
	if err := os.Remove(m.socket); err != nil && !os.IsNotExist(err) {
		return err
	}

	return nil
}

func (m *DevicePluginStub) waitTimeout() error {
	c := make(chan interface{})
	go func() {
		defer close(c)
		m.wg.Wait()
	}()

	select {
	case <-c:
		return nil
	case <-time.After(time.Second):
		return fmt.Errorf("Timeout while stopping device plugin %s", m.SocketName())
	}

	return nil
}

func newInitStubFunc(devs map[string][]*pluginapi.DeviceSpec,
	mounts map[string][]*pluginapi.Mount,
	envs map[string]map[string]string) InitStubFunc {

	return func(r *InitRequest) *InitResponse {
		resp := &InitResponse{
			Spec: &pluginapi.ContainerSpec{
				Envs: make(map[string]string),
			},
		}

		for _, dev := range r.Container.Devices {
			for _, d := range devs[dev] {
				resp.Spec.Devices = append(resp.Spec.Devices, d)
			}

			for _, m := range mounts[dev] {
				resp.Spec.Mounts = append(resp.Spec.Mounts, m)
			}

			for k, v := range envs[dev] {
				resp.Spec.Envs[k] = v
			}
		}

		return resp
	}
}

// dial establishes the gRPC communication with the registered device plugin.
func dial(unixSocket string, timeout time.Duration) (*grpc.ClientConn, error) {
	c, err := grpc.Dial(unixSocket, grpc.WithInsecure(), grpc.WithBlock(),
		grpc.WithTimeout(timeout),
		grpc.WithDialer(func(addr string, timeout time.Duration) (net.Conn, error) {
			return net.DialTimeout("unix", addr, timeout)
		}),
	)

	if err != nil {
		return nil, fmt.Errorf("Failed to dial Device Plugin at %s: %v", unixSocket, err)
	}

	return c, nil
}
