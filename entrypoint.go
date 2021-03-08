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

	log "github.com/Golang-Tools/loggerhelper"
	"github.com/akamensky/argparse"
	"github.com/alecthomas/jsonschema"
	jsoniter "github.com/json-iterator/go"
	"github.com/xeipuuv/gojsonschema"
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

//EntryPointConfig 叶子节点配置接口
type EntryPointConfig interface {
	Main()
}

//EntryPoint 节点类
type EntryPoint struct {
	*EntryPointMeta
	config EntryPointConfig
	Schema []byte
}

//New 创建一个节点对象
//@param meta *EntryPointMeta 为节点的元信息
//@param config ...EntryPointConfig 为一个定义好的struct的对象的指针,根节点和中间节点可以不设置,叶子节点如果不设置则无法执行,最多设置一个
func New(meta *EntryPointMeta, config ...EntryPointConfig) (*EntryPoint, error) {
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
				return nil, errors.New("请设置meta中的Name字段")
			}
		}
		m.DefaultConfigFilePaths = meta.DefaultConfigFilePaths
		m.NotParseEnv = meta.NotParseEnv
		m.NotVerifySchema = meta.NotVerifySchema
		m.EnvPrefix = meta.EnvPrefix
		m.Usage = meta.Usage
	}
	ep.EntryPointMeta = &m

	lenconfig := len(config)
	switch lenconfig {
	case 0:
		{
			return ep, nil
		}
	case 1:
		{
			ep.config = config[0]
			s := jsonschema.Reflect(ep.config)
			schemabytes, err := s.MarshalJSON()
			if err != nil {
				log.Warn("Schema方法返回为空字符串,且config无法映射为jsonschema", log.Dict{"err": err.Error()})
				return ep, nil
			}
			ep.Schema = schemabytes
			return ep, nil
		}
	default:
		{
			return nil, errors.New("最多设置1个config")
		}
	}
}

//RegistConfig 将对象注册进节点
//如果创建时没有设置,那么可以用这个方法设置,
func (ep *EntryPoint) RegistConfig(config EntryPointConfig) {
	if ep.config == nil {
		ep.config = config
	} else {
		log.Warn("节点的config已经被设置过了.")
	}
}

//RegistSubNode 将一个节点注册为当前节点的子节点
//@params child EntryPointNode 节点对象,注意必须传入的是指针
func (ep *EntryPoint) RegistSubNode(child EntryPointNode) {
	RegistSubNode(ep, child)
}

//PassArgsTosub 将解析传导给子节点
func (ep EntryPoint) PassArgsTosub(parser *argparse.Parser, argv []string) {
	if len(argv) <= 1 {
		parser.HelpFunc = func(c *argparse.Command, msg interface{}) string {
			var help string
			help += fmt.Sprintf("命令: %s <subcmd>\n", c.GetName())
			help += fmt.Sprintf("使用:\n")
			help += fmt.Sprintf("  %s\n", ep.Usage)
			help += fmt.Sprintf("说明:\n")
			help += fmt.Sprintf("  %s\n", c.GetDescription())
			help += fmt.Sprintf("支持的子命令:\n")
			for subcmd, subnode := range ep.subcmds {
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
	for subcmd := range ep.EntryPointMeta.subcmds {
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
			if !(argv[1] == "--help" || argv[1] == "-h") {
				help += fmt.Sprintf("未知的子命令`%s`\n", argv[1])
			}
			help += fmt.Sprintf("命令: %s <subcmd>\n", c.GetName())
			help += fmt.Sprintf("使用:\n")
			help += fmt.Sprintf("  %s\n", ep.Usage)
			help += fmt.Sprintf("说明:\n")
			help += fmt.Sprintf("  %s\n", c.GetDescription())
			help += fmt.Sprintf("支持的子命令:\n")
			for subcmd, subnode := range ep.subcmds {
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

//loadConfigFile 加载文件到配置
//@params filename 文件路径
//@return bool 是否结束查找
//@return error 错误信息
func (ep *EntryPoint) loadConfigFile(filename string) (bool, error) {
	if ep.config == nil {
		return true, errors.New("Config为空")
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

	err = json.Unmarshal(fd, &ep.config)
	if err != nil {
		return true, err
	}
	return true, nil

}

//GetConfigFromConfigFile 从设置的或者默认配置文件中获取配置
func (ep *EntryPoint) GetConfigFromConfigFile() error {
	if ep.config == nil {
		return errors.New("Config为nil")
	}
	var conffilepath []string
	if ep.DefaultConfigFilePaths == nil {
		configFileName := strings.Join(GetNodeProgList(ep), "_")
		homepath, err := os.UserHomeDir()
		if err != nil {
			log.Info("find home path error", log.Dict{"err": err})
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
		if ep.LoadAllConfigFile != true && stop {
			if err != nil {
				return err
			}
			break
		} else {
			if err != nil {
				log.Warn("can not loadConfigFile", log.Dict{"filename": filename, "wrong_msg": err})
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
	if ep.config == nil {
		return nil, nil, errors.New("Config为nil")
	}
	flagConfptr := map[string]interface{}{}
	argconfigfilepath := parser.String("c", "config", &argparse.Options{Required: false, Help: "指定配置文件位置"})
	t := reflect.TypeOf(ep.config)
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if unicode.IsLower([]rune(f.Name)[0]) || f.Tag.Get("json") == "-" {
			continue
		}
		description := ""
		title := ""
		jsonschemaTag := f.Tag.Get("jsonschema")
		if jsonschemaTag != "" {
			jstags := strings.Split(jsonschemaTag, ",")
			for _, tag := range jstags {
				if strings.HasPrefix(tag, "description") {
					description = strings.Split(tag, "=")[1]
				}
				if strings.HasPrefix(tag, "title") {
					title = strings.Split(tag, "=")[1]
				}
			}
		}
		switch f.Type.Kind() {
		case reflect.String:
			{
				if description != "" {
					flagConfptr[f.Name] = parser.String(title, f.Name, &argparse.Options{Required: false, Help: description})
				} else {
					flagConfptr[f.Name] = parser.String(title, f.Name, &argparse.Options{Required: false})
				}
			}
		case reflect.Bool:
			{
				if description != "" {
					flagConfptr[f.Name] = parser.Flag(title, f.Name, &argparse.Options{Required: false, Help: description})

				} else {
					flagConfptr[f.Name] = parser.Flag(title, f.Name, &argparse.Options{Required: false})
				}
			}
		case reflect.Int:
			{
				if description != "" {
					flagConfptr[f.Name] = parser.Int(title, f.Name, &argparse.Options{Required: false, Help: description})

				} else {
					flagConfptr[f.Name] = parser.Int(title, f.Name, &argparse.Options{Required: false})
				}
			}
		case reflect.Float64:
			{
				if description != "" {
					flagConfptr[f.Name] = parser.Float(title, f.Name, &argparse.Options{Required: false, Help: description})
				} else {
					flagConfptr[f.Name] = parser.Float(title, f.Name, &argparse.Options{Required: false})
				}
			}
		case reflect.Slice:
			{
				switch f.Type.String() {
				case "[]string":
					{
						if description != "" {
							flagConfptr[f.Name] = parser.StringList(title, f.Name, &argparse.Options{Required: false, Help: description})
						} else {
							flagConfptr[f.Name] = parser.StringList(title, f.Name, &argparse.Options{Required: false})
						}
					}
				case "[]int":
					{
						if description != "" {
							flagConfptr[f.Name] = parser.IntList(title, f.Name, &argparse.Options{Required: false, Help: description})
						} else {
							flagConfptr[f.Name] = parser.IntList(title, f.Name, &argparse.Options{Required: false})
						}
					}
				case "[]float64":
					{
						if description != "" {
							flagConfptr[f.Name] = parser.FloatList(title, f.Name, &argparse.Options{Required: false, Help: description})
						} else {
							flagConfptr[f.Name] = parser.FloatList(title, f.Name, &argparse.Options{Required: false})
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

//GetEnvPrefix 获取实际的EnvPrefix
func (ep *EntryPoint) GetEnvPrefix() string {
	var EnvPrefix string
	if ep.Meta().EnvPrefix != "" {
		EnvPrefix = ep.Meta().EnvPrefix
	} else {
		EnvPrefix = strings.ToUpper(strings.Join(GetNodeProgList(ep), "_"))
	}
	return EnvPrefix
}

//ParseStruct 解析结构体,构造命令行参数解析和环境变量解析,并设置到对象的Config值中
//@params flagConfptr map[string]interface{} 命令行参数除了指定的配置文件位置外的参数->值的指针的映射
//@return error 解析过程中的错误
func (ep *EntryPoint) ParseStruct(flagConfptr map[string]interface{}) error {
	if ep.config == nil {
		return errors.New("Config为nil")
	}
	EnvPrefix := ep.GetEnvPrefix()
	//设置参数
	t := reflect.TypeOf(ep.config)
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	v := reflect.ValueOf(ep.config).Elem()
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if unicode.IsLower([]rune(f.Name)[0]) || f.Tag.Get("json") == "-" {
			continue
		}
		vf := v.Field(i)
		required := false
		jsonschemaTag := f.Tag.Get("jsonschema")
		if jsonschemaTag != "" {
			jstags := strings.Split(jsonschemaTag, ",")
			for _, tag := range jstags {
				if strings.Contains(tag, "required") {
					required = true
					break
				}
			}
		}

		switch f.Type.Kind() {
		case reflect.String:
			{
				//设置环境变量配置
				getenvstr := ""
				if !ep.Meta().NotParseEnv {
					loadenv := fmt.Sprintf("%s_%s", EnvPrefix, strings.ToUpper(f.Name))
					getenvstr = os.Getenv(loadenv)
					log.Info("load config from ENV", log.Dict{"env": loadenv, "value": getenvstr})
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
				if !ep.Meta().NotParseEnv {
					loadenv := fmt.Sprintf("%s_%s", EnvPrefix, strings.ToUpper(f.Name))
					getenvstr = os.Getenv(loadenv)
					log.Info("load config from ENV", log.Dict{"env": loadenv, "value": getenvstr})
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
					if *va {
						log.Info("##########1")
						vf.Set(reflect.ValueOf(*va))
					} else {
						if required {
							log.Info("##########2")
							vf.Set(reflect.ValueOf(*va))
						}
					}
				}

			}
		case reflect.Int:
			{
				//设置环境变量配置
				getenvstr := ""
				if !ep.Meta().NotParseEnv {
					loadenv := fmt.Sprintf("%s_%s", EnvPrefix, strings.ToUpper(f.Name))
					getenvstr = os.Getenv(loadenv)
					log.Info("load config from ENV", log.Dict{"env": loadenv, "value": getenvstr})
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
						vf.Set(reflect.ValueOf(*va))
					}
				}
			}
		case reflect.Float64:
			{
				//设置环境变量配置
				getenvstr := ""
				if !ep.Meta().NotParseEnv {
					loadenv := fmt.Sprintf("%s_%s", EnvPrefix, strings.ToUpper(f.Name))
					getenvstr = os.Getenv(loadenv)
					log.Info("load config from ENV", log.Dict{"env": loadenv, "value": getenvstr})
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
						if !ep.Meta().NotParseEnv {
							loadenv := fmt.Sprintf("%s_%s", EnvPrefix, strings.ToUpper(f.Name))
							getenvstr = os.Getenv(loadenv)
							log.Info("load config from ENV", log.Dict{"env": loadenv, "value": getenvstr})
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
						if !ep.Meta().NotParseEnv {
							loadenv := fmt.Sprintf("%s_%s", EnvPrefix, strings.ToUpper(f.Name))
							getenvstr = os.Getenv(loadenv)
							log.Info("load config from ENV", log.Dict{"env": loadenv, "value": getenvstr})
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
							vf.Set(reflect.ValueOf(r))
						}
						//设置命令行配置
						val, ok := flagConfptr[f.Name]
						if ok {
							va := val.(*[]int)
							if len(*va) != 0 {
								vf.Set(reflect.ValueOf(*va))
							}
						}
					}
				case "[]float64":
					{
						//设置环境变量配置
						getenvstr := ""
						if !ep.Meta().NotParseEnv {
							loadenv := fmt.Sprintf("%s_%s", EnvPrefix, strings.ToUpper(f.Name))
							getenvstr = os.Getenv(loadenv)
							log.Info("load config from ENV", log.Dict{"env": loadenv, "value": getenvstr})
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

//PassArgs 解析叶子节点
//@params parser *argparse.Parser 命令行参数解析器对象
//@params argv []string 待解析的命令行参数
func (ep *EntryPoint) PassArgs(parser *argparse.Parser, argv []string) {
	if ep.config == nil {
		log.Error("叶子节点必须有Config设置")
		os.Exit(1)
	}
	t := reflect.TypeOf(ep.config)
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	count := 0
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if unicode.IsLower([]rune(f.Name)[0]) || f.Tag.Get("json") == "-" {
			continue
		} else {
			count++
		}
	}
	if count == 0 {
		ep.config.Main()
		return
	}
	//默认配置文件
	err := ep.GetConfigFromConfigFile()
	if err != nil {
		log.Warn("GetConfigFromConfigFile wrong", log.Dict{"err": err})
	}

	filepathptr, flagConfptr, err := ep.ConfigPtrFromArgparse(parser, argv)
	if err != nil {
		log.Error("ConfigPtrFromArgparse error", log.Dict{"err": err})
		os.Exit(1)
	}
	//指定配置文件
	filepath := *filepathptr
	if filepath != "" {
		_, err := ep.loadConfigFile(filepath)
		if err != nil {
			log.Warn("loadConfigFile wrong", log.Dict{"err": err, "filename": filepath})
		}
	}
	// 环境变量->命令行
	err = ep.ParseStruct(flagConfptr)
	if err != nil {
		log.Error("ParseStruct error", log.Dict{"err": err})
		os.Exit(1)
	}
	if ep.VerifyConfig() {
		ep.config.Main()
	}
}

//VerifyConfig 验证config是否符合要求
func (ep *EntryPoint) VerifyConfig() bool {
	if ep.Meta().NotVerifySchema == true {
		log.Warn("参数未校验")
		return true
	}
	configLoader := gojsonschema.NewGoLoader(ep.config)
	schemaLoader := gojsonschema.NewBytesLoader(ep.Schema)
	result, err := gojsonschema.Validate(schemaLoader, configLoader)
	if err != nil {
		log.Error("模式校验执行错误", log.Dict{"err": err})
		return false
	}
	if result.Valid() {
		return true
	}
	errs := result.Errors()
	errsS := log.Dict{}
	for index, e := range errs {
		errsS[fmt.Sprintf("conflict-%d", index)] = e.Details()
	}
	log.Error("模式校验错误", errsS)
	return false
}

//Parse 解析节点
func (ep EntryPoint) Parse(argv []string) {
	prog := GetNodeProg(ep)
	parser := argparse.NewParser(prog, ep.EntryPointMeta.Description)
	if ep.IsEndpoint() {
		ep.PassArgs(parser, argv)
	} else {
		ep.PassArgsTosub(parser, argv)
	}
}
