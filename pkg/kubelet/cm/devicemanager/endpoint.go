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
	"time"

	"github.com/golang/glog"
	"golang.org/x/net/context"
	"google.golang.org/grpc"

	pluginapi "k8s.io/kubernetes/pkg/kubelet/apis/deviceplugin/v1alpha"
)

// endpoint maps to a single registered device plugin. It is responsible
// for managing gRPC communications with the device plugin and caching
// device states reported by the device plugin.
type endpoint interface {
	Run()
	Stop() error

	AdmitPod(req *AdmitRequest) (*AdmitResponse, error)
	InitContainer(req *InitRequest) (*InitResponse, error)
	ResourceName() string

	Store() deviceStore
	SetStore(deviceStore)
}

type endpointImpl struct {
	client     pluginapi.DevicePluginClient
	clientConn *grpc.ClientConn

	resourceName string

	wg sync.WaitGroup

	initTimeout int64
	initMutex   sync.Mutex
	initCancel  context.CancelFunc

	storeMutex sync.Mutex
	devStore   deviceStore
}

// newEndpoint creates a new endpoint for the given resourceName.
func newEndpoint(clientConn *grpc.ClientConn, rName string) (*endpointImpl, error) {
	return newEndpointWithStore(clientConn, rName, nil)
}

// TODO move registration out of endpoint
func newEndpointWithStore(clientConn *grpc.ClientConn, rName string, devStore deviceStore) (*endpointImpl, error) {
	e := &endpointImpl{
		clientConn: clientConn,
		client:     pluginapi.NewDevicePluginClient(clientConn),

		resourceName: rName,

		devStore: devStore,
	}

	if err := e.init(); err != nil {
		return nil, err
	}

	return e, nil
}

func (e *endpointImpl) init() error {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	info, err := e.client.GetPluginInfo(ctx, &InfoRequest{})
	if err != nil {
		return err
	}

	e.initTimeout = info.InitTimeout

	return nil
}

func (e *endpointImpl) Run() {
	glog.V(3).Infof("Starting to run endpoint %s", e.resourceName)

	e.wg.Add(1)
	defer e.wg.Done()

	stream, err := e.client.ListAndWatch(context.Background(), &pluginapi.ListAndWatchRequest{})
	if err != nil {
		glog.Errorf("could not connect to Device Plugin %s: %+v", e.resourceName, err)
		return
	}

	for {
		response, err := stream.Recv()
		if err != nil {
			glog.Errorf("device plugin %s caugth error: %+v", e.resourceName, err)
			break
		}

		glog.V(2).Infof("Endpoint %s updated", e.resourceName)

		s := e.Store()
		added, updated, deleted := s.Update(response.Devices)
		s.Callback(e.resourceName, added, updated, deleted)
	}

	glog.V(2).Infof("Endpoint %s is stopping", e.resourceName)

	s := e.Store()

	s.Callback(e.resourceName, nil, nil, s.Devices())
	e.clientConn.Close()
}

// allocate issues Allocate gRPC call to the device plugin.
func (e *endpointImpl) InitContainer(req *InitRequest) (*InitResponse, error) {
	e.wg.Add(1)
	defer e.wg.Done()

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(e.initTimeout)*time.Second)

	e.SetInitCancel(cancel)

	response, err := e.client.InitContainer(ctx, req)

	cancel() // can and will be called twice, should not be a problem according to the docs
	e.SetInitCancel(nil)

	return response, err
}

func (e *endpointImpl) AdmitPod(req *AdmitRequest) (*AdmitResponse, error) {
	e.wg.Add(1)
	defer e.wg.Done()

	resp, err := e.client.AdmitPod(context.Background(), req)

	return resp, err
}

func (e *endpointImpl) Stop() error {
	glog.V(2).Infof("Stopping endpoint %s", e.resourceName)

	e.clientConn.Close()

	// only cancel if there is an Initialize call going on
	if cancel := e.InitCancel(); cancel != nil {
		cancel()
	}

	if err := e.waitTimeout(); err != nil {
		return err
	}

	return nil
}

func (e *endpointImpl) waitTimeout() error {
	c := make(chan interface{})
	go func() {
		defer close(c)
		e.wg.Wait()
	}()

	select {
	case <-c:
		return nil
	case <-time.After(time.Second):
		return fmt.Errorf("timeout while stopping endpoint %s", e.resourceName)
	}

	return nil
}

func (e *endpointImpl) Store() deviceStore {
	e.storeMutex.Lock()
	defer e.storeMutex.Unlock()

	return e.devStore
}

func (e *endpointImpl) SetStore(s deviceStore) {
	e.storeMutex.Lock()
	defer e.storeMutex.Unlock()

	e.devStore = s
}

func (e *endpointImpl) ResourceName() string {
	return e.resourceName
}

func (e *endpointImpl) InitCancel() context.CancelFunc {
	e.initMutex.Lock()
	defer e.initMutex.Unlock()

	return e.initCancel
}

func (e *endpointImpl) SetInitCancel(cancel context.CancelFunc) {
	e.initMutex.Lock()
	defer e.initMutex.Unlock()

	e.initCancel = cancel
}
