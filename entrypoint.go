package schemaentry

import (
	"errors"
	"fmt"
	"os"

	"github.com/Golang-Tools/optparams"
)

// EntryPoint 节点类
type EntryPoint struct {
	meta *EntryPointMeta
}

// New 创建一个节点对象
// @params meta *EntryPointMeta 为节点的元信息
func NewEntryPoint(opts ...optparams.Option[EntryPointMeta]) (*EntryPoint, error) {
	ep := new(EntryPoint)
	m := optparams.GetOption(new(EntryPointMeta), opts...)
	if m.Name == "" {
		return nil, errors.New("请设置meta中的Name字段")
	}
	ep.meta = m
	return ep, nil
}

func (ep *EntryPoint) Schema() []byte {
	return nil
}

func (ep *EntryPoint) Meta() *EntryPointMeta {
	return ep.meta.Meta()
}

func (ep *EntryPoint) IsRoot() bool {
	return ep.meta.IsRoot()
}

func (ep *EntryPoint) IsEndpoint() bool {
	return false
}

func (ep *EntryPoint) SetChild(child EntryPointInterface) error {
	ep.meta.SetChild(child)
	child.Meta().SetParent(ep)
	return nil
}

func (ep *EntryPoint) SetParent(parent EntryPointInterface) EntryPointInterface {
	parent.Meta().SetChild(ep)
	ep.meta.SetParent(parent)
	return parent
}

// Parse 解析节点生成命令行说明文档
func (ep *EntryPoint) Parse(argv []string) {
	ep.passArgsTosub(argv)
}

// passArgsTosub 将解析传导给子节点
func (ep EntryPoint) passArgsTosub(argv []string) {
	if len(argv) <= 1 {
		fmt.Print(ep.helpString(false, ""))
		os.Exit(0)
	}
	insubcmd := false
	for subcmd := range ep.meta.Subcmds() {
		if argv[1] == subcmd {
			insubcmd = true
			break
		}
	}
	if insubcmd {
		args := []string{argv[0] + " " + argv[1]}
		args = append(args, argv[2:]...)
		ep.meta.Subcmds()[argv[1]].Parse(args)
	} else {
		unknown := ""
		if !(argv[1] == "--help" || argv[1] == "-h") {
			unknown = argv[1]
		}
		fmt.Print(ep.helpString(true, unknown))
		os.Exit(1)
	}
}

// helpString 生成枝/根节点的帮助文本,列出其支持的子命令
// @params isError bool 是否因未知子命令触发的报错帮助
// @params unknown string 未知的子命令名(为空则忽略)
// @returns string 帮助文本
func (ep EntryPoint) helpString(isError bool, unknown string) string {
	prog := GetNodeProg(&ep)
	var help string
	if isError && unknown != "" {
		help += fmt.Sprintf("未知的子命令`%s`\n", unknown)
	}
	help += fmt.Sprintf("命令: %s <subcmd>\n", prog)
	help += "使用:\n"
	help += fmt.Sprintf("  %s\n", ep.meta.Usage)
	help += "说明:\n"
	help += fmt.Sprintf("  %s\n", ep.meta.Description)
	help += "支持的子命令:\n"
	for subcmd, subnode := range ep.meta.Subcmds() {
		help += fmt.Sprintf("  子命令: %s\n", subcmd)
		help += fmt.Sprintf("    说明: %s\n", subnode.Meta().Usage)
		if subnode.IsEndpoint() && !subnode.Meta().NotParseEnv {
			EnvPrefix := GetNodeEnvPrefix(subnode)
			help += fmt.Sprintf("    环境变量前缀: %s\n", EnvPrefix)
		}
	}
	return help
}
