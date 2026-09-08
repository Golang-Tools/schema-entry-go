package schemaentry

import log "github.com/Golang-Tools/loggerhelper/v4"

var logger *log.Log

func init() {
	log.Set(log.WithExtFields(log.Dict{"module": "schema-entry-go"}))
	logger = log.Export()
	log.Set(log.WithExtFields(log.Dict{}))
}

// EndPointConfigInterface 叶子节点配置接口
type EndPointConfigInterface interface {
	Main() //进入时执行的程序
}

type EntryPointInterface interface {
	Meta() *EntryPointMeta
	Schema() []byte
	IsRoot() bool
	IsEndpoint() bool
	SetChild(EntryPointInterface) error
	SetParent(EntryPointInterface) EntryPointInterface
	Parse([]string) error
}

// RegistSubNode 将一对节点互设为父子节点
// @params parent EntryPointInterface 父节点
// @params child EntryPointInterface 子节点
func RegistSubNode(parent, child EntryPointInterface) {
	parent.SetChild(child)
	child.SetParent(parent)
}
