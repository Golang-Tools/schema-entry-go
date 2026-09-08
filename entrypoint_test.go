package schemaentry

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNewEntryPointRequiresName(t *testing.T) {
	if _, err := NewEntryPoint(); err == nil {
		t.Fatal("期望未设置 Name 时报错")
	}
}

func TestNewEntryPointWithOptions(t *testing.T) {
	ep, err := NewEntryPoint(
		WithName("foo"),
		WithDescription("desc"),
		WithUsage("usage"),
		WithEnvPrefix("PREFIX"),
		WithLoadAllConfigFile(),
		WithNotParseEnv(),
		WithNotVerifySchema(),
		WithDefaultConfigFilePaths("a.json", "b.json"),
	)
	if err != nil {
		t.Fatal(err)
	}
	m := ep.Meta()
	if m.Name != "foo" || m.Description != "desc" || m.Usage != "usage" {
		t.Fatalf("meta 字段设置错误:%+v", m)
	}
	if m.EnvPrefix != "PREFIX" || !m.LoadAllConfigFile || !m.NotParseEnv || !m.NotVerifySchema {
		t.Fatalf("meta 布尔/前缀设置错误:%+v", m)
	}
	if len(m.DefaultConfigFilePaths) != 2 {
		t.Fatalf("DefaultConfigFilePaths 设置错误:%v", m.DefaultConfigFilePaths)
	}
	if ep.IsRoot() != true || ep.IsEndpoint() != false {
		t.Fatalf("IsRoot/IsEndpoint 错误")
	}
}

func TestTreeStructure(t *testing.T) {
	root, _ := NewEntryPoint(WithName("foo"), WithDescription("root desc"), WithUsage("foo cmd"))
	bar, _ := NewEntryPoint(WithName("bar"), WithDescription("bar desc"), WithUsage("foo bar cmd"))
	par := newCLIEndpoint(t, WithName("par"), WithNotParseEnv())

	RegistSubNode(root, bar)
	RegistSubNode(bar, par)

	if !root.IsRoot() || root.IsEndpoint() {
		t.Fatal("root 应为根且非叶子")
	}
	if bar.IsRoot() || bar.IsEndpoint() {
		t.Fatal("bar 应为非根且非叶子")
	}
	if par.IsRoot() || !par.IsEndpoint() {
		t.Fatal("par 应为非根且为叶子")
	}
	if root.Meta().Subcmds()["bar"] == nil {
		t.Fatal("root 应包含子节点 bar")
	}
	if bar.Meta().Subcmds()["par"] == nil {
		t.Fatal("bar 应包含子节点 par")
	}
	if par.Meta().Parent() != bar {
		t.Fatal("par 的父节点应为 bar")
	}

	if GetNodeProg(root) != "foo" || GetNodeProg(bar) != "foo bar" || GetNodeProg(par) != "foo bar par" {
		t.Fatalf("GetNodeProg 错误:%q %q %q", GetNodeProg(root), GetNodeProg(bar), GetNodeProg(par))
	}
	if GetNodeEnvPrefix(par) != "FOO_BAR_PAR" {
		t.Fatalf("GetNodeEnvPrefix 错误:%q", GetNodeEnvPrefix(par))
	}
}

func TestEndpointCannotSetChild(t *testing.T) {
	par := newCLIEndpoint(t, WithName("par"), WithNotParseEnv())
	other, _ := NewEntryPoint(WithName("other"))
	if err := par.SetChild(other); err != ErrNotAllowSetChildToEndPoint {
		t.Fatalf("期望 ErrNotAllowSetChildToEndPoint,实际 %v", err)
	}
}

func TestLoadJSONConfigFileFromFS(t *testing.T) {
	ep := newCLIEndpoint(t, WithName("par"), WithNotParseEnv())
	dir := t.TempDir()
	p := filepath.Join(dir, "c.json")
	if err := os.WriteFile(p, []byte(`{"a":7,"ok":false,"s":"bar","flag":true,"list":["x"],"ints":[1,2]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	stop, err := ep.loadConfigFileFromFS(SerializationJSON, p)
	if err != nil {
		t.Fatal(err)
	}
	if !stop {
		t.Fatal("期望成功解析并返回 stop=true")
	}
	if ep.config.A != 7 || ep.config.OK || ep.config.S != "bar" || !ep.config.Flag {
		t.Fatalf("JSON 配置解析错误:%+v", ep.config)
	}
	if len(ep.config.List) != 1 || ep.config.List[0] != "x" {
		t.Fatalf("JSON list 解析错误:%v", ep.config.List)
	}
}

type yamlCfg struct {
	A  int    `yaml:"aa" json:"aa"`
	OK bool   `yaml:"ok" json:"ok"`
	S  string `yaml:"s" json:"s"`
}

func (c *yamlCfg) Main() {}

func TestLoadYAMLConfigFileFromFS(t *testing.T) {
	ep, err := NewEndPoint(new(yamlCfg), WithName("par"), WithNotParseEnv())
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	p := filepath.Join(dir, "c.yaml")
	if err := os.WriteFile(p, []byte("aa: 7\nok: false\ns: bar\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	stop, err := ep.loadConfigFileFromFS(SerializationYAML, p)
	if err != nil {
		t.Fatal(err)
	}
	if !stop {
		t.Fatal("期望成功解析并返回 stop=true")
	}
	c := ep.config
	if c.A != 7 || c.OK || c.S != "bar" {
		t.Fatalf("YAML 配置解析错误:%+v", c)
	}
}

func TestVerifyConfigNotVerifySchema(t *testing.T) {
	ep := newCLIEndpoint(t, WithName("par"), WithNotVerifySchema())
	if err := ep.verifyConfig(); err != nil {
		t.Fatalf("期望 NotVerifySchema 时 verifyConfig 返回 nil,实际 %v", err)
	}
}
