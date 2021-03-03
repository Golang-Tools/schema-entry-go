package schemaentry

import (
	log "github.com/Golang-Tools/loggerhelper"
)

//EntryPointMeta 节点的元数据类
type EntryPointMeta struct {
	Name                   string
	Description            string
	Usage                  string
	DefaultConfigFilePaths []string
	LoadAllConfigFile      bool
	NotParseEnv            bool
	EnvPrefix              string
	NotVerifySchema        bool
	parent                 EntryPointNode
	subcmds                map[string]EntryPointNode
}

//Meta 获取节点的元数据
func (ep *EntryPointMeta) Meta() *EntryPointMeta {
	return ep
}

//SetChild 为节点设置子节点
//@params child EntryPointNode 要作为子节点的节点
func (ep *EntryPointMeta) SetChild(child EntryPointNode) {
	subcmdName := child.Meta().Name
	if ep.subcmds == nil || len(ep.subcmds) == 0 {

		ep.subcmds = map[string]EntryPointNode{
			subcmdName: child,
		}
	} else {
		ep.subcmds[subcmdName] = child
	}
}

//SetParent 为节点设置父节点
//@params parent EntryPointNode 要作为父节点的节点
func (ep *EntryPointMeta) SetParent(parent EntryPointNode) {
	if ep.parent == nil {
		ep.parent = parent
	} else {
		log.Warn("节点已经设置过父节点", log.Dict{"parent": ep.parent.Meta().Name})
	}
}

//IsRoot 判断节点的是否为根节点
func (ep *EntryPointMeta) IsRoot() bool {
	if ep.parent == nil {
		return true
	}
	return false
}

//IsEndpoint 判断节点是否为叶子节点
func (ep *EntryPointMeta) IsEndpoint() bool {
	if len(ep.subcmds) == 0 {
		return true
	}
	return false
}
