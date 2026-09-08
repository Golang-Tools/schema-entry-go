package schemaentry

import (
	"fmt"
	"io"
	"net/url"
	"os"
	"time"
)

// ConfigLoader 配置源加载器:负责从某类 URL scheme 读取原始配置内容与序列化协议。
// 核心内置文件系统加载器;etcd 等其它配置源由 contrib 子模块实现并通过
// RegisterConfigLoader 注册,从而保持核心模块依赖精简。
type ConfigLoader interface {
	// Schemes 返回该加载器支持的 url scheme(如 "", "file", "fs", "dockerfs", "etcd")
	Schemes() []string
	// Load 读取 rawurl 对应的原始配置内容与序列化协议
	Load(rawurl string) ([]byte, SupportedSerialization, error)
}

// Watcher 配置变更监听器:每次从 Events 收到信号,表示内容可能已变化,应重新 Load。
type Watcher interface {
	// Events 返回触发刷新的变更信号通道
	Events() <-chan struct{}
	// Close 停止监听并释放资源
	Close() error
}

// WatchFactory 依据 rawurl 构造 Watcher
type WatchFactory func(rawurl string) (Watcher, error)

var (
	configLoaders  = map[string]ConfigLoader{}
	configWatchers = map[string]WatchFactory{}
)

// RegisterConfigLoader 按 scheme 注册配置源加载器
func RegisterConfigLoader(l ConfigLoader) {
	for _, s := range l.Schemes() {
		configLoaders[s] = l
	}
}

// RegisterWatcher 为指定 scheme 注册监听器工厂
func RegisterWatcher(scheme string, f WatchFactory) {
	configWatchers[scheme] = f
}

// schemeOf 返回地址的 url scheme(纯路径视为 "")
func schemeOf(rawurl string) string {
	U, err := url.Parse(rawurl)
	if err != nil {
		return ""
	}
	return U.Scheme
}

// resolveLoader 依据地址解析出对应 scheme 的加载器
func resolveLoader(rawurl string) ConfigLoader {
	return configLoaders[schemeOf(rawurl)]
}

func init() {
	RegisterConfigLoader(fsLoader{})
	for _, s := range []string{"", "file", "fs", "dockerfs"} {
		RegisterWatcher(s, fsWatcherFactory)
	}
}

// fsLoader 内置文件系统加载器,覆盖 ""/file/fs/dockerfs(本地或容器内路径)
type fsLoader struct{}

func (fsLoader) Schemes() []string { return []string{"", "file", "fs", "dockerfs"} }

func (fsLoader) Load(rawurl string) ([]byte, SupportedSerialization, error) {
	serialize, path, err := fsPathOf(rawurl)
	if err != nil {
		return nil, 0, err
	}
	content, err := readLocalFile(path)
	if err != nil {
		return nil, 0, err
	}
	return content, serialize, nil
}

// fsPathOf 把 fs 系地址解析为本地路径与序列化协议
func fsPathOf(rawurl string) (SupportedSerialization, string, error) {
	U, err := url.Parse(rawurl)
	if err != nil {
		return ParseFSPath(rawurl)
	}
	switch U.Scheme {
	case "", "file", "fs", "dockerfs":
		return ParseFSUrl(U)
	default:
		return 0, "", ErrUnsupportedSchema
	}
}

// readLocalFile 读取本地文件内容(要求文件存在且内容非空)
func readLocalFile(path string) ([]byte, error) {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("find file %s not exist", path)
		}
		return nil, fmt.Errorf("find file %s error: %s", path, err)
	}
	if info.IsDir() {
		return nil, fmt.Errorf("find %s is a dir", path)
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	fd, err := io.ReadAll(f)
	if err != nil {
		return nil, err
	}
	if len(fd) == 0 {
		return nil, fmt.Errorf("find file %s 's content is empty", path)
	}
	return fd, nil
}

// fsWatcherFactory 创建基于轮询的内置文件监听器(纯标准库,零依赖;
// 可用于本地文件与容器内文件,不依赖 inotify/docker)
func fsWatcherFactory(rawurl string) (Watcher, error) {
	_, path, err := fsPathOf(rawurl)
	if err != nil {
		return nil, err
	}
	return newPollingWatcher(path), nil
}

// pollingWatcher 通过周期性检查文件状态(修改时间/大小)实现变更监听
type pollingWatcher struct {
	path     string
	interval time.Duration
	events   chan struct{}
	done     chan struct{}
}

func newPollingWatcher(path string) *pollingWatcher {
	w := &pollingWatcher{
		path:     path,
		interval: 500 * time.Millisecond,
		events:   make(chan struct{}),
		done:     make(chan struct{}),
	}
	go w.run()
	return w
}

// Events 返回触发刷新的信号通道
func (w *pollingWatcher) Events() <-chan struct{} { return w.events }

// Close 停止监听
func (w *pollingWatcher) Close() error {
	select {
	case <-w.done:
	default:
		close(w.done)
	}
	return nil
}

func (w *pollingWatcher) stat() (time.Time, int64, bool) {
	info, err := os.Stat(w.path)
	if err != nil {
		return time.Time{}, 0, false
	}
	return info.ModTime(), info.Size(), true
}

func (w *pollingWatcher) run() {
	defer close(w.events)
	lastMod, lastSize, existed := w.stat()
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()
	for {
		select {
		case <-w.done:
			return
		case <-ticker.C:
			mod, size, ex := w.stat()
			if ex != existed || (ex && (mod != lastMod || size != lastSize)) {
				lastMod, lastSize, existed = mod, size, ex
				select {
				case w.events <- struct{}{}:
				case <-w.done:
					return
				}
			}
		}
	}
}
