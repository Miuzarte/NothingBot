package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/fsnotify/fsnotify"
	log "github.com/sirupsen/logrus"
)

func WatchFile(filePath string, callback func(fsnotify.Event)) {
	initWG := sync.WaitGroup{}
	initWG.Add(1)
	go func() {
		watcher, err := fsnotify.NewWatcher()
		if err != nil {
			fmt.Printf("failed to create watcher: %s", err)
			os.Exit(1)
		}
		defer watcher.Close()

		dir, file := filepath.Split(filePath) // 监听路径, 文件名

		eventsWG := sync.WaitGroup{}
		eventsWG.Add(1)
		go func() {
			for {
				select {
				case event, ok := <-watcher.Events:
					log.Debug("case event, ok := <-watcher.Events: ", event, " - ", ok)
					if !ok { // 'Events' channel is closed
						eventsWG.Done()
						return
					}
					if event.Name == file {
						callback(event)
					}

				case err, ok := <-watcher.Errors:
					log.Debug("case err, ok := <-watcher.Errors: ", err, " - ", ok)
					if ok { // 'Errors' channel is not closed
						fmt.Printf("watcher error: %s", err)
					}
					eventsWG.Done()
					return
				}
			}
		}()
		watcher.Add(dir)
		initWG.Done()   // done initializing the watch in this go routine, so the parent routine can move on...
		eventsWG.Wait() // now, wait for event loop to end in this go-routine...
	}()
	initWG.Wait() // make sure that the go routine above fully ended before returning
}

func onConfigChange(in fsnotify.Event) {
	log.Info("文件: ", in.Name, "  发生了: ", in.Op)
}
