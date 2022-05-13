package main

import (
	"fmt"
	"os"

	s "github.com/Golang-Tools/schema-entry-go/v2"
	jsoniter "github.com/json-iterator/go"
)

var json = jsoniter.ConfigCompatibleWithStandardLibrary

type C struct {
	A     int   `yaml:"aa" jsonschema:"required,title=a,description=测试int,maximum=10,default=10"`
	B     int   `yaml:"b" jsonschema:"required,title=a,description=测试int,maximum=10,default=100"`
	OK    bool  `json:"ok" jsonschema:"title=o,description=测试bool"`
	Field []int `json:"field" jsonschema:"required,title=f,description=测试列表"`
	s     int
}

func (c C) Main() {
	fmt.Println(c)
}

// func (c *C) BeforeRefresh()       {} //配置刷新前执行的回调
// func (c *C) OnRefresh()           {} //配置刷新后执行的回调
// func (c *C) OnRefreshError(error) {} //配置刷新失败后执行的回调

func main() {
	root, _ := s.NewEntryPoint(s.WithName("foo"), s.WithDescription("测试用foo"), s.WithUsage("foo cmd test"))
	nodeb, _ := s.NewEntryPoint(s.WithName("bar"), s.WithDescription("测试用foo bar"), s.WithUsage("foo bar cmd test"))
	nodec, _ := s.NewEndPoint[C](s.WithName("par"), s.WithNotVerifySchema(),
		s.WithDefaultConfigFilePaths("conf.json", "config.json", "testconf.yml"),
		s.WithDescription("测试用foo bar par"),
		s.WithUsage("foo bar par cmd test"),
		s.WithLoadAllConfigFile(),
	)
	os.Setenv("FOO_BAR_PAR_A", "123")
	nodec.SetParent(nodeb).SetParent(root).Parse([]string{"foo", "bar", "par"})
}
