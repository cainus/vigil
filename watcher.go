package main

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/fsnotify/fsnotify"
)

// Watcher wraps fsnotify to watch for file changes
type Watcher struct {
	watcher *fsnotify.Watcher
	Events  chan fsnotify.Event
	Errors  chan error
}

// NewWatcher creates a new file watcher for the given directory
func NewWatcher(dir string) (*Watcher, error) {
	fsWatcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	w := &Watcher{
		watcher: fsWatcher,
		Events:  make(chan fsnotify.Event),
		Errors:  make(chan error),
	}

	// Add the root directory and all subdirectories
	err = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip errors
		}
		if info.IsDir() {
			// Skip .git directory
			if strings.Contains(path, ".git") {
				return filepath.SkipDir
			}
			return fsWatcher.Add(path)
		}
		return nil
	})
	if err != nil {
		fsWatcher.Close()
		return nil, err
	}

	// Start the event forwarding goroutine
	go w.run()

	return w, nil
}

// run forwards events from fsnotify, filtering out .git changes
func (w *Watcher) run() {
	for {
		select {
		case event, ok := <-w.watcher.Events:
			if !ok {
				return
			}
			// Filter out .git directory changes
			if strings.Contains(event.Name, ".git") {
				continue
			}
			w.Events <- event

			// If a directory was created, add it to the watcher
			if event.Op&fsnotify.Create == fsnotify.Create {
				info, err := os.Stat(event.Name)
				if err == nil && info.IsDir() {
					w.watcher.Add(event.Name)
				}
			}
		case err, ok := <-w.watcher.Errors:
			if !ok {
				return
			}
			w.Errors <- err
		}
	}
}

// Close stops the watcher
func (w *Watcher) Close() error {
	return w.watcher.Close()
}
