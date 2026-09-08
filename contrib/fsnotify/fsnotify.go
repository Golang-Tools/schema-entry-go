// Package fsnotify 为 schema-entry-go 提供基于 fsnotify 的本地文件监听器(可选的 contrib 子模块),
// 相比核心内置的轮询监听更高效。空导入即可把 ""/file/fs 的 watcher 替换为 fsnotify 实现:
//
//	import _ "github.com/Golang-Tools/schema-entry-go/v4/contrib/fsnotify"
package fsnotify

import (
	"path/filepath"
	"sync"

	schemaentry "github.com/Golang-Tools/schema-entry-go/v4"
	fsn "github.com/fsnotify/fsnotify"
)

func init() {
	// 覆盖核心对本地文件系的 watcher 注册(""/file/fs);dockerfs 容器文件仍由核心轮询处理
	for _, s := range []string{"", "file", "fs"} {
		schemaentry.RegisterWatcher(s, NewWatcher)
	}
}

// NewWatcher 创建基于 fsnotify 的本地文件监听器
func NewWatcher(rawurl string) (schemaentry.Watcher, error) {
	_, path, err := schemaentry.ResolveFSLocation(rawurl)
	if err != nil {
		return nil, err
	}
	fw, err := fsn.NewWatcher()
	if err != nil {
		return nil, err
	}
	// 监听所在目录以兼容编辑器“写入临时文件后改名”的保存方式
	dir := filepath.Dir(path)
	if err := fw.Add(dir); err != nil {
		fw.Close()
		return nil, err
	}
	// 若文件存在也直接监听(可减少目录级噪音)
	_ = fw.Add(path)

	w := &Watcher{
		fw:     fw,
		dir:    dir,
		file:   filepath.Base(path),
		events: make(chan struct{}),
		done:   make(chan struct{}),
	}
	go w.run()
	return w, nil
}

// Watcher 基于 fsnotify 的监听器
type Watcher struct {
	fw     *fsn.Watcher
	dir    string
	file   string
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
		case err, ok := <-w.fw.Errors:
			if !ok {
				return
			}
			_ = err
		case ev, ok := <-w.fw.Events:
			if !ok {
				return
			}
			if ev.Op&(fsn.Write|fsn.Create|fsn.Remove|fsn.Rename) != 0 &&
				filepath.Base(ev.Name) == w.file {
				select {
				case w.events <- struct{}{}:
				case <-w.done:
					return
				}
			}
		}
	}
}
