package main

import (
	"fmt"
	"os"

	s "github.com/Golang-Tools/schema-entry-go"
	"github.com/alecthomas/jsonschema"
	jsoniter "github.com/json-iterator/go"
)

var json = jsoniter.ConfigCompatibleWithStandardLibrary

type C struct {
	A     int   `json:"a" jsonschema:"required,title=field,description=测试int"`
	Field []int `json:"field" jsonschema:"required,title=field,description=测试列表"`
	s     int
}

func (c *C) Main() {
	fmt.Println(c.Field)
	fmt.Println(c.A)
}
func (c *C) Schema() string {
	s := jsonschema.Reflect(c)
	schemabytes, err := s.MarshalJSON()
	if err != nil {
		fmt.Println(err)
		return ""
	}
	return string(schemabytes)
}

func main() {
	root, _ := s.New(&s.EntryPointMeta{Name: "foo", Usage: "foo cmd test"})
	nodeb, _ := s.New(&s.EntryPointMeta{Name: "bar", Usage: "foo bar cmd test"})
	nodec, _ := s.New(&s.EntryPointMeta{Name: "par", Usage: "foo bar par cmd test", ParseEnv: true}, &C{
		Field: []int{1, 2, 3},
	})
	// s.RegistSubNode(root, nodeb)
	root.RegistSubNode(nodeb)
	// s.RegistSubNode(nodeb, nodec)
	nodeb.RegistSubNode(nodec)
	os.Setenv("FOO_BAR_PAR_A", "123")
	root.Parse([]string{"foo", "bar", "par", "--help"})
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
