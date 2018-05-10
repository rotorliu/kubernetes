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
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	pluginapi "k8s.io/kubernetes/pkg/kubelet/apis/deviceplugin/v1alpha"
)

func TestHandlerNewEndpoint(t *testing.T) {
	defer func() { require.NoError(t, os.RemoveAll(testPluginDir)) }()

	callbackCount := int32(0)
	callbackChan := make(chan int32, 1)
	callbackExpected := int32(0)

	// We expect this to be called twice:
	// - once at "registration" time (when devices are sent by the stub)
	// - once at "stop" time
	f := managerCallback(func(n string, a, u, r []pluginapi.Device) {
		if callbackCount > atomic.LoadInt32(&callbackExpected) {
			t.FailNow()
		}

		callbackCount++
		callbackChan <- callbackCount
	})

	hdlr := newEndpointHandlerImpl(f)
	defer hdlr.Stop()

	store := hdlr.Store()
	hdlr.SetStore(store)
	require.Equal(t, hdlr.Store(), store)

	p := NewDevicePluginStub(pluginSocketName, testResourceName, nil)

	require.NoError(t, p.Start())
	defer func() { require.NoError(t, p.Stop()) }()

	atomic.StoreInt32(&callbackExpected, 1)
	e, err := hdlr.NewEndpoint(pluginSocketName, testResourceName)
	require.NoError(t, err)

	select {
	case <-callbackChan:
		break
	case <-time.After(time.Second):
		t.FailNow()
	}

	e, ok := hdlr.Store().Endpoint(testResourceName)
	require.True(t, ok)

	atomic.StoreInt32(&callbackExpected, 2)
	require.NoError(t, e.Stop())

	select {
	case <-callbackChan:
		break
	case <-time.After(time.Second):
		t.FailNow()
	}
}

func TestTrackEndpoint(t *testing.T) {
	defer func() { require.NoError(t, os.RemoveAll(testPluginDir)) }()

	// setup
	p := NewDevicePluginStub(pluginSocketName, testResourceName, nil)
	require.NoError(t, p.Start())

	hdlr := newEndpointHandlerImpl(func(n string, a, u, r []pluginapi.Device) {})

	updateChan := make(chan interface{})
	dStore := newDeviceStoreImpl(func(n string, a, u, r []pluginapi.Device) {
		if updateChan == nil {
			return
		}

		close(updateChan)
	})

	c, err := dial(pluginSocketName, time.Second)
	require.NoError(t, err)

	eImpl, err := newEndpointWithStore(c, testResourceName, dStore)
	require.NoError(t, err)

	// insert endpoint in the store
	hdlr.store.SwapEndpoint(eImpl)
	e, ok := hdlr.Store().Endpoint(eImpl.resourceName)
	require.True(t, ok)

	gofuncChan := make(chan interface{})
	// stop the endpoint as soon as possible (here after sending the devices)
	// we can do this because p.Register was not called
	go func() {
		select {
		case <-updateChan:
			updateChan = nil
			break
		case <-time.After(time.Second):
			t.Fatalf("Device Plugin did not register")
		}

		e.Stop()
		p.Stop()

		close(gofuncChan)
	}()

	// Test that the endpoint was deleted
	hdlr.wg.Add(1)
	hdlr.trackEnpoint(e)
	select {
	case <-gofuncChan:
		break
	case <-time.After(time.Second):
		t.Fatalf("Callback channel was not closed")
	}

	_, ok = hdlr.Store().Endpoint(e.ResourceName())
	require.False(t, ok)

	// If the endpoint closed correctly waitgroup.Done should not fail
	require.NoError(t, hdlr.Stop())
}

// Tests that the device plugin manager correctly handles registration and re-registration by
// making sure that after registration, devices are correctly updated and if a re-registration
// happens, we will NOT delete devices.
func TestReRegistration(t *testing.T) {
	defer func() { require.NoError(t, os.RemoveAll(testPluginDir)) }()

	devs := []*pluginapi.Device{
		{ID: "Dev1", Health: pluginapi.Healthy},
		{ID: "Dev2", Health: pluginapi.Healthy},
	}

	callbackCount := int32(0)
	callbackChan := make(chan int32, 1)
	callbackExpected := int32(0)

	// We expect this to be called twice:
	// - once at "registration" time (when devices are sent by the stub)
	// - once at "stop" time
	f := func(n string, a, u, r []pluginapi.Device) {
		if callbackCount > atomic.LoadInt32(&callbackExpected) {
			t.FailNow()
		}

		callbackCount++
		callbackChan <- callbackCount
	}

	// setup
	hdlr := newEndpointHandlerImpl(f)
	defer hdlr.Stop()

	outChan := make(chan interface{})
	continueChan := make(chan bool)

	hdlr.SetStore(newInstrumentedEndpointStoreShim(outChan, continueChan))

	p1 := NewDevicePluginStub(pluginSocketName, testResourceName, devs)
	p2 := NewDevicePluginStub(pluginSocketName, testResourceName, devs)
	require.NoError(t, p1.Start())

	// Create first endpoint
	go func() {
		select {
		case <-outChan:
			continueChan <- true
			break
		case <-time.After(time.Second):
			t.FailNow()
		}
	}()

	atomic.StoreInt32(&callbackExpected, 1)
	hdlr.NewEndpoint(p1.socket, testResourceName)

	select {
	case <-callbackChan:
		break
	case <-time.After(time.Second):
		t.FailNow()
	}

	require.Len(t, hdlr.Devices(), 1)
	require.Len(t, hdlr.Devices()[testResourceName], 2)

	// Create second endpoint
	require.NoError(t, p2.Start())

	// Check that the old endpoint's store has not been removed before
	// swapping it with the new store. Also check that the new endpoint's
	// store points to the old endpoint's store.
	//
	// InstrumentedEndpoint allows to execute code before swap is called
	go func() {
		endpoints := (<-outChan).(swapMessage)

		require.NotNil(t, endpoints.Old)
		require.Len(t, endpoints.Old.Store().Devices(), 2)

		require.NotNil(t, endpoints.New)
		require.Equal(t, endpoints.New.Store(), endpoints.Old.Store())

		continueChan <- true
	}()

	atomic.StoreInt32(&callbackExpected, 2)
	hdlr.NewEndpoint(pluginSocketName, testResourceName)

	select {
	case <-callbackChan:
		break
	case <-time.After(time.Second):
		t.FailNow()
	}

	// Check that the callback is called at stop time
	e, ok := hdlr.Store().Endpoint(testResourceName)
	require.True(t, ok)

	atomic.StoreInt32(&callbackExpected, 3)
	e.Stop()

	select {
	case <-callbackChan:
		break
	case <-time.After(time.Second):
		t.FailNow()
	}

	require.NoError(t, p1.Stop())
	require.NoError(t, p2.Stop())

	close(outChan)
	close(continueChan)
	close(callbackChan)
}
