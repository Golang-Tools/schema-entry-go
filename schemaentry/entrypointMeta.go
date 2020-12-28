package schemaentry

//EntryPointMeta 节点类
type EntryPointMeta struct {
	Name                   string
	Usage                  string
	DefaultConfigFilePaths []string
	ParseEnv               bool
	EnvPrefix              string
	parent                 EntryPointNode
	subcmds                map[string]EntryPointNode
}

//Meta 获取节点的元数据
func (ep *EntryPointMeta) Meta() *EntryPointMeta {
	return ep
}

//SetChild 为节点设置子节点
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

//SetParent 为节点设置子节点
func (ep *EntryPointMeta) SetParent(parent EntryPointNode) {
	ep.parent = parent
}

//IsRoot 判断节点的是否为根节点
func (ep *EntryPointMeta) IsRoot() bool {
	if ep.parent == nil {
		return true
	}
	return false
}

//IsEndpoint 判断节点是否为终点
func (ep *EntryPointMeta) IsEndpoint() bool {
	if len(ep.subcmds) == 0 {
		return true
	}
	return false
}
