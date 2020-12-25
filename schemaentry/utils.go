package schemaentry

import (
	"fmt"
	"reflect"
	"strings"
)

//GetNodeName 获取节点名
func GetNodeName(node EntryPointNode) string {
	v := reflect.ValueOf(node)
	field, ok := v.Type().FieldByName("Name")
	if !ok || field.Type.Kind() != reflect.String || v.FieldByName("Name").String() == "" {
		namel := strings.Split(v.Type().String(), ".")
		return strings.ToLower(namel[len(namel)-1])
	}
	return v.FieldByName("Name").String()
}

type prog struct {
	result []string
}

func (p *prog) getNodeProgIter(node EntryPointNode) {
	fmt.Println(p.result)
	if node.IsRoot() {
		return
	}
	meta := node.Meta()
	fmt.Println(meta)
	parentName := GetNodeName(meta.parent)
	p.result = append(p.result, parentName)
	p.getNodeProgIter(meta.parent)
}
func reverse(s []string) []string {
	for i, j := 0, len(s)-1; i < j; i, j = i+1, j-1 {
		s[i], s[j] = s[j], s[i]
	}
	return s
}

//GetNodeProg 获取节点的prog值
func GetNodeProg(node EntryPointNode) string {
	p := prog{
		result: []string{},
	}
	p.getNodeProgIter(node)
	reverse(p.result)
	return strings.Join(p.result, " ")
}

//RegistSubNode 为节点注册子节点
func RegistSubNode(parent, child EntryPointNode) {
	parent.SetChild(child)
	child.SetParent(parent)
}
