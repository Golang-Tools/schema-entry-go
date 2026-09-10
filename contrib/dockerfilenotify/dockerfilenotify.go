// Package dockerfilenotify 为 schema-entry-go 提供基于 github.com/docker/docker/pkg/filenotify
// 的文件监听器(可选的 contrib 子模块),复现旧版核心使用的行为:
// 本地文件(""/file/fs)用事件监听,dockerfs 容器内文件用轮询监听。
// 空导入即可覆盖核心对这些 scheme 的 watcher 注册:
//
//	import _ "github.com/Golang-Tools/schema-entry-go/contrib/dockerfilenotify"
package dockerfilenotify

import (
	"sync"

	schemaentry "github.com/Golang-Tools/schema-entry-go/v4"
	"github.com/docker/docker/pkg/filenotify"
)

func init() {
	for _, s := range []string{"", "file", "fs", "dockerfs"} {
		schemaentry.RegisterWatcher(s, NewWatcher)
	}
}

// NewWatcher 创建基于 docker pkg/filenotify 的文件监听器
func NewWatcher(rawurl string) (schemaentry.Watcher, error) {
	_, path, err := schemaentry.ResolveFSLocation(rawurl)
	if err != nil {
		return nil, err
	}
	var fw filenotify.FileWatcher
	if schemaentry.SchemeOf(rawurl) == "dockerfs" {
		fw = filenotify.NewPollingWatcher()
	} else {
		fw, err = filenotify.NewEventWatcher()
		if err != nil {
			return nil, err
		}
	}
	if err := fw.Add(path); err != nil {
		fw.Close()
		return nil, err
	}
	w := &Watcher{
		fw:     fw,
		events: make(chan struct{}),
		done:   make(chan struct{}),
	}
	go w.run()
	return w, nil
}

// Watcher 基于 docker pkg/filenotify 的监听器
type Watcher struct {
	fw     filenotify.FileWatcher
	events chan struct{}
	done   chan struct{}
	once   sync.Once
}

// Events 返回触发刷新的信号通道
func (w *Watcher) Events() <-chan struct{} { return w.events }

// Close 停止监听
func (w *Watcher) Close() error {
	w.once.Do(func() {
		close(w.done)
		_ = w.fw.Close()
	})
	return nil
}

func (w *Watcher) run() {
	defer close(w.events)
	for {
		select {
		case <-w.done:
			return
		case _, ok := <-w.fw.Events():
			if !ok {
				return
			}
			select {
			case w.events <- struct{}{}:
			case <-w.done:
				return
			}
		case _, ok := <-w.fw.Errors():
			if !ok {
				return
			}
		}
	}
}
