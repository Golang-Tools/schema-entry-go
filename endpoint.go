package schemaentry

import (
	"fmt"
	"io"
	"net/url"
	"os"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"unicode"

	log "github.com/Golang-Tools/loggerhelper/v2"
	"github.com/Golang-Tools/optparams"
	"github.com/akamensky/argparse"
	"github.com/docker/docker/pkg/filenotify"
	"github.com/invopop/jsonschema"
	"github.com/xeipuuv/gojsonschema"
	"gopkg.in/yaml.v2"
)

type SupportedSerialization int8

const (
	SerializationJSON SupportedSerialization = iota
	SerializationYAML
)

//EntryPoint 节点类
//@generics T EndPointConfigInterface 内部`config`字段的类型
type EndPoint[T EndPointConfigInterface] struct {
	meta *EntryPointMeta

	config T
	locker *sync.RWMutex

	watchpath      string
	beforeRefresh  func([]byte, SupportedSerialization) bool //刷新前执行,返回false则不会进行刷新
	onRefresh      func(T)                                   //刷新后执行的回调
	onRefreshError func(error)                               //刷新失败后执行的回调
}

//NewEndPoint创建一个节点对象
//@generics T EndPointConfigInterface EntryPoint泛型的实例化参数
//@params meta *EntryPointMeta 为节点的元信息
func NewEndPoint[T EndPointConfigInterface](config T, opts ...optparams.Option[EntryPointMeta]) (*EndPoint[T], error) {
	// config := new(T)
	ep := new(EndPoint[T])
	m := optparams.GetOption(new(EntryPointMeta), opts...)
	if m.Name == "" {
		v := reflect.ValueOf(config)
		namel := strings.Split(v.Type().String(), ".")
		m.Name = strings.ToLower(namel[len(namel)-1])
	}
	ep.meta = m
	ep.config = config
	ep.locker = &sync.RWMutex{}
	return ep, nil
}

func (ep *EndPoint[T]) Schema() []byte {
	s := jsonschema.Reflect(ep.config)
	schemabytes, err := s.MarshalJSON()
	if err != nil {
		logger.Warn("config结构无法映射为jsonschema", log.Dict{"err": err.Error()})
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

//BeforeRefresh  注册刷新前执行
//@generics T EndPointConfigInterface 内部`config`字段的类型
//@params callback func([]byte, SupportedSerialization) bool 刷新前执行的函数,返回false则不会进行刷新
func (ep *EndPoint[T]) BeforeRefresh(callback func([]byte, SupportedSerialization) bool) error {
	if ep.beforeRefresh != nil {
		return ErrReregistCallBack
	}
	ep.beforeRefresh = callback
	return nil
}

//BeforeRefresh  注册刷新前执行
//@generics T EndPointConfigInterface 内部`config`字段的类型
//@params callback func(T) bool 注册刷新后执行的操作
func (ep *EndPoint[T]) OnRefresh(callback func(T)) error {
	if ep.onRefresh != nil {
		return ErrReregistCallBack
	}
	ep.onRefresh = callback
	return nil
}

//OnRefreshError 注册刷新失败后执行的回调
//@generics T EndPointConfigInterface 内部`config`字段的类型
//@params callback func(T) bool 注册刷新失败后执行的回调
func (ep *EndPoint[T]) OnRefreshError(callback func(error)) error {
	if ep.onRefreshError != nil {
		return ErrReregistCallBack
	}
	ep.onRefreshError = callback
	return nil
}

//Parse 解析节点并加载配置,配置加载顺序为
func (ep *EndPoint[T]) Parse(argv []string) {
	if ep.meta.WatchMode {
		if ep.onRefresh == nil {
			logger.Error("watchmode need to set OnRefresh first")
			os.Exit(1)
		}
	}
	prog := GetNodeProg(ep)
	parser := argparse.NewParser(prog, ep.meta.Description)
	ok := ep.passArgs(parser, argv)
	if ok {
		if ep.meta.WatchMode {
			stop, err := ep.startConfigfileWatch()
			if err != nil {
				logger.Warn("start watchmode get error,roll back to nowatchmode", log.Dict{"err": err.Error()})
			} else {
				logger.Info("watchmode is setted", log.Dict{"watch_file": ep.watchpath})
				defer stop()
			}

		}
		ep.config.Main()
	}

}

//passArgs 解析叶子节点获取启动时的配置
//@generics T EndPointConfigInterface 内部`config`字段的类型
//@Params parser *argparse.Parser 命令行参数解析器对象
//@Params argv []string 待解析的命令行参数
func (ep *EndPoint[T]) passArgs(parser *argparse.Parser, argv []string) bool {
	ep.locker.Lock()
	defer ep.locker.Unlock()
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
		return true
	}
	//默认配置文件
	err := ep.getConfigFromConfigFile()
	if err != nil {
		logger.Warn("GetConfigFromConfigFile wrong", log.Dict{"err": err})
	}

	//构造命令行参数
	filepathptr, flagConfptr, err := ep.configPtrFromArgparse(parser, argv)
	if err != nil {
		logger.Error("ConfigPtrFromArgparse error", log.Dict{"err": err})
		os.Exit(1)
	}
	//指定配置文件
	filepath := *filepathptr
	if filepath != "" {
		ep.watchpath = filepath
		U, err := url.Parse(filepath)
		if err != nil {
			// 无法解析为url,当做是文件处理
			_, err := ep.loadConfigFileFromFS(filepath)
			if err != nil {
				logger.Error("load ConfigFile From FS wrong", log.Dict{"err": err, "filepath": filepath})
				os.Exit(1)
			}
		}
		switch U.Scheme {
		case "":
			{
				_, err := ep.loadConfigFileFromFS(filepath)
				if err != nil {
					logger.Error("load ConfigFile From URL wrong", log.Dict{"err": err, "filepath": filepath, "URL": filepath})
					os.Exit(1)
				}
			}

		case "file", "fs", "dockerfs":
			{
				_, err := ep.loadConfigFileFromFS(U.Path)
				if err != nil {
					logger.Error("load ConfigFile From URL wrong", log.Dict{"err": err, "filepath": U.Path, "URL": filepath})
					os.Exit(1)
				}
			}
		default:
			{
				logger.Error("Filepath schema error", log.Dict{"err": fmt.Sprintf("unsupported schema %s", U.Scheme)})
				os.Exit(1)
			}
		}

	}
	// 环境变量->命令行
	err = ep.parseStruct(flagConfptr)
	if err != nil {
		logger.Error("ParseStruct error", log.Dict{"err": err})
		os.Exit(1)
	}
	return ep.verifyConfig()
}

//getConfigFromConfigFile 从设置的或者默认配置文件中获取配置
//@generics T EndPointConfigInterface 内部`config`字段的类型
func (ep *EndPoint[T]) getConfigFromConfigFile() error {
	var conffilepath []string
	if ep.meta.DefaultConfigFilePaths == nil {
		configFileName := strings.Join(GetNodeProgList(ep), "_")
		homepath, err := os.UserHomeDir()
		if err != nil {
			logger.Debug("find home path error", log.Dict{"err": err})
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
		stop, err := ep.loadConfigFileFromFS(filename)
		if !ep.meta.LoadAllConfigFile && stop {
			if err != nil {
				return err
			}
			break
		} else {
			if err != nil {
				logger.Debug("can not load ConfigFile", log.Dict{"filename": filename, "wrong_msg": err})
			}
		}
	}
	return nil
}

//loadConfigFileFromFS 加载文件系统中的文件到配置
//@generics T EndPointConfigInterface 内部`config`字段的类型
//@params filename 文件路径
//@returns []byte 文件内容
//@returns error 错误信息
func (ep *EndPoint[T]) loadConfigFileContentFromFS(filename string) ([]byte, error) {
	info, err := os.Stat(filename)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("find file %s not exist", filename)
		}
		return nil, fmt.Errorf("find file %s error: %s", filename, err)
	}
	if info.IsDir() {
		return nil, fmt.Errorf("find %s is a dir", filename)
	}
	f, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	fd, err := io.ReadAll(f)
	if err != nil {
		return nil, err
	}

	if fd == nil {
		return nil, fmt.Errorf("find file %s 's content is nil", filename)
	}
	fds := string(fd)
	if fds == "" {
		return nil, fmt.Errorf("find file %s 's content is empty", filename)
	}
	return fd, err
}

//loadConfigFileFromFS 加载文件系统中的文件到配置
//@generics T EndPointConfigInterface 内部`config`字段的类型
//@params filename 文件路径
//@returns bool 是否有有含义的配置以结束查找
//@returns error 错误信息
func (ep *EndPoint[T]) loadConfigFileFromFS(filename string) (bool, error) {
	fd, err := ep.loadConfigFileContentFromFS(filename)
	if err != nil {
		return false, err
	}
	if strings.HasSuffix(filename, "json") {
		err := json.Unmarshal(fd, ep.config)
		if err != nil {
			return false, err
		}
	} else if strings.HasSuffix(filename, "yml") || strings.HasSuffix(filename, "yaml") {
		err := yaml.Unmarshal(fd, ep.config)
		if err != nil {
			return false, err
		}
	} else {
		return false, fmt.Errorf("%s has not supported suffix", filename)
	}
	return true, nil
}

//configPtrFromArgparse 构造命令行参数解析,并获取flag的ptr
//@generics T EndPointConfigInterface 内部`config`字段的类型
//@params parser *argparse.Parser flag解析器
//@params argv []string 待解析的命令行参数
//@returns *string 指定configfile位置字符串
//@returns map[string]interface{} flag的ptr位置
//@returns error 错误信息
func (ep *EndPoint[T]) configPtrFromArgparse(parser *argparse.Parser, argv []string) (*string, map[string]interface{}, error) {
	defer func() {
		if err := recover(); err != nil {
			logger.Error("ConfigPtrFromArgparse get error", log.Dict{"err": err})
			os.Exit(1)
		}
	}()
	r := jsonschema.Reflector{
		DoNotReference: true,
	}
	schema := r.Reflect(ep.config)

	flagConfptr := map[string]interface{}{}
	required := false
	help := "指定读取的配置文件位置"
	if ep.meta.WatchMode {
		required = true
		help = "指定监控的配置文件位置"
	}
	argconfigfilepath := parser.String("c", "config", &argparse.Options{Required: required, Help: help})
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
			logger.Warn("schema.Properties.Get(name) not ok", log.Dict{"name": name, "Properties": schema.Properties})
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

//parseStruct 解析结构体,构造命令行参数解析和环境变量解析,并设置到对象的Config值中
//@generics T EndPointConfigInterface 内部`config`字段的类型
//@Params flagConfptr map[string]interface{} 命令行参数除了指定的配置文件位置外的参数->值的指针的映射
//@Returns error 解析过程中的错误
func (ep *EndPoint[T]) parseStruct(flagConfptr map[string]interface{}) error {
	r := jsonschema.Reflector{
		DoNotReference: true,
	}
	schema := r.Reflect(ep.config)
	EnvPrefix := ep.getEnvPrefix()
	//设置参数
	t := reflect.TypeOf(ep.config)
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	v := reflect.ValueOf(ep.config).Elem()
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
		var defau interface{}
		if ok {
			fieldschema := fieldschema_i.(*jsonschema.Schema)
			if fieldschema.Default != nil {
				defau = fieldschema.Default
			}
		}

		switch f.Type.Kind() {
		case reflect.String:
			{
				if defau != nil {
					vf.Set(reflect.ValueOf(defau))
				}
				//设置环境变量配置
				getenvstr := ""
				if !ep.meta.NotParseEnv {
					loadenv := fmt.Sprintf("%s_%s", EnvPrefix, strings.ToUpper(f.Name))
					getenvstr = os.Getenv(loadenv)
					logger.Debug("load config from ENV", log.Dict{"env": loadenv, "value": getenvstr})
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
				if defau != nil {
					vf.Set(reflect.ValueOf(defau))
				}
				//设置环境变量配置
				getenvstr := ""
				if !ep.meta.NotParseEnv {
					loadenv := fmt.Sprintf("%s_%s", EnvPrefix, strings.ToUpper(f.Name))
					getenvstr = os.Getenv(loadenv)
					logger.Debug("load config from ENV", log.Dict{"env": loadenv, "value": getenvstr})
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
				if defau != nil {
					vf.Set(reflect.ValueOf(defau))
				}
				//设置环境变量配置
				getenvstr := ""
				if !ep.meta.NotParseEnv {
					loadenv := fmt.Sprintf("%s_%s", EnvPrefix, strings.ToUpper(f.Name))
					getenvstr = os.Getenv(loadenv)
					logger.Debug("load config from ENV", log.Dict{"env": loadenv, "value": getenvstr})
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
				if defau != nil {
					vf.Set(reflect.ValueOf(defau))
				}
				//设置环境变量配置
				getenvstr := ""
				if !ep.meta.NotParseEnv {
					loadenv := fmt.Sprintf("%s_%s", EnvPrefix, strings.ToUpper(f.Name))
					getenvstr = os.Getenv(loadenv)
					logger.Debug("load config from ENV", log.Dict{"env": loadenv, "value": getenvstr})
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
						if defau != nil {
							defa_i := defau.([]interface{})
							defa_r := []string{}
							for _, v := range defa_i {
								defa_r = append(defa_r, v.(string))
							}
							vf.Set(reflect.ValueOf(defa_r))
						}
						//设置环境变量配置
						getenvstr := ""
						if !ep.meta.NotParseEnv {
							loadenv := fmt.Sprintf("%s_%s", EnvPrefix, strings.ToUpper(f.Name))
							getenvstr = os.Getenv(loadenv)
							logger.Debug("load config from ENV", log.Dict{"env": loadenv, "value": getenvstr})
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
						if defau != nil {
							defa_i := defau.([]interface{})
							defa_r := []int{}
							for _, v := range defa_i {
								value_str := v.(string)
								value, err := strconv.Atoi(value_str)
								if err != nil {
									break
								}
								defa_r = append(defa_r, value)
							}
							vf.Set(reflect.ValueOf(defa_r))
						}
						//设置环境变量配置
						getenvstr := ""
						if !ep.meta.NotParseEnv {
							loadenv := fmt.Sprintf("%s_%s", EnvPrefix, strings.ToUpper(f.Name))
							getenvstr = os.Getenv(loadenv)
							logger.Debug("load config from ENV", log.Dict{"env": loadenv, "value": getenvstr})
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
						if defau != nil {
							defa_i := defau.([]interface{})
							defa_r := []float64{}
							for _, v := range defa_i {
								value_str := v.(string)
								value, err := strconv.ParseFloat(value_str, 64)
								if err != nil {
									break
								}
								defa_r = append(defa_r, value)
							}
							vf.Set(reflect.ValueOf(defa_r))
						}
						//设置环境变量配置
						getenvstr := ""
						if !ep.meta.NotParseEnv {
							loadenv := fmt.Sprintf("%s_%s", EnvPrefix, strings.ToUpper(f.Name))
							getenvstr = os.Getenv(loadenv)
							logger.Debug("load config from ENV", log.Dict{"env": loadenv, "value": getenvstr})
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

//getEnvPrefix 获取实际的EnvPrefix
//@generics T EndPointConfigInterface 内部`config`字段的类型
func (ep *EndPoint[T]) getEnvPrefix() string {
	var EnvPrefix string
	if ep.meta.EnvPrefix != "" {
		EnvPrefix = ep.meta.EnvPrefix
	} else {
		EnvPrefix = strings.ToUpper(strings.Join(GetNodeProgList(ep), "_"))
	}
	return EnvPrefix
}

//VerifyConfig 验证config是否符合要求
//@generics T EndPointConfigInterface 内部`config`字段的类型
func (ep *EndPoint[T]) verifyConfig() bool {
	if ep.meta.NotVerifySchema {
		logger.Warn("参数未校验")
		return true
	}
	configLoader := gojsonschema.NewGoLoader(ep.config)
	schemaLoader := gojsonschema.NewBytesLoader(ep.Schema())
	result, err := gojsonschema.Validate(schemaLoader, configLoader)
	if err != nil {
		logger.Error("模式校验执行错误", log.Dict{"err": err})
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
	logger.Error("模式校验错误", errsS)
	return false
}

type StopWatchFunc func()

//fsWatchHandler 监听文件系统执行操作
//@generics T EndPointConfigInterface 内部`config`字段的类型
//@params filepath string 文件路径
//@params watcher filenotify.FileWatcher 文件系统监听器
func (ep *EndPoint[T]) fsWatchHandler(filepath string, watcher filenotify.FileWatcher) {

	logger.Debug("FSWatchHandler start")

	defer func() {
		logger.Debug("FSWatchHandler end")
		if r := recover(); r != nil {
			logger.Error("FSWatchHandler get error", log.Dict{"r": r})
		}
	}()
	for {
		select {
		case event, ok := <-watcher.Events():
			{
				logger.Debug("FSWatchHandler get event", log.Dict{"event": event.String(), "ok": ok})
				if !ok {
					return
				}
				if strings.Contains(event.Op.String(), "WRITE") {
					logger.Debug("FSWatchHandler active")
					skip := ep.refreshFSProcess(filepath)
					if skip {
						logger.Debug("FSWatchHandler skip update", log.Dict{"event": event})
					}
				} else {
					logger.Debug("FSWatchHandler not active", log.Dict{"event": event.String()})
				}
			}
		case err, ok := <-watcher.Errors():
			if !ok {
				return
			}
			logger.Warn("FSWatchHandler watcher get error", log.Dict{"err": err.Error(), "ok": ok})
		}
	}
}

//refreshFSProcess 文件系统刷新配置流程
//@params filepath string 配置文件路径
//@returns bool 是否跳过更新
func (ep *EndPoint[T]) refreshFSProcess(filepath string) bool {
	var serialize_protocol SupportedSerialization
	if strings.HasSuffix(filepath, "json") {
		serialize_protocol = SerializationJSON
	} else if strings.HasSuffix(filepath, "yml") || strings.HasSuffix(filepath, "yaml") {
		serialize_protocol = SerializationYAML
	} else {
		if ep.onRefreshError != nil {
			ep.onRefreshError(ErrUnsupportedSerialization)
		} else {
			logger.Error("RefreshFSProcess get error", log.Dict{"step": "fix serialize_protocol", "error": fmt.Sprintf("unsupported serialization protocol %s", filepath)})
		}
		return false
	}

	fd, err := ep.loadConfigFileContentFromFS(filepath)
	if err != nil {
		if ep.onRefreshError != nil {
			ep.onRefreshError(err)
		} else {
			logger.Error("RefreshFSProcess get error", log.Dict{"step": "load config file content", "error": err.Error()})
		}
		return false
	}
	refresh := true
	if ep.beforeRefresh != nil {
		refresh = ep.beforeRefresh(fd, serialize_protocol)
	}
	if refresh {
		ep.locker.Lock()
		defer ep.locker.Unlock()
		switch serialize_protocol {
		case SerializationJSON:
			{
				err := json.Unmarshal(fd, ep.config)
				if err != nil {
					if ep.onRefreshError != nil {
						ep.onRefreshError(err)
					} else {
						logger.Error("RefreshFSProcess get error", log.Dict{"step": "Unmarshal JSON", "error": err.Error()})
					}
					return false
				}
			}
		case SerializationYAML:
			{
				err := yaml.Unmarshal(fd, ep.config)
				if err != nil {
					if ep.onRefreshError != nil {
						ep.onRefreshError(err)
					} else {
						logger.Error("RefreshFSProcess get error", log.Dict{"step": "Unmarshal YAML", "error": err.Error()})
					}
					return false
				}
			}
		default:
			{
				if ep.onRefreshError != nil {
					ep.onRefreshError(ErrUnsupportedSerialization)
				} else {
					logger.Error("RefreshFSProcess get error", log.Dict{"step": "Unmarshal Unsupported Protocol", "error": fmt.Sprintf("unsupported serialization protocol %s", filepath)})
				}
				return false
			}
		}
		ep.onRefresh(ep.config)
		return false
		// err := json.Unmarshal(fd, ep.config)
		// if err != nil {
		// 	return false, err
		// }
	} else {
		return true
	}

}

//startConfigfileWatch 开始监听配置文件
//@generics T EndPointConfigInterface 内部`config`字段的类型
//@returns StopWatchFunc 停止监听函数
func (ep *EndPoint[T]) startConfigfileWatch() (StopWatchFunc, error) {
	U, err := url.Parse(ep.watchpath)

	if err != nil {
		path := ep.watchpath
		watcher, err := filenotify.NewEventWatcher()
		if err != nil {
			logger.Error("new fswatcher get error", log.Dict{"err": err.Error()})
			return nil, err
		}
		go ep.fsWatchHandler(path, watcher)
		watcher.Add(path)
		return func() { watcher.Close() }, nil
	}
	switch U.Scheme {
	case "":
		{
			path := ep.watchpath
			watcher, err := filenotify.NewEventWatcher()
			if err != nil {
				logger.Error("new fswatcher get error", log.Dict{"err": err.Error()})
				return nil, err
			}
			go ep.fsWatchHandler(path, watcher)
			watcher.Add(path)
			return func() { watcher.Close() }, nil
		}
	case "file", "fs":
		{
			path := U.Path
			watcher, err := filenotify.NewEventWatcher()
			if err != nil {
				logger.Error("new fswatcher get error", log.Dict{"err": err.Error()})
				return nil, err
			}
			go ep.fsWatchHandler(path, watcher)
			watcher.Add(path)
			return func() { watcher.Close() }, nil
		}
	case "dockerfs":
		{
			path := U.Path
			watcher := filenotify.NewPollingWatcher()
			if err != nil {
				logger.Error("new fswatcher get error", log.Dict{"err": err.Error()})
				return nil, err
			}
			go ep.fsWatchHandler(path, watcher)
			watcher.Add(path)
			return func() { watcher.Close() }, nil
		}
	default:
		{
			logger.Error("Filepath schema error", log.Dict{"err": fmt.Sprintf("unsupported schema %s", U.Scheme)})
			return nil, ErrUnsupportedSchema
		}
	}
}
