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
	"fmt"
	"os"
	"io/ioutil"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/golang/glog"

	runtimeapi "k8s.io/kubernetes/pkg/kubelet/apis/cri/v1alpha1/runtime"
	dockertypes "github.com/docker/docker/api/types"

	"k8s.io/kubernetes/pkg/kubelet/dockershim/libdocker"
)

type DockerHookService interface {
	Start() error
	Stop() error

	GetRuntime(*dockertypes.ImageInspect, *runtimeapi.ContainerConfig, *runtimeapi.PodSandboxConfig) string
}

type HookStore interface {
	Store([]dockerHook)
	Get() []dockerHook
}

type dockerHook struct {
	Runtime     string            `json:"runtime"`
	Annotations map[string]string `json:"annotations"`
	Images      []string          `json:"images"`
}

type dockerHookService struct {
	hooksDir string
	watch    *fsnotify.Watcher

	stop chan interface{}
	wg   sync.WaitGroup

	store  HookStore
	client libdocker.Interface
}

func newDockerHookService(hooksDir string, client libdocker.Interface) (*dockerHookService, error) {
	return newDockerHookServiceWithStore(hooksDir, client, newDockerHookStore())
}

func newDockerHookServiceWithStore(hooksDir string, client libdocker.Interface, store HookStore) (*dockerHookService, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	return &dockerHookService{
		hooksDir: hooksDir,
		watch:    watcher,

		stop:  make(chan interface{}),
		store: store,
		client: client,
	}, nil
}

func (dh *dockerHookService) Start() error {
	glog.Infof("Starting Docker Hooks watcher")

	if err := os.MkdirAll(dh.hooksDir, 0644); err != nil {
		return err
	}

	err := dh.watch.Add(dh.hooksDir)
	if err != nil {
		return err
	}

	dh.wg.Add(2)

	go func() {
		dh.watch.Events <- fsnotify.Event{Name: "Startup", Op: fsnotify.Create}
		dh.wg.Done()
	}()

	go func() {
		for {
			select {
			case <-dh.watch.Events: // reloads hooks everytime
				glog.V(2).Infof("Got Docker Hooks event")
				dh.loadHooks()

			// TODO recreate the watch
			case err := <-dh.watch.Errors:
				glog.Errorf("got fsnotify error: %v", err)

			case <-dh.stop:
				dh.wg.Done()
				return
			}
		}
	}()

	return nil
}

func (dh *dockerHookService) Stop() error {
	glog.V(2).Infof("Stopping Hook service")

	close(dh.stop)
	if err := dh.waitTimeout(); err != nil {
		return err
	}

	dh.watch.Close()
	return nil
}

func (dh *dockerHookService) GetRuntime(image *dockertypes.ImageInspect, cfg *runtimeapi.ContainerConfig, sandbox *runtimeapi.PodSandboxConfig) string {
	glog.V(4).Infof("Got reposTags: %+v, image %+v and annotations %v", image.RepoTags, *image, cfg.Annotations)

	for _, h := range dh.store.Get() {
		for k, v := range h.Annotations {
			if a, ok := cfg.Annotations[k]; ok && a == v {
				return h.Runtime
			}
		}

		for _, hookImage := range h.Images {
			for _, tag := range image.RepoTags {
				if strings.HasPrefix(tag, hookImage) {
					glog.V(2).Infof("Matched runtime %s", h.Runtime)
					return h.Runtime
				}
			}
		}
	}

	return ""
}

func (dh *dockerHookService) loadHooks() error {
	files, err := ioutil.ReadDir(dh.hooksDir)
	if err != nil {
		return fmt.Errorf("Failed to readdir with err: %v", err)
	}

	var hooks []dockerHook

	for _, f := range files {
		h, err := dh.readHook(filepath.Join(dh.hooksDir, f.Name()))
		if err != nil {
			glog.Errorf("error %v while reading hook %q", err, f.Name())
			continue
		}

		if err := dh.isHookValid(h); err != nil {
			glog.Errorf("error %v while validating hook %q", err, f.Name())
			continue
		}

		glog.Infof("Adding hook: %+v from file %s", h, f.Name())
		hooks = append(hooks, h)
	}

	dh.store.Store(hooks)

	return nil
}

func (dh *dockerHookService) readHook(fName string) (dockerHook, error) {
	var hook dockerHook

	if !strings.HasSuffix(fName, ".json") {
		return hook, fmt.Errorf("File %s doesn't end with .json", fName)
	}

	raw, err := ioutil.ReadFile(fName)
	if err != nil {
		return hook, err
	}

	if err := json.Unmarshal(raw, &hook); err != nil {
		return hook, err
	}

	return hook, nil
}

func (dh *dockerHookService) isHookValid(h dockerHook) (error) {
	info, err := dh.client.Info()
	if err != nil {
		return err
	}

	if _, ok := info.Runtimes[h.Runtime]; !ok && h.Runtime != "" {
		return fmt.Errorf("Docker does not advertise runtime '%s' for hook", h.Runtime)
	}

	return nil
}

func (ds *dockerHookService) waitTimeout() error {
	c := make(chan interface{})
	go func() {
		defer close(c)
		ds.wg.Wait()
	}()

	select {
	case <-c:
		return nil
	case <-time.After(time.Second):
		return fmt.Errorf("Timeout while stopping Hook Service")
	}
}

type dockerHookStore struct {
	m     sync.Mutex
	hooks []dockerHook
}

func newDockerHookStore() *dockerHookStore {
	return &dockerHookStore{}
}

func (ds *dockerHookStore) Store(hooks []dockerHook) {
	ds.m.Lock()
	defer ds.m.Unlock()

	ds.hooks = hooks
}

func (ds *dockerHookStore) Get() []dockerHook {
	ds.m.Lock()
	defer ds.m.Unlock()

	var hooks []dockerHook
	for _, h := range ds.hooks {
		hooks = append(hooks, h)
	}

	return hooks
}
