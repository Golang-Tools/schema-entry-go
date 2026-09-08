// 命令 watch 演示 schema-entry-go 的多级子命令与配置监听(watchmode)能力。
//
// 默认监听本地文件 watch.json(位于仓库根目录,可离线演示):
//
//	go run ./example/watch
//
// 运行后修改 watch.json,将触发 OnRefresh 回调刷新配置。
//
// 如需监听 etcd 等其它配置源,需在应用侧空导入对应 contrib 包并传入其 url,
// 例如 (etcd):
//
//	go run ./example/watch "etcd://localhost:12379/foo/bar?serialize=JSON"
//
// 并在外部应用(而非本示例所在模块)中 import _ "github.com/Golang-Tools/schema-entry-go/v4/contrib/etcd"。
package main

import (
	"errors"
	"fmt"
	"os"
	"time"

	log "github.com/Golang-Tools/loggerhelper/v4"
	s "github.com/Golang-Tools/schema-entry-go/v4"
)

// C 演示一个带 jsonschema 约束的配置结构体
type C struct {
	A            int   `yaml:"aa" jsonschema:"required,title=a,description=测试int,maximum=10,default=10"`
	B            int   `yaml:"b" jsonschema:"required,title=b,description=测试int,maximum=10,default=100"`
	OK           bool  `json:"ok" jsonschema:"title=o,description=测试bool"`
	Field        []int `json:"field" jsonschema:"required,title=f,description=测试列表"`
	FieldDefault []int `json:"field_default" jsonschema:"required,title=d,description=测试列表默认值,default=1,default=2,default=3,default=4,default=5"`
	WatchValue   int   `json:"WatchValue" jsonschema:"required,title=w,description=测试监听"`
	s            int
}

func (c *C) Test() {
	fmt.Println(c)
}

func (c *C) Main() {
	c.Test()
	time.Sleep(time.Minute)
}

func main() {
	log.Set(log.WithLevel("Warn"))
	// 默认监听仓库根目录的 watch.json(本地文件,可离线演示);
	// 也可传入一个 etcd url 监听远端配置
	target := "watch.json"
	if len(os.Args) > 1 {
		target = os.Args[1]
	}
	root, _ := s.NewEntryPoint(s.WithName("foo"), s.WithDescription("测试用foo"), s.WithUsage("foo cmd test"))
	nodeb, _ := s.NewEntryPoint(s.WithName("bar"), s.WithDescription("测试用foo bar"), s.WithUsage("foo bar cmd test"))
	nodec, _ := s.NewEndPoint(new(C), s.WithName("par"), s.WithNotVerifySchema(),
		s.WithDefaultConfigFilePaths("conf.json", "config.json", "testconf.yml"),
		s.WithDescription("测试用foo bar par"),
		s.WithUsage("foo bar par cmd test"),
		s.WithLoadAllConfigFile(),
		s.WithWatchMode(),
	)
	nodec.OnRefresh(func(c *C) {
		c.Test()
	})
	os.Setenv("FOO_BAR_PAR_A", "123")
	err := nodec.SetParent(nodeb).SetParent(root).Parse([]string{"foo", "bar", "par", "-c", target})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		if !errors.Is(err, s.ErrHelp) {
			os.Exit(1)
		}
	}
}
