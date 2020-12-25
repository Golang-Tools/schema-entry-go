package main

import (
	"fmt"

	s "github.com/hsz1273327/schema-entry-go/schemaentry"
)

type A struct {
	*s.EntryPointMeta
	Filed int
}

type B struct {
	*s.EntryPointMeta
	Filed int
}

type C struct {
	*s.EntryPointMeta
	Filed int
}

func main() {
	root := A{EntryPointMeta: &s.EntryPointMeta{Name: "root"}}
	nodeb := B{EntryPointMeta: &s.EntryPointMeta{Name: "bbb"}}
	nodec := C{EntryPointMeta: &s.EntryPointMeta{Name: "ccc"}}
	s.RegistSubNode(root, nodeb)
	s.RegistSubNode(nodeb, nodec)
	fmt.Println(nodec.Meta())
	fmt.Println("ok")
	fmt.Println(s.GetNodeProg(root))
	fmt.Println(s.GetNodeProg(nodec))
}
