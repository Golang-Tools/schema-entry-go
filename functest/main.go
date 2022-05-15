package main

import (
	"fmt"
	"os"
	"time"

	s "github.com/Golang-Tools/schema-entry-go/v2"
	jsoniter "github.com/json-iterator/go"
)

var json = jsoniter.ConfigCompatibleWithStandardLibrary

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
	time.Sleep(time.Duration(1) * time.Minute)
}

func main() {
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
	nodec.SetParent(nodeb).SetParent(root).Parse([]string{"foo", "bar", "par", "-c", "watch.json"})
}
