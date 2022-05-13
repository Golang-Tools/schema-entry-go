package schemaentry

import (
	"errors"
	"fmt"
	"os"

	"github.com/Golang-Tools/optparams"
	"github.com/akamensky/argparse"
)

//EntryPoint 节点类
type EntryPoint struct {
	meta *EntryPointMeta
}

//New 创建一个节点对象
//@params meta *EntryPointMeta 为节点的元信息
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

//Parse 解析节点
func (ep *EntryPoint) Parse(argv []string) {
	prog := GetNodeProg(ep)
	parser := argparse.NewParser(prog, ep.meta.Description)
	ep.PassArgsTosub(parser, argv)
}

//PassArgsTosub 将解析传导给子节点
func (ep EntryPoint) PassArgsTosub(parser *argparse.Parser, argv []string) {
	if len(argv) <= 1 {
		parser.HelpFunc = func(c *argparse.Command, msg interface{}) string {
			var help string
			help += fmt.Sprintf("命令: %s <subcmd>\n", c.GetName())
			help += "使用:\n"
			help += fmt.Sprintf("  %s\n", ep.meta.Usage)
			help += "说明:\n"
			help += fmt.Sprintf("  %s\n", c.GetDescription())
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
		fmt.Print(parser.Help(nil))
		os.Exit(0)
	}
	scmds := []string{}
	insubcmd := false
	for subcmd := range ep.meta.Subcmds() {
		if argv[1] == subcmd {
			insubcmd = true
			break
		} else {
			scmds = append(scmds, subcmd)
		}
	}
	if insubcmd {
		args := []string{argv[0] + " " + argv[1]}
		args = append(args, argv[2:]...)
		ep.meta.Subcmds()[argv[1]].Parse(args)
	} else {
		parser.HelpFunc = func(c *argparse.Command, msg interface{}) string {
			var help string
			if !(argv[1] == "--help" || argv[1] == "-h") {
				help += fmt.Sprintf("未知的子命令`%s`\n", argv[1])
			}
			help += fmt.Sprintf("命令: %s <subcmd>\n", c.GetName())
			help += "使用:\n"
			help += fmt.Sprintf("  %s\n", ep.meta.Usage)
			help += "说明:\n"
			help += fmt.Sprintf("  %s\n", c.GetDescription())
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
		fmt.Print(parser.Help(nil))
		os.Exit(1)
	}
}
