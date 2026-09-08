package schemaentry

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// 核心内置轮询 watcher:文件内容变化后应发出事件
func TestPollingWatcherDetectsChange(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "c.json")
	if err := os.WriteFile(p, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	w := newPollingWatcher(p)
	defer w.Close()

	// 初始(未变化)不应有事件
	select {
	case <-w.Events():
		t.Fatal("初始不应产生事件")
	case <-time.After(100 * time.Millisecond):
	}

	// 修改文件应触发事件(轮询间隔 500ms)
	if err := os.WriteFile(p, []byte(`{"a":1}`), 0o644); err != nil {
		t.Fatal(err)
	}
	select {
	case <-w.Events():
	case <-time.After(4 * time.Second):
		t.Fatal("期望收到文件变更事件")
	}
}

// 纯路径应解析到内置 fs 加载器并能正确加载内容
func TestFSLoaderLoadPlainPath(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "c.json")
	if err := os.WriteFile(p, []byte(`{"a":1}`), 0o644); err != nil {
		t.Fatal(err)
	}
	loader := resolveLoader(p)
	if loader == nil {
		t.Fatal("纯路径应解析到 fs 加载器")
	}
	content, ser, err := loader.Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if ser != SerializationJSON {
		t.Fatalf("期望 JSON 序列化,实际 %v", ser)
	}
	if string(content) != `{"a":1}` {
		t.Fatalf("内容不符:%s", content)
	}
}

// 未注册 scheme 的 -c 加载应返回 ErrUnsupportedSchema(提示需要对应 contrib)
func TestLoadConfigFileByPathUnsupportedScheme(t *testing.T) {
	ep := newCLIEndpoint(t, WithName("par"), WithNotParseEnv(), WithNotVerifySchema())
	err := ep.loadConfigFileByPath("nosuchscheme://host/key?serialize=JSON")
	if !errors.Is(err, ErrUnsupportedSchema) {
		t.Fatalf("期望 ErrUnsupportedSchema,实际 %v", err)
	}
}
