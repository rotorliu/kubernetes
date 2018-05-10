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
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestInstrumentedEndpointStore(t *testing.T) {
	outChan := make(chan interface{})
	continueChan := make(chan bool)

	iStore := newInstrumentedEndpointStoreShim(outChan, continueChan)
	store := iStore.store

	ie, iok := iStore.Endpoint("foo")
	e, ok := store.Endpoint("foo")

	require.Equal(t, ie, e)
	require.Equal(t, iok, ok)

	ierr := iStore.DeleteEndpoint("foo")
	err := store.DeleteEndpoint("foo")

	require.Equal(t, ierr, err)

	go func() {
		select {
		case <-outChan:
			break
		case <-time.After(time.Second):
			t.FailNow()
		}

		continueChan <- true
	}()

	e = newTestEndpoint("foo")
	ie, iok = iStore.SwapEndpoint(e)
	require.False(t, iok)
	require.Nil(t, ie)

	go func() {
		select {
		case <-outChan:
			break
		case <-time.After(time.Second):
			t.FailNow()
		}

		continueChan <- true
	}()

	ie2, iok := iStore.SwapEndpoint(newTestEndpoint("foo"))
	require.True(t, iok)
	require.Equal(t, ie2, e)

	var endpoints []endpoint
	iStore.Range(func(k string, e endpoint) {
		endpoints = append(endpoints, e)
	})

	require.Len(t, endpoints, 1)
	require.Equal(t, endpoints[0].ResourceName(), "foo")
}
