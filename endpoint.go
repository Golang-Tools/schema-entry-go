package schemaentry

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"os"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	log "github.com/Golang-Tools/loggerhelper/v2"
	"github.com/Golang-Tools/optparams"
	"github.com/akamensky/argparse"
	"github.com/docker/docker/pkg/filenotify"
	"github.com/invopop/jsonschema"
	"github.com/xeipuuv/gojsonschema"
	"go.etcd.io/etcd/api/v3/mvccpb"
	clientv3 "go.etcd.io/etcd/client/v3"
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
			serialize, path, err := ParseFSPath(filepath)
			if err != nil {
				logger.Error("Parse URL wrong", log.Dict{"err": err.Error(), "filepath": path, "URL": filepath})
				os.Exit(1)
			}
			_, err = ep.loadConfigFileFromFS(serialize, path)
			if err != nil {
				logger.Error("load ConfigFile From FS wrong", log.Dict{"err": err, "filepath": filepath})
				os.Exit(1)
			}
		}
		switch U.Scheme {
		case "":
			{
				serialize, path, err := ParseFSUrl(U)
				if err != nil {
					logger.Error("Parse URL wrong", log.Dict{"err": err.Error(), "filepath": path, "URL": filepath})
					os.Exit(1)
				}
				_, err = ep.loadConfigFileFromFS(serialize, path)
				if err != nil {
					logger.Error("load ConfigFile From URL wrong", log.Dict{"err": err, "filepath": filepath, "URL": filepath})
					os.Exit(1)
				}
			}

		case "file", "fs", "dockerfs":
			{
				serialize, path, err := ParseFSUrl(U)
				if err != nil {
					logger.Error("Parse URL wrong", log.Dict{"err": err.Error(), "filepath": path, "URL": filepath})
					os.Exit(1)
				}
				_, err = ep.loadConfigFileFromFS(serialize, path)
				if err != nil {
					logger.Error("load ConfigFile From URL wrong", log.Dict{"err": err, "filepath": U.Path, "URL": filepath})
					os.Exit(1)
				}
			}
		case "etcd":
			{
				serialize, path, config, timeout, err := ParseEtcdUrl(U)
				if err != nil {
					logger.Error("Parse URL wrong", log.Dict{"err": err.Error(), "filepath": path, "URL": filepath})
					os.Exit(1)
				}
				_, err = ep.loadConfigFileFromEtcd(serialize, path, config, timeout)
				if err != nil {
					logger.Error("load ConfigFile From URL wrong", log.Dict{"err": err.Error(), "filepath": U.Path, "URL": filepath})
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
	for _, filepath := range conffilepath {
		serialize, path, err := ParseFSPath(filepath)
		if err != nil {
			logger.Error("Parse URL wrong", log.Dict{"err": err.Error(), "filepath": path, "URL": filepath})
			os.Exit(1)
		}
		stop, err := ep.loadConfigFileFromFS(serialize, path)
		if !ep.meta.LoadAllConfigFile && stop {
			if err != nil {
				return err
			}
			break
		} else {
			if err != nil {
				logger.Debug("can not load ConfigFile", log.Dict{"filepath": filepath, "err": err.Error()})
			}
		}
	}
	return nil
}

//loadContentAsConfig 加载文本内容到config
//@generics T EndPointConfigInterface 内部`config`字段的类型
//@params serialization SupportedSerialization 文件使用的序列化协议
//@params content ]byte 待加载内容
//@returns bool 是否有有含义的配置以结束查找
//@returns error 错误信息
func (ep *EndPoint[T]) loadContentAsConfig(serialization SupportedSerialization, content []byte) (bool, error) {
	switch serialization {
	case SerializationJSON:
		{
			err := json.Unmarshal(content, ep.config)
			if err != nil {
				return false, err
			}
		}
	case SerializationYAML:
		{
			err := yaml.Unmarshal(content, ep.config)
			if err != nil {
				return false, err
			}
		}
	default:
		{
			return false, ErrUnsupportedSerialization
		}
	}
	return true, nil
}

//loadConfigFileContentFromFS 加载文件系统中的文件到配置
//@generics T EndPointConfigInterface 内部`config`字段的类型
//@params path string 文件路径
//@returns []byte 文件内容
//@returns error 错误信息
func (ep *EndPoint[T]) loadConfigFileContentFromFS(path string) ([]byte, error) {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("find file %s not exist", path)
		}
		return nil, fmt.Errorf("find file %s error: %s", path, err)
	}
	if info.IsDir() {
		return nil, fmt.Errorf("find %s is a dir", path)
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	fd, err := io.ReadAll(f)
	if err != nil {
		return nil, err
	}

	if fd == nil {
		return nil, fmt.Errorf("find file %s 's content is nil", path)
	}
	fds := string(fd)
	if fds == "" {
		return nil, fmt.Errorf("find file %s 's content is empty", path)
	}
	return fd, err
}

//loadConfigFileFromFS 加载文件系统中的文件到配置
//@generics T EndPointConfigInterface 内部`config`字段的类型
//@params serialization SupportedSerialization 文件使用的序列化协议
//@params path string 文件路径
//@returns bool 是否有有含义的配置以结束查找
//@returns error 错误信息
func (ep *EndPoint[T]) loadConfigFileFromFS(serialization SupportedSerialization, path string) (bool, error) {
	content, err := ep.loadConfigFileContentFromFS(path)
	if err != nil {
		return false, err
	}
	return ep.loadContentAsConfig(serialization, content)
}

//loadConfigFileContentFromEtcd 加载etcd中的内容到系统
//@generics T EndPointConfigInterface 内部`config`字段的类型
//@params path string key路径
//@params config clientv3.Config etcd配置
//@params timeout time.Duration 请求超时
//@returns []byte 文件内容
//@returns error 错误信息
func (ep *EndPoint[T]) loadConfigFileContentFromEtcd(path string, config clientv3.Config, timeout time.Duration) ([]byte, error) {
	cli, err := clientv3.New(clientv3.Config{Endpoints: []string{"localhost:2379"}})
	if err != nil {
		return nil, err
	}
	defer cli.Close()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	resp, err := cli.Get(ctx, path, clientv3.WithLastRev()...)
	if err != nil {
		return nil, err
	}
	for _, ev := range resp.Kvs {
		logger.Warn("get etcd kv result", log.Dict{"key": ev.Key, "value": ev.Value})
	}
	if len(resp.Kvs) != 1 {
		return nil, ErrEtcdKeyLenNotMatch
	}
	return resp.Kvs[0].Value, nil
}

//loadConfigFileFromFS 加载文件系统中的文件到配置
//@generics T EndPointConfigInterface 内部`config`字段的类型
//@params filename 文件路径
//@returns bool 是否有有含义的配置以结束查找
//@returns error 错误信息
func (ep *EndPoint[T]) loadConfigFileFromEtcd(serialization SupportedSerialization, path string, config clientv3.Config, timeout time.Duration) (bool, error) {
	content, err := ep.loadConfigFileContentFromEtcd(path, config, timeout)
	if err != nil {
		return false, err
	}
	return ep.loadContentAsConfig(serialization, content)
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

//startConfigfileWatch 开始监听配置文件
//@generics T EndPointConfigInterface 内部`config`字段的类型
//@returns StopWatchFunc 停止监听函数
//@returns error 程序错误
func (ep *EndPoint[T]) startConfigfileWatch() (StopWatchFunc, error) {
	U, err := url.Parse(ep.watchpath)

	if err != nil {
		serialize, path, err := ParseFSPath(ep.watchpath)
		if err != nil {
			logger.Error("Parse URL wrong", log.Dict{"err": err.Error(), "filepath": path})
			return nil, err
		}
		return ep.GenFSWatcher(serialize, path, false)
	}
	switch U.Scheme {
	case "":
		{
			serialize, path, err := ParseFSUrl(U)
			if err != nil {
				logger.Error("Parse URL wrong", log.Dict{"err": err.Error(), "URL": ep.watchpath})
				return nil, err
			}
			return ep.GenFSWatcher(serialize, path, false)
		}
	case "file", "fs":
		{
			serialize, path, err := ParseFSUrl(U)
			if err != nil {
				logger.Error("Parse URL wrong", log.Dict{"err": err.Error(), "URL": ep.watchpath})
				return nil, err
			}
			return ep.GenFSWatcher(serialize, path, false)
		}
	case "dockerfs":
		{
			serialize, path, err := ParseFSUrl(U)
			if err != nil {
				logger.Error("Parse URL wrong", log.Dict{"err": err.Error(), "URL": ep.watchpath})
				return nil, err
			}
			return ep.GenFSWatcher(serialize, path, true)
		}
	case "etcd":
		{
			//TODO
			serialize, path, config, _, err := ParseEtcdUrl(U)
			if err != nil {
				logger.Error("Parse URL wrong", log.Dict{"err": err.Error(), "URL": ep.watchpath})
				return nil, err
			}
			return ep.GenEtcdWatcher(serialize, path, config)
		}
	default:
		{
			logger.Error("Filepath schema error", log.Dict{"err": fmt.Sprintf("unsupported schema %s", U.Scheme)})
			return nil, ErrUnsupportedSchema
		}
	}
}

//GenFSWatcher 生成文件系统监听器
//@generics T EndPointConfigInterface 内部`config`字段的类型
//@params serialize_protocol SupportedSerialization 使用的序列化协议
//@params filepath string 文件路径
//@params indocker bool 文件系统是否在docker中
//@returns StopWatchFunc 停止监听函数
//@returns error 程序错误
func (ep *EndPoint[T]) GenFSWatcher(serialize_protocol SupportedSerialization, filepath string, indocker bool) (StopWatchFunc, error) {
	var watcher filenotify.FileWatcher
	var err error
	if indocker {
		watcher = filenotify.NewPollingWatcher()
	} else {
		watcher, err = filenotify.NewEventWatcher()
		if err != nil {
			return nil, err
		}
	}
	go ep.fsWatchHandler(serialize_protocol, filepath, watcher)
	watcher.Add(filepath)
	return func() { watcher.Close() }, nil
}

//fsWatchHandler 监听文件系统执行操作
//@generics T EndPointConfigInterface 内部`config`字段的类型
//@params serialize_protocol SupportedSerialization 使用的序列化协议
//@params filepath string 文件路径
//@params watcher filenotify.FileWatcher 文件系统监听器
func (ep *EndPoint[T]) fsWatchHandler(serialize_protocol SupportedSerialization, filepath string, watcher filenotify.FileWatcher) {
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
					skip := ep.refreshFSProcess(serialize_protocol, filepath)
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

//refreshContentProcess 根据内容刷新配置
//@params serialize_protocol SupportedSerialization 使用的序列化协议
//@params content []byte 带序列化的内容
func (ep *EndPoint[T]) refreshContentProcess(serialize_protocol SupportedSerialization, content []byte) bool {
	refresh := true
	if ep.beforeRefresh != nil {
		refresh = ep.beforeRefresh(content, serialize_protocol)
	}
	if refresh {
		ep.locker.Lock()
		defer ep.locker.Unlock()
		switch serialize_protocol {
		case SerializationJSON:
			{
				err := json.Unmarshal(content, ep.config)
				if err != nil {
					if ep.onRefreshError != nil {
						ep.onRefreshError(err)
					} else {
						logger.Error("RefreshContentProcess get error", log.Dict{"step": "Unmarshal JSON", "error": err.Error()})
					}
					return false
				}
			}
		case SerializationYAML:
			{
				err := yaml.Unmarshal(content, ep.config)
				if err != nil {
					if ep.onRefreshError != nil {
						ep.onRefreshError(err)
					} else {
						logger.Error("RefreshContentProcess get error", log.Dict{"step": "Unmarshal YAML", "error": err.Error()})
					}
					return false
				}
			}
		default:
			{
				if ep.onRefreshError != nil {
					ep.onRefreshError(ErrUnsupportedSerialization)
				} else {
					logger.Error("RefreshContentProcess get error", log.Dict{"step": "Unmarshal Unsupported Protocol", "error": fmt.Sprintf("unsupported serialization protocol %d", serialize_protocol)})
				}
				return false
			}
		}
		ep.onRefresh(ep.config)
		return false
	} else {
		return true
	}
}

//refreshFSProcess 文件系统刷新配置流程
//@params serialize_protocol SupportedSerialization 文件使用的序列化协议
//@params filepath string 配置文件路径
//@returns bool 是否跳过更新
func (ep *EndPoint[T]) refreshFSProcess(serialize_protocol SupportedSerialization, filepath string) bool {

	content, err := ep.loadConfigFileContentFromFS(filepath)
	if err != nil {
		if ep.onRefreshError != nil {
			ep.onRefreshError(err)
		} else {
			logger.Error("RefreshFSProcess get error", log.Dict{"step": "load config file content", "error": err.Error()})
		}
		return false
	}
	return ep.refreshContentProcess(serialize_protocol, content)
}

//GenEtcdWatcher 生成文件系统监听器
//@generics T EndPointConfigInterface 内部`config`字段的类型
//@params serialize_protocol SupportedSerialization 文件使用的序列化协议
//@params filepath string 文件路径
//@params config clientv3.Config etcd配置
//@returns StopWatchFunc 停止监听函数
//@returns error 程序错误
func (ep *EndPoint[T]) GenEtcdWatcher(serialize_protocol SupportedSerialization, filepath string, config clientv3.Config) (StopWatchFunc, error) {
	cli, err := clientv3.New(clientv3.Config{Endpoints: []string{"localhost:2379"}})
	if err != nil {
		return nil, err
	}
	go ep.etcdWatchHandler(serialize_protocol, filepath, cli)
	return func() { cli.Close() }, nil
}

//etcdWatchHandler 监听etcd系统执行操作
//@generics T EndPointConfigInterface 内部`config`字段的类型
//@params serialize_protocol SupportedSerialization 使用的序列化协议
//@params filepath string key路径
//@params cli *clientv3.Client etcd连接
func (ep *EndPoint[T]) etcdWatchHandler(serialize_protocol SupportedSerialization, filepath string, cli *clientv3.Client) {

	logger.Debug("EtcdWatchHandler start")
	defer func() {
		logger.Debug("EtcdWatchHandler end")
		if r := recover(); r != nil {
			logger.Error("EtcdWatchHandler get error", log.Dict{"r": r})
		}
	}()
	rch := cli.Watch(context.Background(), filepath)
	for wresp := range rch {
		for _, ev := range wresp.Events {
			fmt.Printf("%s %q : %q\n", ev.Type, ev.Kv.Key, ev.Kv.Value)
			logger.Debug("EtcdWatchHandler get event", log.Dict{"event": ev.Type.String(), "key": string(ev.Kv.Key)})
			if ev.Type == mvccpb.PUT && string(ev.Kv.Key) == filepath {
				logger.Debug("FSWatchHandler active")
				skip := ep.refreshContentProcess(serialize_protocol, ev.Kv.Value)
				if skip {
					logger.Debug("FSWatchHandler skip update", log.Dict{"event": ev.Type.String(), "key": string(ev.Kv.Key)})
				}
			} else {
				logger.Debug("FSWatchHandler not active", log.Dict{"event": ev.Type.String(), "key": string(ev.Kv.Key)})
			}
		}
	}
}
