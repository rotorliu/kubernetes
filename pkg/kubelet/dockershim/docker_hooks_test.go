/*
Copyright 2016 The Kubernetes Authors.

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

package dockershim

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/golang/glog"
	"github.com/stretchr/testify/require"
	dockertypes "github.com/docker/docker/api/types"

	"k8s.io/kubernetes/pkg/kubelet/dockershim/libdocker"
	"k8s.io/apimachinery/pkg/util/clock"
)

func init() {
	var logLevel string

	flag.Set("alsologtostderr", fmt.Sprintf("%t", true))
	flag.StringVar(&logLevel, "logLevel", "4", "test")
	flag.Lookup("v").Value.Set(logLevel)
}

var testHooksDir = ""

func TestMain(m *testing.M) {
	var err error

	testHooksDir, err = ioutil.TempDir("", "")
	if err != nil {
		glog.Errorf("could not create temp dir with error: %v", err)
		os.Exit(1)
	}

	ret := m.Run()

	err = os.RemoveAll(testHooksDir)
	if err != nil {
		glog.Errorf("could not create temp dir with error: %v", err)
		os.Exit(1)
	}

	os.Exit(ret)
}

func cleanupHooks() error {
	return filepath.Walk(testHooksDir, func(path string, f os.FileInfo, err error) error {
		if path != testHooksDir {
			glog.Errorf("removing file %s:", path)

			return os.RemoveAll(path)
		}

		return nil
	})
}

func TestAddHook(t *testing.T) {
	defer func() { require.NoError(t, cleanupHooks()) }()
	hooks := []dockerHook{
		{
			Runtime:     "",
			Annotations: map[string]string{},
			Images:      []string{},
		},
		{
			Runtime:     "nvidia",
			Annotations: map[string]string{},
			Images:      []string{},
		},
		{
			Runtime:     "nvidia",
			Annotations: map[string]string{"foo": "bar"},
			Images:      []string{},
		},
		{
			Runtime:     "nvidia",
			Annotations: map[string]string{"foo": "bar"},
			Images:      []string{"nvidia-device-plugin"},
		},
	}

	fakeClock := clock.NewFakeClock(time.Time{})
	c := libdocker.NewFakeDockerClient().WithClock(fakeClock).WithVersion("1.11.2", "1.23")
	c.Information.Runtimes = map[string]dockertypes.Runtime{}
	c.Information.Runtimes["nvidia"] = dockertypes.Runtime{}

	shim := newHookStoreShim()
	hookService, err := newDockerHookServiceWithStore(testHooksDir, c, shim)
	require.NoError(t, err)

	require.NoError(t, hookService.Start())
	waitForUpdateOrFail(t, shim) // startup event

	for i, h := range hooks {
		require.NoError(t, writeHookJson(h, fmt.Sprintf("%d.json", i)))
		glog.V(2).Infof("Wrote file %d.json:", i)

		// Creating a file is two events: CREATE + WRITE
		for i := 0; i < 2; i++ {
			waitForUpdateOrFail(t, shim)
		}

		require.NoError(t, listEqual(hooks[:(i+1)], shim.Get()))
	}

	require.NoError(t, hookService.Stop())
	shim.Cleanup()
}

// Write file + start hook service == restart
func TestRestartHookService(t *testing.T) {
	defer func() { require.NoError(t, cleanupHooks()) }()
	hooks := []dockerHook{
		{
			Runtime:     "",
			Annotations: map[string]string{},
			Images:      []string{},
		},
		{
			Runtime:     "nvidia",
			Annotations: map[string]string{},
			Images:      []string{},
		},
		{
			Runtime:     "nvidia",
			Annotations: map[string]string{"foo": "bar"},
			Images:      []string{},
		},
		{
			Runtime:     "nvidia",
			Annotations: map[string]string{"foo": "bar"},
			Images:      []string{"nvidia-device-plugin"},
		},
	}

	for i, h := range hooks {
		require.NoError(t, writeHookJson(h, fmt.Sprintf("%d.json", i)))
		glog.V(2).Infof("Wrote file %d.json:", i)
	}

	shim := newHookStoreShim()

	fakeClock := clock.NewFakeClock(time.Time{})
	c := libdocker.NewFakeDockerClient().WithClock(fakeClock).WithVersion("1.11.2", "1.23")
	c.Information.Runtimes = map[string]dockertypes.Runtime{}
	c.Information.Runtimes["nvidia"] = dockertypes.Runtime{}

	hookService, err := newDockerHookServiceWithStore(testHooksDir, c, shim)
	require.NoError(t, err)

	require.NoError(t, hookService.Start())
	waitForUpdateOrFail(t, shim) // startup

	require.NoError(t, listEqual(hooks, shim.Get()))
	require.NoError(t, hookService.Stop())
	shim.Cleanup()
}

func TestRemoveHook(t *testing.T) {
	defer func() { require.NoError(t, cleanupHooks()) }()
	hooks := []dockerHook{
		{
			Runtime:     "",
			Annotations: map[string]string{},
			Images:      []string{},
		},
		{
			Runtime:     "nvidia",
			Annotations: map[string]string{},
			Images:      []string{},
		},
		{
			Runtime:     "nvidia",
			Annotations: map[string]string{"foo": "bar"},
			Images:      []string{},
		},
		{
			Runtime:     "nvidia",
			Annotations: map[string]string{"foo": "bar"},
			Images:      []string{"nvidia-device-plugin"},
		},
	}

	for i, h := range hooks {
		require.NoError(t, writeHookJson(h, fmt.Sprintf("%d.json", i)))
		glog.V(2).Infof("Wrote file %d.json:", i)
	}

	shim := newHookStoreShim()

	fakeClock := clock.NewFakeClock(time.Time{})
	c := libdocker.NewFakeDockerClient().WithClock(fakeClock).WithVersion("1.11.2", "1.23")
	c.Information.Runtimes = map[string]dockertypes.Runtime{}
	c.Information.Runtimes["nvidia"] = dockertypes.Runtime{}

	hookService, err := newDockerHookServiceWithStore(testHooksDir, c, shim)
	require.NoError(t, err)

	require.NoError(t, hookService.Start())
	waitForUpdateOrFail(t, shim) // startup
	require.NoError(t, listEqual(hooks, shim.Get()))

	for i := range hooks {
		fName := fmt.Sprintf("%d.json", i)

		os.Remove(filepath.Join(testHooksDir, fName))
		waitForUpdateOrFail(t, shim)

		require.NoError(t, listEqual(hooks[(i+1):], shim.Get()))
	}

	require.NoError(t, hookService.Stop())
	shim.Cleanup()
}

func writeHookJson(h dockerHook, fName string) error {
	dump, err := json.Marshal(h)
	if err != nil {
		return err
	}

	fName = filepath.Join(testHooksDir, fName)
	return ioutil.WriteFile(fName, dump, 0644)
}

func listEqual(expected, actual []dockerHook) error {
	if len(expected) != len(actual) {
		return fmt.Errorf("len(expected) = %d != len(actual) = %d", len(expected), len(actual))
	}

	for i, a := range actual {
		if !reflect.DeepEqual(a, expected[i]) {
			return fmt.Errorf("hook[%d] = %#v but expected %#v", i, a, expected[i])
		}
	}

	return nil
}

func waitForUpdateOrFail(t *testing.T, shim *hookStoreShim) {
	select {
	case <-shim.updated:
		break
	case <-time.After(time.Second):
		t.Fatal("Could not fetch update from shim")
	}
}

type hookStoreShim struct {
	store   *dockerHookStore
	updated chan bool
}

func newHookStoreShim() *hookStoreShim {
	return &hookStoreShim{
		store:   newDockerHookStore(),
		updated: make(chan bool),
	}
}

func (ds *hookStoreShim) Store(hooks []dockerHook) {
	ds.store.Store(hooks)
	ds.updated <- true
}

func (ds *hookStoreShim) Get() []dockerHook {
	return ds.store.Get()
}

func (ds *hookStoreShim) Cleanup() {
	glog.V(2).Infof("Closing channel")
	close(ds.updated)
}
