package fsnotify

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// fsnotify watcher:本地文件写入后应发出事件
func TestWatcherDetectsChange(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "c.json")
	if err := os.WriteFile(p, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	w, err := NewWatcher(p)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()

	// 等待 fsnotify 完成目录/文件监听注册
	time.Sleep(300 * time.Millisecond)

	if err := os.WriteFile(p, []byte(`{"a":1}`), 0o644); err != nil {
		t.Fatal(err)
	}
	select {
	case <-w.Events():
	case <-time.After(4 * time.Second):
		t.Fatal("期望收到文件变更事件")
	}
}
