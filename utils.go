package schemaentry

import (
	"strings"
)

type prog struct {
	result []string
}

func (p *prog) getNodeProgIter(node EntryPointNode) {
	meta := node.Meta()
	name := meta.Name
	p.result = append(p.result, name)
	if node.IsRoot() {
		return
	}
	p.getNodeProgIter(meta.parent)
}
func reverse(s []string) []string {
	for i, j := 0, len(s)-1; i < j; i, j = i+1, j-1 {
		s[i], s[j] = s[j], s[i]
	}
	return s
}

//GetNodeProgList 获取节点的prog值
func GetNodeProgList(node EntryPointNode) []string {
	p := prog{
		result: []string{},
	}
	p.getNodeProgIter(node)
	reverse(p.result)
	return p.result
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
