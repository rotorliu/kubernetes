package pluginregistration

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/golang/glog"

	utilfs "k8s.io/kubernetes/pkg/util/filesystem"
)

type PluginWatcher interface {
	Start() error
	Stop() error

	// Out is the same chan returned by start
	Added() chan string
	Removed() chan string
}

type PluginWatcherImpl struct {
	pluginDir string

	addedChan     chan string
	removedChan   chan string

	addDirChan chan string // removed dirs watchs are auto-deleted
	stopChan   chan interface{}

	watch *fsnotify.Watcher
	fs    utilfs.Filesystem

	wg sync.WaitGroup
}

func NewPluginWatcherImpl(pluginDir string) (*PluginWatcherImpl, error) {
	if err := os.MkdirAll(pluginDir, 0755); err != nil {
		return nil, err
	}

	// Make sure the path is Absolute
	pluginDirAbs, err := filepath.Abs(pluginDir)
	if err != nil {
		return nil, err
	}

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	return &PluginWatcherImpl{
		pluginDir: pluginDirAbs,
		addedChan:     make(chan string),
		removedChan:   make(chan string),

		addDirChan: make(chan string),
		stopChan:       make(chan interface{}),

		fs:    &utilfs.DefaultFs{},
		watch: watcher,
	}, nil
}

func (p *PluginWatcherImpl) Start() error {

	p.wg.Add(1)
	go p.watchFsNotify()

	p.wg.Add(1)
	go p.watchDirectories()

	// This needs to be executed in sequential order because files might be added
	// after start and caught by both init and fsnotify
	if err := p.init(); err != nil {
		return err
	}

	glog.V(4).Infof("Starting to watch plugin directory: %+s", p.pluginDir)
	if err := p.watch.Add(p.pluginDir); err != nil {
		return err
	}


	return nil
}

func (p *PluginWatcherImpl) init() error {
	return filepath.Walk(p.pluginDir, func(path string, f os.FileInfo, err error) error {
		if path == p.pluginDir {
			return nil
		}

		if f.Mode().IsDir() {
			if filepath.Dir(path) != p.pluginDir {
				glog.Errorf("Directory %s should not be present", path)
				return nil
			}

			if err := p.watch.Add(path); err != nil {
				return err
			}

			return nil
		}

		if filepath.Dir(filepath.Dir(path)) != p.pluginDir {
			glog.Errorf("File %s should not be present", path)
			return nil
		}

		// This needs to be in a goroutine so that Start can return and start processing the events
		p.wg.Add(1)
		go func() {
			defer p.wg.Done()
			p.addedChan <- path
		}()

		return nil
	})
}

func (p *PluginWatcherImpl) watchFsNotify() {
	for {
		select {
		case event := <-p.watch.Events: // reloads hooks everytime
			glog.V(4).Infof("Got fsnotify event for plugin watcher: %+v", event)

			if utilfs.EventOpIs(event, fsnotify.Create) {
				if err := p.handleCreate(event.Name, event); err != nil {
					glog.Errorf("device plugin %s failed registration: %s", event.Name, err)
				}

				continue
			}

			// No choice but to send all events on the chan (including directories)
			if utilfs.EventOpIs(event, fsnotify.Remove) {
				p.removedChan <- event.Name

				continue
			}


		// TODO recreate the watch?
		case err := <-p.watch.Errors:
			glog.Errorf("caught fsnotify error for Device Manager: %s", err)

		case <-p.stopChan:
			p.wg.Done()
			return
		}
	}
}

func (p *PluginWatcherImpl) watchDirectories() {
	for {
		select {
		case path := <-p.addDirChan:
			glog.V(5).Infof("Adding directory %s to fsnotify", path)

			if err := p.watch.Add(path); err != nil {
				glog.Errorf("Failed to add %s to fsnotify: %v", path, err)
				continue
			}

			// Run in a different goroutine to prevent deadlock
			p.wg.Add(1)
			go func() {
				filepath.Walk(path, func(pa string, f os.FileInfo, err error) error {
					if pa == path || f.Mode().IsDir() {
						return nil
					}

					evt := fsnotify.Event{Name: pa, Op: fsnotify.Create}
					p.watch.Events <- evt

					return nil
				})

				p.wg.Done()
			}()

		case <-p.stopChan:
			p.wg.Done()
			return
		}
	}
}

func (p *PluginWatcherImpl) Stop() error {
	close(p.stopChan)
	if err := p.waitTimeout(); err != nil {
		return err
	}

	if err := p.watch.Close(); err != nil {
		return err
	}

	close(p.addedChan)
	close(p.removedChan)

	close(p.addDirChan)

	return nil
}

func (p *PluginWatcherImpl) Added() chan string {
	return p.addedChan
}

func (p *PluginWatcherImpl) Removed() chan string {
	return p.removedChan
}

// Handles plugin added to the plugin directory
func (p *PluginWatcherImpl) handleCreate(path string, event fsnotify.Event) error {
	f, err := os.Stat(path)
	if err != nil {
		return err
	}

	if filepath.Dir(path) == p.pluginDir {
		if !f.Mode().IsDir() {
			return fmt.Errorf("Plugin `%s` in root plugin directory, ignoring", event)
		}

		p.addDirChan <- path
		return nil
	}

	if f.Mode().IsDir() {
		return fmt.Errorf("Directory `%s` in a plugin directory, ignoring", event)
	}

	p.addedChan <- path

	return nil
}

func (p *PluginWatcherImpl) waitTimeout() error {
	c := make(chan interface{})
	go func() {
		defer close(c)
		p.wg.Wait()
	}()

	select {
	case <-c:
		return nil
	case <-time.After(time.Second):
		return fmt.Errorf("Timeout while stopping plugin watcher")
	}

	return nil
}
