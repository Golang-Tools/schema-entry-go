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
	"github.com/Golang-Tools/optparams"
	"github.com/akamensky/argparse"
	"github.com/invopop/jsonschema"
	"github.com/xeipuuv/gojsonschema"
	"gopkg.in/yaml.v2"
)

var (
	//ErrNotAllowSetChildToEndPoint 叶子节点无法设置子节点
	ErrNotAllowSetChildToEndPoint = errors.New("not allow set child to endpoint")
)

//EntryPoint 节点类
//@generic T EndPointConfigInterface 内部`config`字段的类型
type EndPoint[T EndPointConfigInterface] struct {
	config T
	meta   *EntryPointMeta
}

//NewEndPoint创建一个节点对象
//@generic T EndPointConfigInterface EntryPoint泛型的实例化参数
//@params meta *EntryPointMeta 为节点的元信息
func NewEndPoint[T EndPointConfigInterface](opts ...optparams.Option[EntryPointMeta]) (*EndPoint[T], error) {
	config := new(T)
	ep := new(EndPoint[T])
	m := optparams.GetOption(new(EntryPointMeta), opts...)
	if m.Name == "" {
		v := reflect.ValueOf(*config)
		namel := strings.Split(v.Type().String(), ".")
		m.Name = strings.ToLower(namel[len(namel)-1])
	}
	ep.meta = m
	ep.config = *config
	return ep, nil
}

func (ep *EndPoint[T]) Schema() []byte {
	s := jsonschema.Reflect(ep.config)
	schemabytes, err := s.MarshalJSON()
	if err != nil {
		log.Warn("config结构无法映射为jsonschema", log.Dict{"err": err.Error()})
		return nil
	}
	return schemabytes
}

func (ep *EndPoint[T]) Meta() *EntryPointMeta {
	return ep.meta.Meta()
}

func (ep *EndPoint[T]) IsRoot() bool {
	return ep.meta.IsRoot()
}

func (ep *EndPoint[T]) IsEndpoint() bool {
	return true
}

func (ep *EndPoint[T]) SetChild(child EntryPointInterface) error {
	return ErrNotAllowSetChildToEndPoint
}

func (ep *EndPoint[T]) SetParent(parent EntryPointInterface) EntryPointInterface {
	parent.Meta().SetChild(ep)
	ep.meta.SetParent(parent)
	return parent
}

//Parse 解析节点
func (ep *EndPoint[T]) Parse(argv []string) {
	prog := GetNodeProg(ep)
	parser := argparse.NewParser(prog, ep.meta.Description)
	ep.PassArgs(parser, argv)
}

//PassArgs 解析叶子节点
//@Params parser *argparse.Parser 命令行参数解析器对象
//@Params argv []string 待解析的命令行参数
func (ep *EndPoint[T]) PassArgs(parser *argparse.Parser, argv []string) {
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
			log.Error("load ConfigFile wrong", log.Dict{"err": err, "filename": filepath})
			os.Exit(1)
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

//GetConfigFromConfigFile 从设置的或者默认配置文件中获取配置
func (ep *EndPoint[T]) GetConfigFromConfigFile() error {
	var conffilepath []string
	if ep.meta.DefaultConfigFilePaths == nil {
		configFileName := strings.Join(GetNodeProgList(ep), "_")
		homepath, err := os.UserHomeDir()
		if err != nil {
			if ep.meta.DebugMode {
				log.Info("find home path error", log.Dict{"err": err})
			}
			conffilepath = []string{
				fmt.Sprintf("./%s.json", configFileName),
				fmt.Sprintf("/%s/config.json", configFileName),
				fmt.Sprintf("./%s.yml", configFileName),
				fmt.Sprintf("/%s/config.yml", configFileName),
			}
		} else {
			conffilepath = []string{
				fmt.Sprintf("./%s.json", configFileName),
				fmt.Sprintf("%s/%s/config.json", homepath, configFileName),
				fmt.Sprintf("/%s/config.json", configFileName),
				fmt.Sprintf("./%s.yml", configFileName),
				fmt.Sprintf("%s/%s/config.yml", homepath, configFileName),
				fmt.Sprintf("/%s/config.yml", configFileName),
			}
		}
	} else {
		conffilepath = ep.meta.DefaultConfigFilePaths
	}
	for _, filename := range conffilepath {
		stop, err := ep.loadConfigFile(filename)
		if !ep.meta.LoadAllConfigFile && stop {
			if err != nil {
				return err
			}
			break
		} else {
			if err != nil {
				if ep.meta.DebugMode {
					log.Info("can not load ConfigFile", log.Dict{"filename": filename, "wrong_msg": err})
				}
			}
		}
	}
	return nil
}

//loadConfigFile 加载文件到配置
//@Params filename 文件路径
//@returns bool 是否结束查找
//@returns error 错误信息
func (ep *EndPoint[T]) loadConfigFile(filename string) (bool, error) {
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
	if strings.HasSuffix(filename, "json") {
		err := json.Unmarshal(fd, &ep.config)
		if err != nil {
			return true, err
		}
	} else if strings.HasSuffix(filename, "yml") || strings.HasSuffix(filename, "yaml") {
		err := yaml.Unmarshal(fd, &ep.config)
		if err != nil {
			return true, err
		}
	} else {
		return false, fmt.Errorf("%s has not supported suffix", filename)
	}
	return true, nil
}

//ConfigPtrFromArgparse 构造命令行参数解析,并获取flag的ptr
//@Params parser *argparse.Parser flag解析器
//@Params argv []string 待解析的命令行参数
//@Returns *string 指定configfile位置字符串
//@Returns map[string]interface{} flag的ptr位置
//@Returns error 错误信息
func (ep *EndPoint[T]) ConfigPtrFromArgparse(parser *argparse.Parser, argv []string) (*string, map[string]interface{}, error) {
	defer func() {
		if err := recover(); err != nil {
			log.Error("ConfigPtrFromArgparse get error", log.Dict{"err": err})
			os.Exit(1)
		}
	}()
	r := jsonschema.Reflector{
		DoNotReference: true,
	}
	schema := r.Reflect(ep.config)

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
		// TOtest
		description := ""
		title := ""
		name := ReflectFieldName(f)
		fieldschema_i, ok := schema.Properties.Get(name)
		if !ok {
			log.Warn("schema.Properties.Get(name) not ok", log.Dict{"name": name, "Properties": schema.Properties})
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
		} else {
			fieldschema := fieldschema_i.(*jsonschema.Schema)
			description = fieldschema.Description
			title = fieldschema.Title
		}
		// TOtest

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

//ParseStruct 解析结构体,构造命令行参数解析和环境变量解析,并设置到对象的Config值中
//@Params flagConfptr map[string]interface{} 命令行参数除了指定的配置文件位置外的参数->值的指针的映射
//@Returns error 解析过程中的错误
func (ep *EndPoint[T]) ParseStruct(flagConfptr map[string]interface{}) error {
	r := jsonschema.Reflector{
		DoNotReference: true,
	}
	schema := r.Reflect(ep.config)
	EnvPrefix := ep.GetEnvPrefix()
	//设置参数
	t := reflect.TypeOf(ep.config)
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	v := reflect.ValueOf(&ep.config).Elem()
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if unicode.IsLower([]rune(f.Name)[0]) || f.Tag.Get("json") == "-" || f.Tag.Get("yaml") == "-" {
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
		name := ReflectFieldName(f)
		fieldschema_i, ok := schema.Properties.Get(name)
		if ok {
			fieldschema := fieldschema_i.(*jsonschema.Schema)
			if fieldschema.Default != nil {
				vf.Set(reflect.ValueOf(fieldschema.Default))
			}
		}

		switch f.Type.Kind() {
		case reflect.String:
			{
				//设置环境变量配置
				getenvstr := ""
				if !ep.meta.NotParseEnv {
					loadenv := fmt.Sprintf("%s_%s", EnvPrefix, strings.ToUpper(f.Name))
					getenvstr = os.Getenv(loadenv)
					if ep.meta.DebugMode {
						log.Info("load config from ENV", log.Dict{"env": loadenv, "value": getenvstr})
					}
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
				if !ep.meta.NotParseEnv {
					loadenv := fmt.Sprintf("%s_%s", EnvPrefix, strings.ToUpper(f.Name))
					getenvstr = os.Getenv(loadenv)
					if ep.meta.DebugMode {
						log.Info("load config from ENV", log.Dict{"env": loadenv, "value": getenvstr})
					}
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
						vf.Set(reflect.ValueOf(*va))
					} else {
						if required {
							vf.Set(reflect.ValueOf(*va))
						}
					}
				}

			}
		case reflect.Int:
			{
				//设置环境变量配置
				getenvstr := ""
				if !ep.meta.NotParseEnv {
					loadenv := fmt.Sprintf("%s_%s", EnvPrefix, strings.ToUpper(f.Name))
					getenvstr = os.Getenv(loadenv)
					if ep.meta.DebugMode {
						log.Info("load config from ENV", log.Dict{"env": loadenv, "value": getenvstr})
					}
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
				if !ep.meta.NotParseEnv {
					loadenv := fmt.Sprintf("%s_%s", EnvPrefix, strings.ToUpper(f.Name))
					getenvstr = os.Getenv(loadenv)
					if ep.meta.DebugMode {
						log.Info("load config from ENV", log.Dict{"env": loadenv, "value": getenvstr})
					}
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
						if !ep.meta.NotParseEnv {
							loadenv := fmt.Sprintf("%s_%s", EnvPrefix, strings.ToUpper(f.Name))
							getenvstr = os.Getenv(loadenv)
							if ep.meta.DebugMode {
								log.Info("load config from ENV", log.Dict{"env": loadenv, "value": getenvstr})
							}
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
						if !ep.meta.NotParseEnv {
							loadenv := fmt.Sprintf("%s_%s", EnvPrefix, strings.ToUpper(f.Name))
							getenvstr = os.Getenv(loadenv)
							if ep.meta.DebugMode {
								log.Info("load config from ENV", log.Dict{"env": loadenv, "value": getenvstr})
							}
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
						if !ep.meta.NotParseEnv {
							loadenv := fmt.Sprintf("%s_%s", EnvPrefix, strings.ToUpper(f.Name))
							getenvstr = os.Getenv(loadenv)
							if ep.meta.DebugMode {
								log.Info("load config from ENV", log.Dict{"env": loadenv, "value": getenvstr})
							}
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

//GetEnvPrefix 获取实际的EnvPrefix
func (ep *EndPoint[T]) GetEnvPrefix() string {
	var EnvPrefix string
	if ep.meta.EnvPrefix != "" {
		EnvPrefix = ep.meta.EnvPrefix
	} else {
		EnvPrefix = strings.ToUpper(strings.Join(GetNodeProgList(ep), "_"))
	}
	return EnvPrefix
}

//VerifyConfig 验证config是否符合要求
func (ep *EndPoint[T]) VerifyConfig() bool {
	if ep.meta.NotVerifySchema {
		log.Warn("参数未校验")
		return true
	}
	configLoader := gojsonschema.NewGoLoader(ep.config)
	schemaLoader := gojsonschema.NewBytesLoader(ep.Schema())
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

// //RegistConfig 将对象注册进节点
// //如果创建时没有设置,那么可以用这个方法设置,
// func (ep *EntryPoint[T]) RegistConfig(config T) {
// 	if ep.config == nil {
// 		ep.config = config
// 	} else {
// 		log.Warn("节点的config已经被设置过了.")
// 	}
// }

// //RegistSubNode 将一个节点注册为当前节点的子节点
// //@Params child EntryPointInterface节点对象,注意必须传入的是指针
// func (ep *EntryPoint[T]) RegistSubNode(child EntryPointInterface) {
// 	ep.meta.SetChild(child)
// }
