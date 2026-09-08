package schemaentry

import (
	"errors"
	"fmt"

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

// Parse 解析节点,将命令行下传给匹配的子节点并返回其解析结果
// 无子命令或请求帮助(-h/--help)时返回包装 ErrHelp 的 UsageError
func (ep *EntryPoint) Parse(argv []string) error {
	return ep.passArgsTosub(argv)
}

// passArgsTosub 将解析传导给子节点
func (ep EntryPoint) passArgsTosub(argv []string) error {
	if len(argv) <= 1 {
		return &UsageError{Usage: ep.helpString(), Err: ErrHelp}
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
		return ep.meta.Subcmds()[argv[1]].Parse(args)
	}
	if argv[1] == "--help" || argv[1] == "-h" {
		return &UsageError{Usage: ep.helpString(), Err: ErrHelp}
	}
	return &UsageError{Usage: ep.helpString(), Err: fmt.Errorf("未知的子命令`%s`", argv[1])}
}

// helpString 生成枝/根节点的帮助文本,列出其支持的子命令
// @returns string 帮助文本
func (ep EntryPoint) helpString() string {
	prog := GetNodeProg(&ep)
	var help string
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
