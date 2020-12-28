package main

import (
	"fmt"

	s "github.com/hsz1273327/schema-entry-go/schemaentry"
	jsoniter "github.com/json-iterator/go"
)

var json = jsoniter.ConfigCompatibleWithStandardLibrary

type C struct {
	A     int   `json:"a"`
	Field []int `json:"field"`
	s     int
}

func (c *C) Main() {
	fmt.Println(c.Field)
	fmt.Println(c.A)
}
func (c *C) Schema() string {
	return ""
}

func main() {
	root, _ := s.New(&s.EntryPointMeta{Name: "foo", Usage: "foo cmd test"}, nil)
	nodeb, _ := s.New(&s.EntryPointMeta{Name: "bar", Usage: "foo bar cmd test"}, nil)
	nodec, _ := s.New(&s.EntryPointMeta{Name: "par", Usage: "foo bar par cmd test"}, &C{})
	s.RegistSubNode(root, nodeb)
	s.RegistSubNode(nodeb, nodec)
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
