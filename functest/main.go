package main

import (
	"fmt"
	"os"

	s "github.com/Golang-Tools/schema-entry-go"
	jsoniter "github.com/json-iterator/go"
)

var json = jsoniter.ConfigCompatibleWithStandardLibrary

type C struct {
	A     int   `json:"a" jsonschema:"required,title=a,description=测试int,maximum=10"`
	OK    bool  `json:"ok" jsonschema:"title=o,description=测试bool"`
	Field []int `json:"field" jsonschema:"required,title=f,description=测试列表"`
	s     int
}

func (c *C) Main() {
	fmt.Println(c)
}

func main() {
	root, _ := s.New(&s.EntryPointMeta{Name: "foo", Description: "测试用foo", Usage: "foo cmd test"})
	nodeb, _ := s.New(&s.EntryPointMeta{Name: "bar", Description: "测试用foo bar", Usage: "foo bar cmd test"})
	nodec, _ := s.New(&s.EntryPointMeta{
		Name:                   "par",
		NotVerifySchema:        true,
		DefaultConfigFilePaths: []string{"conf.json", "config.json", "con.yml"},
		Description:            "测试用foo bar par",
		Usage:                  "foo bar par cmd test",
		LoadAllConfigFile:      true,
		// DebugMode:              true,
	}, &C{
		Field: []int{1, 2, 3},
		OK:    true,
	})
	// fmt.Println("获得schema  ", string(nodec.Schema))
	// s.RegistSubNode(root, nodeb)
	root.RegistSubNode(nodeb)
	// s.RegistSubNode(nodeb, nodec)
	nodeb.RegistSubNode(nodec)
	os.Setenv("FOO_BAR_PAR_A", "123")
	root.Parse([]string{"foo", "bar", "par"})
}

// 	root.Parse([]string{"foo", "bar", "par"})
// }
// func main() {
// 	c := C{
// 		Field: []int{1, 2, 3},
// 		s:     10,
// 	}
// 	t := reflect.TypeOf(c)
// 	// v := reflect.ValueOf(c)
// 	f, ok := t.FieldByName("Field")
// 	if ok {
// 		fmt.Println(f.Type.String())
// 		fmt.Println(f.Name)
// 	}
// 	json.Unmarshal([]byte(`{"field":[1,2,3,4,5]}`), &c)
// 	fmt.Println(c)
// }

// 	vf := v.FieldByName("Field")
// 	if ok {
// 		vf.Set
// 		fmt.Println(vf)

// 	}
// }
