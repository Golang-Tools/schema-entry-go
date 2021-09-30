package schemaentry

import (
	log "github.com/Golang-Tools/loggerhelper"
)

//EntryPointMeta 节点的元数据类
type EntryPointMeta struct {
	Name                   string   //节点名
	Description            string   //节点简介
	Usage                  string   //节点用法介绍
	DefaultConfigFilePaths []string //节点执行的默认配置文件路径列表
	LoadAllConfigFile      bool     //是否加载全部配置文件,否则找到第一个后就停止搜索
	NotParseEnv            bool     //是否不解析环境变量
	EnvPrefix              string   //解析环境变量时的前缀
	NotVerifySchema        bool     //是否不校验配置的schema
	DebugMode              bool     //当设置为debugmode时才会打印中间过程的log
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
	return ep.parent == nil

}

//IsEndpoint 判断节点是否为叶子节点
func (ep *EntryPointMeta) IsEndpoint() bool {
	return len(ep.subcmds) == 0
}
