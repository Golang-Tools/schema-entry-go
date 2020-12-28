package schemaentry

import (
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"reflect"
	"strconv"
	"strings"
	"unicode"

	"github.com/akamensky/argparse"
	jsoniter "github.com/json-iterator/go"
)

var json = jsoniter.ConfigCompatibleWithStandardLibrary

//EntryPointNode 节点接口
type EntryPointNode interface {
	Meta() *EntryPointMeta
	IsRoot() bool
	IsEndpoint() bool
	SetChild(EntryPointNode)
	SetParent(EntryPointNode)
	Parse([]string)
	// PassArgsTosub(*argparse.Parser, []string)
}

//EntryPointConfig 节点配置
type EntryPointConfig interface {
	Schema() string
	Main()
}

//EntryPoint 节点类
type EntryPoint struct {
	*EntryPointMeta
	*EntryPointOption
	Config EntryPointConfig
}

//New 创建一个节点对象
//@param config interface{} 为一个定义好的struct的空对象,中间节点可以为nil
//@param meta *EntryPointMeta 为节点的元信息
//@param opt *EntryPointOption 为对解析行为的设置.可以不传表示使用默认设置
func New(meta *EntryPointMeta, config EntryPointConfig, opt ...*EntryPointOption) (*EntryPoint, error) {
	ep := new(EntryPoint)
	m := EntryPointMeta{}
	if meta != nil {
		if meta.Name != "" {
			m.Name = meta.Name
		} else {
			if config != nil {
				v := reflect.ValueOf(config)
				namel := strings.Split(v.Type().String(), ".")
				m.Name = strings.ToLower(namel[len(namel)-1])
			} else {
				return nil, errors.New("请设置meta中的name字段")
			}
		}
		m.DefaultConfigFilePaths = meta.DefaultConfigFilePaths
		m.ParseEnv = meta.ParseEnv
		m.EnvPrefix = meta.EnvPrefix
		m.Usage = meta.Usage
	}
	ep.EntryPointMeta = &m

	var option *EntryPointOption
	optlen := len(opt)
	switch optlen {
	case 0:
		{
			option = &EntryPointOption{}
		}
	case 1:
		{
			option = opt[0]
		}
	default:
		{
			return nil, errors.New("最多只能设置一种解析行为")
		}
	}
	ep.EntryPointOption = option
	ep.Config = config
	return ep, nil
}

//PassArgsTosub 将解析传导给下级
func (ep EntryPoint) PassArgsTosub(parser *argparse.Parser, argv []string) {
	if len(argv) <= 1 {
		parser.HelpFunc = func(c *argparse.Command, msg interface{}) string {
			var help string
			help += fmt.Sprintf("命令: %s <subcmd>\n", c.GetName())
			help += fmt.Sprintf("说明:\n")
			help += fmt.Sprintf("%s\n", c.GetDescription())
			help += fmt.Sprintf("支持的子命令:\n")
			for subcmd, subnode := range ep.subcmds {
				help += fmt.Sprintf(" %s %s\n", subcmd, subnode.Meta().Usage)
			}
			return help
		}
		fmt.Print(parser.Help(nil))
		os.Exit(0)
	}
	scmds := []string{}
	insubcmd := false
	for subcmd, _ := range ep.EntryPointMeta.subcmds {
		if argv[1] == subcmd {
			insubcmd = true
			break
		} else {
			scmds = append(scmds, subcmd)

		}
	}
	if insubcmd {
		args := []string{argv[0] + " " + argv[1]}
		for _, value := range argv[2:] {
			args = append(args, value)
		}
		ep.EntryPointMeta.subcmds[argv[1]].Parse(args)
	} else {
		parser.HelpFunc = func(c *argparse.Command, msg interface{}) string {
			var help string
			help += fmt.Sprintf("未知的子命令`%s`\n", argv[1])
			help += fmt.Sprintf("命令: %s <subcmd>\n", c.GetName())
			help += fmt.Sprintf("说明:\n")
			help += fmt.Sprintf("%s\n", c.GetDescription())
			help += fmt.Sprintf("支持的子命令:\n")
			for subcmd, subnode := range ep.subcmds {
				help += fmt.Sprintf(" %s: %s\n", subcmd, subnode.Meta().Usage)
			}
			return help
		}
		fmt.Print(parser.Help(nil))
		os.Exit(1)
	}
}

//loadConfigFile 加载文件到配置
//@params filename 文件路径
//@return bool 是否结束查找
//@return error 错误信息
func (ep *EntryPoint) loadConfigFile(filename string) (bool, error) {
	if ep.Config == nil {
		return true, errors.New("Config为nil")
	}
	info, err := os.Stat(filename)
	if err != nil {
		if os.IsNotExist(err) {
			return false, fmt.Errorf("find file %s not exist", filename)
		}
		return false, fmt.Errorf("find file %s error: %s", filename, err)

	}
	if info.IsDir() {
		return false, fmt.Errorf("find %s is a dir", filename)
	}
	f, err := os.Open(filename)
	if err != nil {
		return true, err
	}
	defer f.Close()
	fd, err := ioutil.ReadAll(f)
	if err != nil {
		return true, err
	}

	err = json.Unmarshal(fd, &ep.Config)
	if err != nil {
		fmt.Println("###########")
		return true, err
	}
	return true, nil

}

//GetConfigFromConfigFile 从配置文件中获取配置
func (ep *EntryPoint) GetConfigFromConfigFile() error {
	if ep.Config == nil {
		return errors.New("Config为nil")
	}
	var conffilepath []string
	if ep.DefaultConfigFilePaths == nil {
		configFileName := strings.Join(GetNodeProgList(ep), "_")
		homepath, err := os.UserHomeDir()
		if err != nil {
			fmt.Println("find home error")
			conffilepath = []string{
				fmt.Sprintf("./%s.json", configFileName),
				fmt.Sprintf("/%s/config.json", configFileName),
			}
		} else {
			conffilepath = []string{
				fmt.Sprintf("./%s.json", configFileName),
				fmt.Sprintf("%s/%s/config.json", homepath, configFileName),
				fmt.Sprintf("/%s/config.json", configFileName),
			}
		}
	} else {
		conffilepath = ep.DefaultConfigFilePaths
	}
	for _, filename := range conffilepath {
		stop, err := ep.loadConfigFile(filename)
		if stop {
			if err != nil {
				return err
			}
			break
		} else {
			if err != nil {
				fmt.Println(err.Error())
			}
		}
	}
	return nil
}

//ConfigPtrFromArgparse 构造命令行参数解析,并获取flag的ptr
//@params parser *argparse.Parser flag解析器
//@params argv []string 待解析的命令行参数
//@return *string 指定configfile位置字符串
//@return map[string]interface{} flag的ptr位置
//@return error 错误信息
func (ep *EntryPoint) ConfigPtrFromArgparse(parser *argparse.Parser, argv []string) (*string, map[string]interface{}, error) {
	if ep.Config == nil {
		return nil, nil, errors.New("Config为nil")
	}
	flagConfptr := map[string]interface{}{}
	argconfigfilepath := parser.String("c", "config_path", &argparse.Options{Required: false, Help: "指定配置文件位置"})
	t := reflect.TypeOf(ep.Config)
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if unicode.IsLower([]rune(f.Name)[0]) || f.Tag.Get("json") == "-" {
			continue
		}
		description := ""
		jsonschemaTag := f.Tag.Get("jsonschema")
		if jsonschemaTag != "" {
			jstags := strings.Split(jsonschemaTag, ",")
			for _, tag := range jstags {
				if strings.HasPrefix(tag, "description") {
					description = strings.Split(tag, "=")[1]
					break
				}
			}
		}
		switch f.Type.Kind() {
		case reflect.String:
			{
				if description != "" {
					flagConfptr[f.Name] = parser.String("", f.Name, &argparse.Options{Required: false, Help: description})
				} else {
					flagConfptr[f.Name] = parser.String("", f.Name, &argparse.Options{Required: false})
				}
			}
		case reflect.Bool:
			{
				if description != "" {
					flagConfptr[f.Name] = parser.Flag("", f.Name, &argparse.Options{Required: false, Help: description})

				} else {
					flagConfptr[f.Name] = parser.Flag("", f.Name, &argparse.Options{Required: false})
				}
			}
		case reflect.Int:
			{
				if description != "" {
					flagConfptr[f.Name] = parser.Int("", f.Name, &argparse.Options{Required: false, Help: description})

				} else {
					flagConfptr[f.Name] = parser.Int("", f.Name, &argparse.Options{Required: false})
				}
			}
		case reflect.Float64:
			{
				if description != "" {
					flagConfptr[f.Name] = parser.Float("", f.Name, &argparse.Options{Required: false, Help: description})
				} else {
					flagConfptr[f.Name] = parser.Float("", f.Name, &argparse.Options{Required: false})
				}
			}
		case reflect.Slice:
			{
				switch f.Type.String() {
				case "[]string":
					{
						if description != "" {
							flagConfptr[f.Name] = parser.StringList("", f.Name, &argparse.Options{Required: false, Help: description})
						} else {
							flagConfptr[f.Name] = parser.StringList("", f.Name, &argparse.Options{Required: false})
						}
					}
				case "[]int":
					{
						if description != "" {
							flagConfptr[f.Name] = parser.IntList("", f.Name, &argparse.Options{Required: false, Help: description})
						} else {
							flagConfptr[f.Name] = parser.IntList("", f.Name, &argparse.Options{Required: false})
						}
					}
				case "[]float64":
					{
						if description != "" {
							flagConfptr[f.Name] = parser.FloatList("", f.Name, &argparse.Options{Required: false, Help: description})
						} else {
							flagConfptr[f.Name] = parser.FloatList("", f.Name, &argparse.Options{Required: false})
						}
					}
				default:
					{
						return nil, nil, fmt.Errorf("字段%s是未支持的类型%v", f.Name, f.Type)
					}
				}
			}

		default:
			{
				return nil, nil, fmt.Errorf("字段%s是未支持的类型%v", f.Name, f.Type)
			}
		}
	}
	err := parser.Parse(argv)
	if err != nil {
		// In case of error print error and print usage
		// This can also be done by passing -h or --help flags
		fmt.Print(parser.Usage(err))
		return nil, nil, err
	}
	return argconfigfilepath, flagConfptr, nil
}

//ParseStruct 解析结构,构造命令行参数解析和环境变量解析
func (ep *EntryPoint) ParseStruct(flagConfptr map[string]interface{}) error {

	if ep.Config == nil {
		return errors.New("Config为nil")
	}
	var EnvPrefix string
	if ep.Meta().EnvPrefix != "" {
		EnvPrefix = ep.Meta().EnvPrefix
	} else {
		EnvPrefix = strings.ToUpper(strings.Join(GetNodeProgList(ep), "_"))
	}
	//设置参数
	t := reflect.TypeOf(ep.Config)
	v := reflect.ValueOf(ep.Config)
	//v := reflect.ValueOf(&ep.Config).Elem()
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if unicode.IsLower([]rune(f.Name)[0]) || f.Tag.Get("json") == "-" {
			continue
		}
		vf := v.Field(i)
		switch f.Type.Kind() {
		case reflect.String:
			{
				//设置环境变量配置
				getenvstr := ""
				if ep.Meta().ParseEnv {
					getenvstr = os.Getenv(fmt.Sprintf("%s_%s", EnvPrefix, strings.ToUpper(f.Name)))
				}
				if getenvstr != "" {
					vf.Set(reflect.ValueOf(getenvstr))
				}
				//设置命令行配置
				val, ok := flagConfptr[f.Name]
				if ok {
					va := val.(*string)
					if *va != "" {
						vf.Set(reflect.ValueOf(*va))
					}
				}
			}
		case reflect.Bool:
			{
				//设置环境变量配置
				getenvstr := ""
				if ep.Meta().ParseEnv {
					getenvstr = os.Getenv(fmt.Sprintf("%s_%s", EnvPrefix, strings.ToUpper(f.Name)))
				}
				if getenvstr != "" {
					if strings.ToUpper(getenvstr) == "TRUE" {
						vf.Set(reflect.ValueOf(true))
					} else {
						vf.Set(reflect.ValueOf(false))
					}
				}
				//设置命令行配置
				val, ok := flagConfptr[f.Name]
				if ok {
					va := val.(*bool)
					vf.Set(reflect.ValueOf(*va))
				}
			}
		case reflect.Int:
			{
				//设置环境变量配置
				getenvstr := ""
				if ep.Meta().ParseEnv {
					getenvstr = os.Getenv(fmt.Sprintf("%s_%s", EnvPrefix, strings.ToUpper(f.Name)))
				}
				if getenvstr != "" {
					intv, err := strconv.Atoi(getenvstr)
					if err != nil {
						return err
					}
					vf.Set(reflect.ValueOf(intv))
				}
				//设置命令行配置
				val, ok := flagConfptr[f.Name]
				if ok {
					va := val.(*int)
					if *va != 0 {
						if vf.CanSet() {
							fmt.Println("@@@@@@@@@@@@@@@@@can set")
							vf.Set(reflect.ValueOf(*va))
						}
					}
				}
			}
		case reflect.Float64:
			{
				//设置环境变量配置
				getenvstr := ""
				if ep.Meta().ParseEnv {
					getenvstr = os.Getenv(fmt.Sprintf("%s_%s", EnvPrefix, strings.ToUpper(f.Name)))
				}
				if getenvstr != "" {
					fv, err := strconv.ParseFloat(getenvstr, 64)
					if err != nil {
						return err
					}
					vf.Set(reflect.ValueOf(fv))
				}
				//设置命令行配置
				val, ok := flagConfptr[f.Name]
				if ok {
					va := val.(*float64)
					if *va != 0.0 {
						vf.Set(reflect.ValueOf(*va))
					}
				}
			}
		case reflect.Slice:
			{
				switch f.Type.String() {
				case "[]string":
					{
						//设置环境变量配置
						getenvstr := ""
						if ep.Meta().ParseEnv {
							getenvstr = os.Getenv(fmt.Sprintf("%s_%s", EnvPrefix, strings.ToUpper(f.Name)))
						}
						if getenvstr != "" {
							sl := strings.Split(getenvstr, ",")
							vf.Set(reflect.ValueOf(sl))
						}
						//设置命令行配置
						val, ok := flagConfptr[f.Name]
						if ok {
							va := val.(*[]string)
							if len(*va) != 0 {
								vf.Set(reflect.ValueOf(*va))
							}
						}
					}
				case "[]int":
					{
						//设置环境变量配置
						getenvstr := ""
						if ep.Meta().ParseEnv {
							getenvstr = os.Getenv(fmt.Sprintf("%s_%s", EnvPrefix, strings.ToUpper(f.Name)))
						}
						if getenvstr != "" {
							r := []int{}
							for _, ele := range strings.Split(getenvstr, ",") {
								intv, err := strconv.Atoi(ele)
								if err != nil {
									return err
								}
								r = append(r, intv)
							}
							if vf.CanSet() {
								fmt.Println("@@@@@@@@@@@@@@@@@can set")
								vf.Set(reflect.ValueOf(r))
							}
						}
						//设置命令行配置
						val, ok := flagConfptr[f.Name]
						if ok {
							va := val.(*[]int)
							if len(*va) != 0 {
								fmt.Println(*va)
								if vf.CanSet() {
									fmt.Println("@@@@@@@@@@@@@@@@@can set")
									vf.Set(reflect.ValueOf(*va))
								}
							}
						}
					}
				case "[]float64":
					{
						//设置环境变量配置
						getenvstr := ""
						if ep.Meta().ParseEnv {
							getenvstr = os.Getenv(fmt.Sprintf("%s_%s", EnvPrefix, strings.ToUpper(f.Name)))
						}
						if getenvstr != "" {
							r := []float64{}
							for _, ele := range strings.Split(getenvstr, ",") {
								fv, err := strconv.ParseFloat(ele, 64)
								if err != nil {
									return err
								}
								r = append(r, fv)
							}
							vf.Set(reflect.ValueOf(r))
						}
						//设置命令行配置
						val, ok := flagConfptr[f.Name]
						if ok {
							va := val.(*[]float64)
							if len(*va) != 0 {
								vf.Set(reflect.ValueOf(*va))
							}
						}
					}
				default:
					{
						return fmt.Errorf("字段%s是未支持的类型%v", f.Name, f.Type)
					}
				}
			}
		default:
			{
				return fmt.Errorf("字段%s是未支持的类型%v", f.Name, f.Type)
			}
		}
	}
	return nil
}

//PassArgs 解析根节点
func (ep *EntryPoint) PassArgs(parser *argparse.Parser, argv []string) {
	err := ep.GetConfigFromConfigFile()
	if err != nil {
		fmt.Println(err)
	}
	filepathptr, flagConfptr, err := ep.ConfigPtrFromArgparse(parser, argv)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	filepath := *filepathptr
	if filepath != "" {
		_, err := ep.loadConfigFile(filepath)
		if err != nil {
			fmt.Println(err)
		}
	}
	err = ep.ParseStruct(flagConfptr)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	// if ep.Config.Schema() != "" {

	// }
	ep.Config.Main()

}

//Parse 解析节点
func (ep EntryPoint) Parse(argv []string) {
	prog := GetNodeProg(ep)
	parser := argparse.NewParser(prog, ep.EntryPointMeta.Usage)
	if ep.IsEndpoint() {
		ep.PassArgs(parser, argv)
	} else {
		ep.PassArgsTosub(parser, argv)
	}
}
func (ep EntryPoint) runMain() {
	ep.Config.Main()
}
