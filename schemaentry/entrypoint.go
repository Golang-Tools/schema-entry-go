package schemaentry

import (
	"github.com/akamensky/argparse"
)

//EntryPointNode 节点接口
type EntryPointNode interface {
	Meta() *EntryPointMeta
	IsRoot() bool
	IsEndpoint() bool
	SetChild(EntryPointNode)
	SetParent(EntryPointNode)
}

//EntryPoint 节点类
type EntryPoint struct {
	*EntryPointMeta
	*EntryPointConfig
}

func New(name string, config *EntryPointConfig) *EntryPoint {
	ep := new(EntryPoint)
	ep.EntryPointMeta = &EntryPointMeta{Name: name}
	ep.EntryPointConfig = config
	return ep
}

func (ep *EntryPoint) Parse() {
	prog := GetNodeProg(ep)
	if ep.IsEndpoint() {

	} else {
		parser := argparse.NewParser("progname", "Description of my awesome program. It can be as long as I wish it to be")
	}
}
