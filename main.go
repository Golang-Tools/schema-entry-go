package main

import (
	"fmt"
	"reflect"
	// s "github.com/hsz1273327/schema-entry-go/schemaentry"
)

type C struct {
	Field []int `json:"field,o"`
	s     int
}

func (c *C) Main() {
	fmt.Println(c.Field)
}

// func main() {
// 	root, _ := s.New(&s.EntryPointMeta{Name: "foo", Usage: "foo cmd test"}, nil)
// 	nodeb, _ := s.New(&s.EntryPointMeta{Name: "bar", Usage: "foo bar cmd test"}, nil)
// 	nodec, _ := s.New(&s.EntryPointMeta{Name: "par", Usage: "foo bar par cmd test"}, &C{})
// 	s.RegistSubNode(root, nodeb)
// 	s.RegistSubNode(nodeb, nodec)

// 	root.Parse([]string{"foo", "bar", "par"})
// }
func main() {
	c := C{
		Field: []int{1, 2, 3},
		s:     10,
	}
	t := reflect.TypeOf(c)
	// v := reflect.ValueOf(c)
	f, ok := t.FieldByName("Field")
	if ok {
		fmt.Println(f.Type.String())
		fmt.Println(f.Name)
	}
}

// 	vf := v.FieldByName("Field")
// 	if ok {
// 		vf.Set
// 		fmt.Println(vf)

// 	}
// }
