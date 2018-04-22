package pluginregistration

import (
	"fmt"
	"flag"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/golang/glog"
	"github.com/stretchr/testify/require"
)

var (
	testPluginDir = ""
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

// TestHandleEvent
// We can do this because the prober was not started
func TestHandleCreate(t *testing.T) {
	defer func() { require.NoError(t, cleanupPlugins(testPluginDir)) }()
	var wg sync.WaitGroup

	// some setup
	p, err := NewPluginWatcherImpl(testPluginDir)
	require.NoError(t, err)

	require.NoError(t, touchPath(filepath.Join(testPluginDir, "nvidia.com/gpu.sock")))

	noError := []struct {
		path    string
		waitDir chan string
	}{
		{"nvidia.com", p.addDirChan},
		{"nvidia.com/gpu.sock", p.addedChan},
	}

	for _, test := range noError {
		glog.V(2).Infof("Testing %s", test.path)
		wg.Add(1)

		path := filepath.Join(testPluginDir, test.path)
		event := fsnotify.Event{Name: path, Op: fsnotify.Create}

		go waitForPath(t, &wg, path, test.waitDir)
		require.NoError(t, p.handleCreate(path, event))

		wg.Wait()
	}

	// The last test is not tested because the previous case should
	// prevent subdirectories from
	// being added to the watch
	errors := []string{
		"gpu.sock",
		"nvidia.com/foo/",
		//		"nvidia.com/foo/gpu.sock",
	}

	for _, rpath := range errors {
		path := filepath.Join(testPluginDir, rpath)
		event := fsnotify.Event{Name: path, Op: fsnotify.Create}
		if strings.HasSuffix(rpath, "/") {
			path += "/"
		}

		require.NoError(t, touchPath(path))
		require.Error(t, p.handleCreate(path, event))
	}

	require.NoError(t, p.Stop())
}

func TestAddAndDeletePlugins(t *testing.T) {
	defer func() { require.NoError(t, cleanupPlugins(testPluginDir)) }()

	p, err := NewPluginWatcherImpl(testPluginDir)
	require.NoError(t, err)
	p.Start()

	addedPlugins := []string{
		"nvidia.com/gpu.sock",
		"nvidia.com/infiniband",
		"foo.bar/baz.sock",
	}

	for _, rpath := range addedPlugins {
		glog.V(2).Infof("Testing adding plugin %s", rpath)

		path := filepath.Join(testPluginDir, rpath)
		require.NoError(t, touchPath(path))
		waitForPath(t, nil, path, p.Added())
	}

	removedEvents := []struct {
		path    string
		plugins []string
	}{
		{"nvidia.com", []string{"nvidia.com/gpu.sock", "nvidia.com/infiniband", "nvidia.com", "nvidia.com"}},
		{"foo.bar", []string{"foo.bar/baz.sock", "foo.bar", "foo.bar"}},
	}

	for _, r := range removedEvents {
		glog.V(2).Infof("Testing removing plugin %s", r.path)

		path := filepath.Join(testPluginDir, r.path)
		require.NoError(t, os.RemoveAll(path))

		for i := range r.plugins {
			r.plugins[i] = filepath.Join(testPluginDir, r.plugins[i])
		}

		waitForPaths(t, r.plugins, p.Removed())
	}

	require.NoError(t, p.Stop())
}

// TestRestartProber

func waitForPath(t *testing.T, wg *sync.WaitGroup, expectedPath string, c chan string) {
	defer func() {
		if wg != nil {
			wg.Done()
		}
	}()

	select {
	case p := <-c:
		require.Equal(t, expectedPath, p)
		break
	case <-time.After(time.Second):
		t.Fatal("Could not fetch path from probe")
	}
}

func waitForPaths(t *testing.T, expectedPaths []string, c chan string) {
	var actualPaths []string
	for i := 0; i < len(expectedPaths); i++ {
		select {
		case p := <-c:
			actualPaths = append(actualPaths, p)
			break
		case <-time.After(time.Second):
			t.Fatal("Could not fetch path from probe")
		}
	}

	sort.Strings(expectedPaths)
	sort.Strings(actualPaths)

	glog.V(2).Infof("expected: %v, actual: %v", expectedPaths, actualPaths)

	require.Equal(t, len(expectedPaths), len(actualPaths))
	for i := 0; i < len(expectedPaths); i++ {
		require.Equal(t, expectedPaths[i], actualPaths[i])
	}
}

func touchPath(path string) error {
	// Just create the directory
	if strings.HasSuffix(path, "/") {
		if err := os.MkdirAll(path, 0755); err != nil {
			return err
		}

		return nil
	}

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}

	f, err := os.OpenFile(path, os.O_RDONLY|os.O_CREATE, 0666)
	if err != nil {
		return err
	}

	f.Close()
	return nil
}
