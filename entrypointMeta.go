package schemaentry

import (
	log "github.com/Golang-Tools/loggerhelper"
	"github.com/Golang-Tools/optparams"
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
	parent                 EntryPointInterface
	subcmds                map[string]EntryPointInterface
}

func (ep *EntryPointMeta) Parent() EntryPointInterface {
	return ep.parent
}

func (ep *EntryPointMeta) Subcmds() map[string]EntryPointInterface {
	return ep.subcmds
}

//Meta 获取节点的元数据
func (ep *EntryPointMeta) Meta() *EntryPointMeta {
	return ep
}

//SetChild 为节点设置子节点
//@Params child EntryPointInterface  要作为子节点的节点
func (ep *EntryPointMeta) SetChild(child EntryPointInterface) {
	subcmdName := child.Meta().Name
	if ep.subcmds == nil || len(ep.subcmds) == 0 {
		ep.subcmds = map[string]EntryPointInterface{
			subcmdName: child,
		}
	} else {
		ep.subcmds[subcmdName] = child
	}
}

//SetParent 为节点设置父节点
//@Params parent EntryPointInterface  要作为父节点的节点
func (ep *EntryPointMeta) SetParent(parent EntryPointInterface) {
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

//WithConfig 使用EntryPointMeta实例设置配置
func WithConfig(conf *EntryPointMeta) optparams.Option[EntryPointMeta] {
	return optparams.NewFuncOption(func(o *EntryPointMeta) {
		o.Name = conf.Name
		o.Description = conf.Description
		o.Usage = conf.Usage
		o.DefaultConfigFilePaths = conf.DefaultConfigFilePaths
		o.LoadAllConfigFile = conf.LoadAllConfigFile
		o.NotParseEnv = conf.NotParseEnv
		o.EnvPrefix = conf.EnvPrefix
		o.NotVerifySchema = conf.NotVerifySchema
		o.DebugMode = conf.DebugMode
	})
}

//WithName 设置节点名
func WithName(name string) optparams.Option[EntryPointMeta] {
	return optparams.NewFuncOption(func(o *EntryPointMeta) {
		o.Name = name
	})
}

//WithDescription 设置节点说明文本
func WithDescription(desc string) optparams.Option[EntryPointMeta] {
	return optparams.NewFuncOption(func(o *EntryPointMeta) {
		o.Description = desc
	})
}

//WithUsage 设置节点用法说明
func WithUsage(usage string) optparams.Option[EntryPointMeta] {
	return optparams.NewFuncOption(func(o *EntryPointMeta) {
		o.Usage = usage
	})
}

//WithDefaultConfigFilePaths 设置节点用法说明
func WithDefaultConfigFilePaths(paths ...string) optparams.Option[EntryPointMeta] {
	return optparams.NewFuncOption(func(o *EntryPointMeta) {
		o.DefaultConfigFilePaths = paths
	})
}

//WithEnvPrefix 设置节点用法说明
func WithEnvPrefix(prefix string) optparams.Option[EntryPointMeta] {
	return optparams.NewFuncOption(func(o *EntryPointMeta) {
		o.EnvPrefix = prefix
	})
}

//WithLoadAllConfigFile 设置节点是加载全部配置文件,否则找到第一个后就停止搜索
func WithLoadAllConfigFile() optparams.Option[EntryPointMeta] {
	return optparams.NewFuncOption(func(o *EntryPointMeta) {
		o.LoadAllConfigFile = true
	})
}

//WithNotParseEnv 设置节点不解析环境变量
func WithNotParseEnv() optparams.Option[EntryPointMeta] {
	return optparams.NewFuncOption(func(o *EntryPointMeta) {
		o.NotParseEnv = true
	})
}

//WithNotVerifySchema 设置节点不校验配置的schema
func WithNotVerifySchema() optparams.Option[EntryPointMeta] {
	return optparams.NewFuncOption(func(o *EntryPointMeta) {
		o.NotVerifySchema = true
	})
}

//WithDebugMode 设置打印中间过程的log
func WithDebugMode() optparams.Option[EntryPointMeta] {
	return optparams.NewFuncOption(func(o *EntryPointMeta) {
		o.DebugMode = true
	})
}
