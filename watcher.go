package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/fsnotify/fsnotify"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"
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

func WatchFileChanges(filePath string, callback func(in fsnotify.Event)) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Fatal(err)
	}
	defer watcher.Close()

	err = watcher.Add(filePath)
	if err != nil {
		log.Fatal(err)
	}

	go func() {
		for {
			select {
			case event, ok := <-watcher.Events:
				if !ok {
					log.Error("[watcher] !ok1: ", !ok)
					return
				}
				if event.Op&fsnotify.Write == fsnotify.Write || event.Op&fsnotify.Create == fsnotify.Create {
					log.Debug("[watcher] File: ", event.Name, "  Op: ", event.Op)
					callback(event)
				}
			case err, ok := <-watcher.Errors:
				if !ok || err != nil {
					log.Error("[watcher] !ok2: ", !ok)
					log.Error("[watcher] err: ", err)
					return
				}
			}
		}
	}()
}

// WatchConfig starts watching a config file for changes.
func WatchConfig(v *viper.Viper) {
	initWG := sync.WaitGroup{}
	initWG.Add(1)
	go func() {
		// 创建文件监视器
		watcher, err := fsnotify.NewWatcher()
		if err != nil {
			log.Error(fmt.Sprintf("failed to create watcher: %s", err))
			os.Exit(1)
		}
		defer watcher.Close() // 在函数结束时关闭监视器

		// 获取配置文件的路径
		filename, err := "filename", nil
		if err != nil {
			log.Error(fmt.Sprintf("get config file: %s", err))
			initWG.Done()
			return
		}

		configFile := filepath.Clean(filename)               // 清理路径中的冗余部分
		configDir, _ := filepath.Split(configFile)           // 获取文件所在的目录
		realConfigFile, _ := filepath.EvalSymlinks(filename) // 获取配置文件的实际路径

		eventsWG := sync.WaitGroup{}
		eventsWG.Add(1)
		go func() {
			for {
				select {
				case event, ok := <-watcher.Events:
					if !ok { // 'Events' 通道已关闭
						eventsWG.Done()
						return
					}
					currentConfigFile, _ := filepath.EvalSymlinks(filename) // 获取当前配置文件的实际路径
					// 对于以下情况，我们只关心配置文件的变化：
					// 1 - 如果配置文件被修改或创建
					// 2 - 如果配置文件的真实路径改变（例如：k8s ConfigMap 替换）
					if (filepath.Clean(event.Name) == configFile &&
						(event.Has(fsnotify.Write) || event.Has(fsnotify.Create))) ||
						(currentConfigFile != "" && currentConfigFile != realConfigFile) {
						realConfigFile = currentConfigFile // 更新真实配置文件路径
						// err := v.ReadInConfig()            // 重新加载配置文件
						if err != nil {
							log.Error(fmt.Sprintf("read config file: %s", err))
						}
						onConfigChange(event) // 调用配置更改回调函数
					} else if filepath.Clean(event.Name) == configFile && event.Has(fsnotify.Remove) {
						eventsWG.Done()
						return
					}

				case err, ok := <-watcher.Errors:
					if ok { // 'Errors' 通道未关闭
						log.Error(fmt.Sprintf("watcher error: %s", err))
					}
					eventsWG.Done()
					return
				}
			}
		}()
		watcher.Add(configDir) // 监视配置文件所在目录
		initWG.Done()          // 在此 goroutine 中完成监视的初始化，以便父例程可以继续...
		eventsWG.Wait()        // 等待事件循环在此 goroutine 中结束...
	}()
	initWG.Wait() // 确保上面的 goroutine 完全结束后再返回
}

func onConfigChange(in fsnotify.Event) {
	log.Info("文件: ", in.Name, "  发生了: ", in.Op)
}
