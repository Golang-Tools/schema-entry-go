package schemaentry

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	log "github.com/Golang-Tools/loggerhelper/v3"
	"github.com/Golang-Tools/optparams"
	"github.com/spf13/pflag"
)

func TestMain(m *testing.M) {
	// 静默测试期间的日志输出
	log.Set(log.WithLevel("Error"))
	os.Exit(m.Run())
}

type cliCfg struct {
	A      int       `json:"a" jsonschema:"title=a,default=1"`
	OK     bool      `json:"ok" jsonschema:"title=o,default=true"`
	Flag   bool      `json:"flag" jsonschema:"title=f"`
	S      string    `json:"s" jsonschema:"default=foo"`
	F      float64   `json:"f"`
	List   []string  `json:"list"`
	Ints   []int     `json:"ints"`
	Floats []float64 `json:"floats"`
}

func (c *cliCfg) Main() {}

func newCLIEndpoint(t *testing.T, opts ...optparams.Option[EntryPointMeta]) *EndPoint[*cliCfg] {
	t.Helper()
	ep, err := NewEndPoint(new(cliCfg), opts...)
	if err != nil {
		t.Fatalf("NewEndPoint err: %v", err)
	}
	return ep
}

func parseCLIFlags(t *testing.T, ep *EndPoint[*cliCfg], args ...string) *pflag.FlagSet {
	t.Helper()
	fs, err := ep.buildConfigFlagSet()
	if err != nil {
		t.Fatalf("buildConfigFlagSet err: %v", err)
	}
	if err := fs.Parse(args); err != nil {
		t.Fatalf("fs.Parse(%v) err: %v", args, err)
	}
	return fs
}

func apply(t *testing.T, ep *EndPoint[*cliCfg], fs *pflag.FlagSet) {
	t.Helper()
	if err := ep.applyConfigDefaults(); err != nil {
		t.Fatalf("applyConfigDefaults err: %v", err)
	}
	if err := ep.applyFlagsToConfig(fs); err != nil {
		t.Fatalf("applyFlagsToConfig err: %v", err)
	}
}

// 回归:jsonschema default=1 时,命令行显式传入 0 不应被默认值覆盖
func TestCLIIntZeroOverridesDefault(t *testing.T) {
	ep := newCLIEndpoint(t, WithName("par"), WithNotParseEnv())
	fs := parseCLIFlags(t, ep, "--A=0")
	apply(t, ep, fs)
	if ep.config.A != 0 {
		t.Fatalf("期望 A=0(显式 0 应覆盖默认 1),实际 %d", ep.config.A)
	}
}

// 未传 flag 时使用 jsonschema 默认值
func TestCLIDefaultUsedWhenFlagAbsent(t *testing.T) {
	ep := newCLIEndpoint(t, WithName("par"), WithNotParseEnv())
	fs := parseCLIFlags(t, ep)
	apply(t, ep, fs)
	if ep.config.A != 1 {
		t.Fatalf("期望 A=1(默认值),实际 %d", ep.config.A)
	}
	if !ep.config.OK {
		t.Fatalf("期望 OK=true(默认值),实际 %v", ep.config.OK)
	}
	if ep.config.S != "foo" {
		t.Fatalf("期望 S=foo(默认值),实际 %q", ep.config.S)
	}
}

// 回归:jsonschema default=true 时,命令行显式传入 false 不应被默认值覆盖
func TestCLIBoolFalseOverridesDefault(t *testing.T) {
	ep := newCLIEndpoint(t, WithName("par"), WithNotParseEnv())
	fs := parseCLIFlags(t, ep, "--OK=false")
	apply(t, ep, fs)
	if ep.config.OK {
		t.Fatalf("期望 OK=false(显式 false 应覆盖默认 true),实际 true")
	}
}

// bool flag 以裸 flag 形式(--Flag)传入时置为 true
func TestCLIBoolTrueBareFlag(t *testing.T) {
	ep := newCLIEndpoint(t, WithName("par"), WithNotParseEnv())
	fs := parseCLIFlags(t, ep, "--Flag")
	apply(t, ep, fs)
	if !ep.config.Flag {
		t.Fatalf("期望 Flag=true(裸 --Flag),实际 false")
	}
}

// 回归:string 默认值时,命令行显式传空串不应被默认值覆盖
func TestCLIEmptyStringOverridesDefault(t *testing.T) {
	ep := newCLIEndpoint(t, WithName("par"), WithNotParseEnv())
	fs := parseCLIFlags(t, ep, "--S=")
	apply(t, ep, fs)
	if ep.config.S != "" {
		t.Fatalf("期望 S=\"\"(显式空串应覆盖默认 foo),实际 %q", ep.config.S)
	}
}

// float 显式传 0
func TestCLIFloatZero(t *testing.T) {
	ep := newCLIEndpoint(t, WithName("par"), WithNotParseEnv())
	fs := parseCLIFlags(t, ep, "--F=0")
	apply(t, ep, fs)
	if ep.config.F != 0.0 {
		t.Fatalf("期望 F=0,实际 %v", ep.config.F)
	}
}

// slice 类型:重复传入与 int/float64 列表
func TestCLISlices(t *testing.T) {
	ep := newCLIEndpoint(t, WithName("par"), WithNotParseEnv())
	fs := parseCLIFlags(t, ep, "--List=x", "--List=y", "--Ints=1", "--Ints=2", "--Floats=1.5", "--Floats=2.5")
	apply(t, ep, fs)
	if !reflect.DeepEqual(ep.config.List, []string{"x", "y"}) {
		t.Fatalf("期望 List=[x y],实际 %v", ep.config.List)
	}
	if !reflect.DeepEqual(ep.config.Ints, []int{1, 2}) {
		t.Fatalf("期望 Ints=[1 2],实际 %v", ep.config.Ints)
	}
	if !reflect.DeepEqual(ep.config.Floats, []float64{1.5, 2.5}) {
		t.Fatalf("期望 Floats=[1.5 2.5],实际 %v", ep.config.Floats)
	}
}

// 优先级:环境变量 > 默认值;命令行(显式) > 环境变量
func TestCLIEnvAndCommandLinePrecedence(t *testing.T) {
	t.Setenv("PAR_A", "5")
	ep := newCLIEndpoint(t, WithName("par"))
	fs := parseCLIFlags(t, ep)
	apply(t, ep, fs)
	if ep.config.A != 5 {
		t.Fatalf("期望 A=5(环境变量覆盖默认 1),实际 %d", ep.config.A)
	}
	fs2 := parseCLIFlags(t, ep, "--A=0")
	apply(t, ep, fs2)
	if ep.config.A != 0 {
		t.Fatalf("期望 A=0(命令行覆盖环境变量 5),实际 %d", ep.config.A)
	}
}

// NewEndPoint 未命名时自动使用 config 结构体类型名(小写)
func TestNewEndPointAutoName(t *testing.T) {
	ep := newCLIEndpoint(t, WithNotParseEnv())
	if ep.Meta().Name != "clicfg" {
		t.Fatalf("期望自动命名 clicfg,实际 %q", ep.Meta().Name)
	}
}

// 端到端:passArgs 完整链路(默认值->环境->-c 配置文件->命令行)
func TestPassArgsWithConfigFile(t *testing.T) {
	ep := newCLIEndpoint(t, WithName("par"), WithNotParseEnv(), WithNotVerifySchema())
	dir := t.TempDir()
	p := filepath.Join(dir, "c.json")
	if err := os.WriteFile(p, []byte(`{"a":3,"ok":false,"s":"cfg"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	ok := ep.passArgs("par", []string{"par", "-c", p, "--A=0"})
	if !ok {
		t.Fatal("passArgs 应返回 true")
	}
	if ep.config.A != 0 {
		t.Fatalf("期望 A=0(命令行覆盖配置文件的 3),实际 %d", ep.config.A)
	}
	if ep.config.OK {
		t.Fatalf("期望 OK=false(配置文件覆盖默认 true),实际 true")
	}
	if ep.config.S != "cfg" {
		t.Fatalf("期望 S=cfg(配置文件),实际 %q", ep.config.S)
	}
}
