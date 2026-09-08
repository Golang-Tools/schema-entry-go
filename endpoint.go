package schemaentry

import (
	"context"
	"encoding/json"
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

	log "github.com/Golang-Tools/loggerhelper/v4"
	"github.com/Golang-Tools/optparams"
	"github.com/docker/docker/pkg/filenotify"
	"github.com/invopop/jsonschema"
	"github.com/spf13/pflag"
	"github.com/xeipuuv/gojsonschema"
	"go.etcd.io/etcd/api/v3/mvccpb"
	clientv3 "go.etcd.io/etcd/client/v3"
	"gopkg.in/yaml.v3"
)

type SupportedSerialization int8

const (
	SerializationJSON SupportedSerialization = iota
	SerializationYAML
)

// EntryPoint 节点类
// @generics T EndPointConfigInterface 内部`config`字段的类型
type EndPoint[T EndPointConfigInterface] struct {
	meta *EntryPointMeta

	config T
	locker *sync.RWMutex

	watchpath      string
	beforeRefresh  func([]byte, SupportedSerialization) bool //刷新前执行,返回false则不会进行刷新
	onRefresh      func(T)                                   //刷新后执行的回调
	onRefreshError func(error)                               //刷新失败后执行的回调
}

// NewEndPoint创建一个节点对象
// @generics T EndPointConfigInterface EntryPoint泛型的实例化参数
// @params meta *EntryPointMeta 为节点的元信息
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

// BeforeRefresh  注册刷新前执行
// @generics T EndPointConfigInterface 内部`config`字段的类型
// @params callback func([]byte, SupportedSerialization) bool 刷新前执行的函数,返回false则不会进行刷新
func (ep *EndPoint[T]) BeforeRefresh(callback func([]byte, SupportedSerialization) bool) error {
	if ep.beforeRefresh != nil {
		return ErrReregistCallBack
	}
	ep.beforeRefresh = callback
	return nil
}

// BeforeRefresh  注册刷新前执行
// @generics T EndPointConfigInterface 内部`config`字段的类型
// @params callback func(T) bool 注册刷新后执行的操作
func (ep *EndPoint[T]) OnRefresh(callback func(T)) error {
	if ep.onRefresh != nil {
		return ErrReregistCallBack
	}
	ep.onRefresh = callback
	return nil
}

// OnRefreshError 注册刷新失败后执行的回调
// @generics T EndPointConfigInterface 内部`config`字段的类型
// @params callback func(T) bool 注册刷新失败后执行的回调
func (ep *EndPoint[T]) OnRefreshError(callback func(error)) error {
	if ep.onRefreshError != nil {
		return ErrReregistCallBack
	}
	ep.onRefreshError = callback
	return nil
}

// Parse 解析节点并加载配置,成功解析后执行 config.Main(命令执行模型)
// 解析/加载/校验失败会返回 error;请求帮助(-h/--help)返回包装了 ErrHelp 的 UsageError
func (ep *EndPoint[T]) Parse(argv []string) error {
	if ep.meta.WatchMode && ep.onRefresh == nil {
		return ErrWatchOnRefreshNotSet
	}
	if err := ep.passArgs(argv); err != nil {
		return err
	}
	if ep.meta.WatchMode {
		stop, err := ep.startConfigfileWatch()
		if err != nil {
			return fmt.Errorf("start config file watch: %w", err)
		}
		logger.Info("watchmode is setted", log.Dict{"watch_file": ep.watchpath})
		defer stop()
	}
	ep.config.Main()
	return nil
}

// passArgs 解析叶子节点获取启动时的配置
// @generics T EndPointConfigInterface 内部`config`字段的类型
// @Params argv []string 待解析的命令行参数(argv[0] 为命令名)
// @Returns error 解析过程中的错误;请求帮助返回包装 ErrHelp 的 UsageError
func (ep *EndPoint[T]) passArgs(argv []string) error {
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
		return nil
	}
	//先应用 jsonschema 默认值(基础值,优先级最低)
	if err := ep.applyConfigDefaults(); err != nil {
		return err
	}
	//默认配置文件(非致命,缺失时仅记录并继续)
	if err := ep.getConfigFromConfigFile(); err != nil {
		logger.Warn("GetConfigFromConfigFile wrong", log.Dict{"err": err})
	}
	//构造命令行参数并解析
	fs, err := ep.buildConfigFlagSet()
	if err != nil {
		return err
	}
	if len(argv) > 0 {
		argv = argv[1:] // 跳过命令名
	}
	if err := fs.Parse(argv); err != nil {
		return &UsageError{Usage: ep.usageString(fs), Err: err}
	}
	if fs.NArg() > 0 {
		return &UsageError{Usage: ep.usageString(fs), Err: fmt.Errorf("未知参数: %v", fs.Args())}
	}
	//-h/--help 请求帮助
	if help, _ := fs.GetBool("help"); help {
		return &UsageError{Usage: ep.usageString(fs), Err: ErrHelp}
	}
	//watchmode 下必须显式提供配置文件
	filepath, _ := fs.GetString("config")
	if ep.meta.WatchMode && filepath == "" {
		return &UsageError{Usage: ep.usageString(fs), Err: fmt.Errorf("watchmode 下必须用 -c/--config 指定配置文件")}
	}
	//加载命令行指定的配置文件
	if filepath != "" {
		if err := ep.loadConfigFileByPath(filepath); err != nil {
			return err
		}
	}
	// 环境变量->命令行
	if err := ep.applyFlagsToConfig(fs); err != nil {
		return err
	}
	return ep.verifyConfig()
}

// loadConfigFileByPath 加载 -c/--config 指定的配置文件(本地路径或 url/etcd)
// @generics T EndPointConfigInterface 内部`config`字段的类型
// @Params filepath string -c/--config 传入的路径
// @Returns error 错误信息
func (ep *EndPoint[T]) loadConfigFileByPath(filepath string) error {
	ep.watchpath = filepath
	U, err := url.Parse(filepath)
	if err != nil {
		// 无法解析为url,当做是文件处理
		serialize, path, err := ParseFSPath(filepath)
		if err != nil {
			logger.Error("Parse URL wrong", log.Dict{"err": err.Error(), "filepath": path, "URL": filepath})
			return err
		}
		if _, err := ep.loadConfigFileFromFS(serialize, path); err != nil {
			logger.Error("load ConfigFile From FS wrong", log.Dict{"err": err, "filepath": filepath})
			return err
		}
		return nil
	}
	switch U.Scheme {
	case "", "file", "fs", "dockerfs":
		serialize, path, err := ParseFSUrl(U)
		if err != nil {
			logger.Error("Parse URL wrong", log.Dict{"err": err.Error(), "filepath": path, "URL": filepath})
			return err
		}
		if _, err := ep.loadConfigFileFromFS(serialize, path); err != nil {
			logger.Error("load ConfigFile From URL wrong", log.Dict{"err": err, "filepath": path, "URL": filepath})
			return err
		}
	case "etcd":
		serialize, path, config, timeout, err := ParseEtcdUrl(U)
		if err != nil {
			logger.Error("Parse URL wrong", log.Dict{"err": err.Error(), "filepath": path, "URL": filepath})
			return err
		}
		if _, err := ep.loadConfigFileFromEtcd(serialize, path, config, timeout); err != nil {
			logger.Error("load ConfigFile From URL wrong", log.Dict{"err": err.Error(), "filepath": path, "URL": filepath})
			return err
		}
	default:
		logger.Error("Filepath schema error", log.Dict{"err": fmt.Sprintf("unsupported schema %s", U.Scheme)})
		return ErrUnsupportedSchema
	}
	return nil
}

// getConfigFromConfigFile 从设置的或者默认配置文件中获取配置
// @generics T EndPointConfigInterface 内部`config`字段的类型
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
			return err
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

// loadContentAsConfig 加载文本内容到config
// @generics T EndPointConfigInterface 内部`config`字段的类型
// @params serialization SupportedSerialization 文件使用的序列化协议
// @params content ]byte 待加载内容
// @returns bool 是否有有含义的配置以结束查找
// @returns error 错误信息
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

// loadConfigFileContentFromFS 加载文件系统中的文件到配置
// @generics T EndPointConfigInterface 内部`config`字段的类型
// @params path string 文件路径
// @returns []byte 文件内容
// @returns error 错误信息
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

// loadConfigFileFromFS 加载文件系统中的文件到配置
// @generics T EndPointConfigInterface 内部`config`字段的类型
// @params serialization SupportedSerialization 文件使用的序列化协议
// @params path string 文件路径
// @returns bool 是否有有含义的配置以结束查找
// @returns error 错误信息
func (ep *EndPoint[T]) loadConfigFileFromFS(serialization SupportedSerialization, path string) (bool, error) {
	content, err := ep.loadConfigFileContentFromFS(path)
	if err != nil {
		return false, err
	}
	return ep.loadContentAsConfig(serialization, content)
}

// loadConfigFileContentFromEtcd 加载etcd中的内容到系统
// @generics T EndPointConfigInterface 内部`config`字段的类型
// @params path string key路径
// @params config clientv3.Config etcd配置
// @params timeout time.Duration 请求超时
// @returns []byte 文件内容
// @returns error 错误信息
func (ep *EndPoint[T]) loadConfigFileContentFromEtcd(path string, config clientv3.Config, timeout time.Duration) ([]byte, error) {
	cli, err := clientv3.New(config)
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

// loadConfigFileFromFS 加载文件系统中的文件到配置
// @generics T EndPointConfigInterface 内部`config`字段的类型
// @params filename 文件路径
// @returns bool 是否有有含义的配置以结束查找
// @returns error 错误信息
func (ep *EndPoint[T]) loadConfigFileFromEtcd(serialization SupportedSerialization, path string, config clientv3.Config, timeout time.Duration) (bool, error) {
	content, err := ep.loadConfigFileContentFromEtcd(path, config, timeout)
	if err != nil {
		return false, err
	}
	return ep.loadContentAsConfig(serialization, content)
}

// buildConfigFlagSet 依据 config 结构体与其 jsonschema 信息构建命令行 flag 集合
// @generics T EndPointConfigInterface 内部`config`字段的类型
// @returns *pflag.FlagSet 构建出的 flag 集合
// @returns error 构建过程中的错误
func (ep *EndPoint[T]) buildConfigFlagSet() (*pflag.FlagSet, error) {
	fs := pflag.NewFlagSet(GetNodeProg(ep), pflag.ContinueOnError)
	fs.Usage = func() {}
	fs.SetOutput(io.Discard)

	fs.BoolP("help", "h", false, "打印帮助信息")
	configHelp := "指定读取的配置文件位置"
	if ep.meta.WatchMode {
		configHelp = "指定监控的配置文件位置"
	}
	fs.StringP("config", "c", "", configHelp)

	r := jsonschema.Reflector{DoNotReference: true}
	schema := r.Reflect(ep.config)

	t := reflect.TypeOf(ep.config)
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if unicode.IsLower([]rune(f.Name)[0]) || f.Tag.Get("json") == "-" {
			continue
		}
		description, title := fieldSchemaInfo(schema, f)
		if err := registerFieldFlag(fs, f, title, description); err != nil {
			return nil, err
		}
	}
	return fs, nil
}

// fieldSchemaInfo 从 jsonschema 中获取字段的说明与标题
// @params schema *jsonschema.Schema 已生成的 schema
// @params f reflect.StructField 结构体字段
// @returns string 字段说明
// @returns string 字段标题(单字符时作为短 flag 名)
func fieldSchemaInfo(schema *jsonschema.Schema, f reflect.StructField) (description, title string) {
	name := ReflectFieldName(f)
	if fschema, ok := schema.Properties.Get(name); ok {
		return fschema.(*jsonschema.Schema).Description, fschema.(*jsonschema.Schema).Title
	}
	if jsonschemaTag := f.Tag.Get("jsonschema"); jsonschemaTag != "" {
		for _, tag := range strings.Split(jsonschemaTag, ",") {
			switch {
			case strings.HasPrefix(tag, "description="):
				description = strings.TrimPrefix(tag, "description=")
			case strings.HasPrefix(tag, "title="):
				title = strings.TrimPrefix(tag, "title=")
			}
		}
	}
	return description, title
}

// registerFieldFlag 为单个结构体字段注册命令行 flag(长名=小写 json/yaml 字段名,短名=jsonschema title)
// @params fs *pflag.FlagSet 目标 flag 集合
// @params f reflect.StructField 结构体字段
// @params title string jsonschema 标题(恰为单字符时作为短 flag)
// @params usage string 帮助说明
// @returns error 不支持的字段类型
// flagNameOf 返回字段对应的命令行长 flag 名:v4 起使用小写的 json/yaml 字段名(如 --a/--ok)
func flagNameOf(f reflect.StructField) string {
	return strings.ToLower(ReflectFieldName(f))
}

func registerFieldFlag(fs *pflag.FlagSet, f reflect.StructField, title, usage string) error {
	longName := flagNameOf(f)
	shortName := ""
	if len(title) == 1 {
		shortName = title
	}
	// 避免与内置 -h/-c 及已注册短名冲突
	if shortName != "" {
		if shortName == "h" || shortName == "c" || fs.ShorthandLookup(shortName) != nil {
			shortName = ""
		}
	}
	switch f.Type.Kind() {
	case reflect.String:
		fs.StringP(longName, shortName, "", usage)
	case reflect.Bool:
		fs.BoolP(longName, shortName, false, usage)
	case reflect.Int:
		fs.IntP(longName, shortName, 0, usage)
	case reflect.Float64:
		fs.Float64P(longName, shortName, 0.0, usage)
	case reflect.Slice:
		switch f.Type.String() {
		case "[]string":
			fs.StringArrayP(longName, shortName, []string{}, usage)
		case "[]int":
			fs.IntSliceP(longName, shortName, []int{}, usage)
		case "[]float64":
			fs.Float64SliceP(longName, shortName, []float64{}, usage)
		default:
			return fmt.Errorf("字段%s是未支持的类型%v", f.Name, f.Type)
		}
	default:
		return fmt.Errorf("字段%s是未支持的类型%v", f.Name, f.Type)
	}
	return nil
}

// usageString 生成当前节点的帮助文本
// @params fs *pflag.FlagSet 已构建的 flag 集合
// @returns string 帮助文本
func (ep *EndPoint[T]) usageString(fs *pflag.FlagSet) string {
	var b strings.Builder
	prog := GetNodeProg(ep)
	fmt.Fprintf(&b, "命令: %s\n", prog)
	fmt.Fprintf(&b, "用法: %s [选项]\n", prog)
	if ep.meta.Usage != "" {
		fmt.Fprintf(&b, "使用: %s\n", ep.meta.Usage)
	}
	if ep.meta.Description != "" {
		fmt.Fprintf(&b, "说明: %s\n", ep.meta.Description)
	}
	if !ep.meta.NotParseEnv {
		fmt.Fprintf(&b, "环境变量前缀: %s\n", ep.getEnvPrefix())
	}
	b.WriteString("\n选项:\n")
	b.WriteString(fs.FlagUsages())
	return b.String()
}

// flagIsChanged 判断命令行是否显式传入了字段对应的 flag
// @params fs *pflag.FlagSet 已解析完成的 flag 集合
// @params fieldName string 结构体字段名(即长 flag 名)
// @returns bool 是否被显式传入
func flagIsChanged(fs *pflag.FlagSet, fieldName string) bool {
	return fs.Lookup(fieldName) != nil && fs.Changed(fieldName)
}

// applyConfigDefaults 将 jsonschema 中的 default 应用到 config,作为基础默认值
// 在 passArgs 中于加载任何配置文件之前调用,保证 默认值<配置文件<环境变量<命令行 的优先级
// @generics T EndPointConfigInterface 内部`config`字段的类型
// @Returns error 解析过程中的错误
func (ep *EndPoint[T]) applyConfigDefaults() error {
	r := jsonschema.Reflector{DoNotReference: true}
	schema := r.Reflect(ep.config)
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
		if defau, ok := fieldDefaultOf(schema, f); ok {
			if err := applyDefaultValue(v.Field(i), f, defau); err != nil {
				return err
			}
		}
	}
	return nil
}

// fieldDefaultOf 返回字段在 jsonschema 中声明的 default 值
// @params schema *jsonschema.Schema 已生成的 schema
// @params f reflect.StructField 结构体字段
// @returns interface{} 默认值
// @returns bool 是否存在默认值
func fieldDefaultOf(schema *jsonschema.Schema, f reflect.StructField) (interface{}, bool) {
	if fschema, ok := schema.Properties.Get(ReflectFieldName(f)); ok {
		if d := fschema.(*jsonschema.Schema).Default; d != nil {
			return d, true
		}
	}
	return nil, false
}

// applyDefaultValue 将单个字段的默认值写入 config
// @params vf reflect.Value 字段值
// @params f reflect.StructField 结构体字段
// @params defau interface{} 默认值
// @returns error 默认值类型不匹配等错误
func applyDefaultValue(vf reflect.Value, f reflect.StructField, defau interface{}) error {
	switch f.Type.Kind() {
	case reflect.String:
		vf.SetString(defau.(string))
	case reflect.Bool:
		vf.SetBool(defau.(bool))
	case reflect.Int:
		switch dv := defau.(type) {
		case int:
			vf.SetInt(int64(dv))
		case float64:
			vf.SetInt(int64(dv))
		default:
			vf.Set(reflect.ValueOf(defau))
		}
	case reflect.Float64:
		switch dv := defau.(type) {
		case float64:
			vf.SetFloat(dv)
		case int:
			vf.SetFloat(float64(dv))
		default:
			vf.Set(reflect.ValueOf(defau))
		}
	case reflect.Slice:
		defa_i, ok := defau.([]interface{})
		if !ok {
			return fmt.Errorf("字段%s的默认值类型不支持%v", f.Name, f.Type)
		}
		switch f.Type.String() {
		case "[]string":
			defa_r := make([]string, 0, len(defa_i))
			for _, v := range defa_i {
				defa_r = append(defa_r, v.(string))
			}
			vf.Set(reflect.ValueOf(defa_r))
		case "[]int":
			defa_r := make([]int, 0, len(defa_i))
			for _, v := range defa_i {
				s, ok := v.(string)
				if !ok {
					if n, ok := v.(int); ok {
						defa_r = append(defa_r, n)
						continue
					}
					break
				}
				value, err := strconv.Atoi(s)
				if err != nil {
					break
				}
				defa_r = append(defa_r, value)
			}
			vf.Set(reflect.ValueOf(defa_r))
		case "[]float64":
			defa_r := make([]float64, 0, len(defa_i))
			for _, v := range defa_i {
				s, ok := v.(string)
				if !ok {
					if n, ok := v.(float64); ok {
						defa_r = append(defa_r, n)
						continue
					}
					break
				}
				value, err := strconv.ParseFloat(s, 64)
				if err != nil {
					break
				}
				defa_r = append(defa_r, value)
			}
			vf.Set(reflect.ValueOf(defa_r))
		default:
			return fmt.Errorf("字段%s是未支持的类型%v", f.Name, f.Type)
		}
	default:
		return fmt.Errorf("字段%s是未支持的类型%v", f.Name, f.Type)
	}
	return nil
}

// applyFlagsToConfig 解析环境变量与命令行参数,并设置到 config 对象中
// 优先级:默认值(applyConfigDefaults)< 配置文件 < 环境变量 < 命令行(仅当对应 flag 被显式传入时)
// @generics T EndPointConfigInterface 内部`config`字段的类型
// @Params fs *pflag.FlagSet 已解析完成的命令行 flag 集合
// @Returns error 解析过程中的错误
func (ep *EndPoint[T]) applyFlagsToConfig(fs *pflag.FlagSet) error {
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
		fname := flagNameOf(f)

		switch f.Type.Kind() {
		case reflect.String:
			{
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
				//设置命令行配置:仅当 flag 被显式传入时应用(修复显式传空串被默认值覆盖的问题)
				if flagIsChanged(fs, fname) {
					val, _ := fs.GetString(fname)
					vf.SetString(val)
				}
			}
		case reflect.Bool:
			{
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
				//设置命令行配置:bool flag 支持 --flag=true 与 --flag=false
				//仅当显式传入时应用(修复显式传 false 被默认值覆盖的问题)
				if flagIsChanged(fs, fname) {
					val, _ := fs.GetBool(fname)
					vf.SetBool(val)
				}

			}
		case reflect.Int:
			{
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
				//设置命令行配置:仅当 flag 被显式传入时应用(修复显式传 0 被默认值覆盖的问题)
				if flagIsChanged(fs, fname) {
					val, _ := fs.GetInt(fname)
					vf.SetInt(int64(val))
				}
			}
		case reflect.Float64:
			{
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
				//设置命令行配置:仅当 flag 被显式传入时应用(修复显式传 0.0 被默认值覆盖的问题)
				if flagIsChanged(fs, fname) {
					val, _ := fs.GetFloat64(fname)
					vf.SetFloat(val)
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
							logger.Debug("load config from ENV", log.Dict{"env": loadenv, "value": getenvstr})
						}
						if getenvstr != "" {
							sl := strings.Split(getenvstr, ",")
							vf.Set(reflect.ValueOf(sl))
						}
						//设置命令行配置:仅当 flag 被显式传入时应用
						if flagIsChanged(fs, fname) {
							val, _ := fs.GetStringArray(fname)
							vf.Set(reflect.ValueOf(val))
						}
					}
				case "[]int":
					{
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
						//设置命令行配置:仅当 flag 被显式传入时应用
						if flagIsChanged(fs, fname) {
							val, _ := fs.GetIntSlice(fname)
							vf.Set(reflect.ValueOf(val))
						}
					}
				case "[]float64":
					{
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
						//设置命令行配置:仅当 flag 被显式传入时应用
						if flagIsChanged(fs, fname) {
							val, _ := fs.GetFloat64Slice(fname)
							vf.Set(reflect.ValueOf(val))
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

// getEnvPrefix 获取实际的EnvPrefix
// @generics T EndPointConfigInterface 内部`config`字段的类型
func (ep *EndPoint[T]) getEnvPrefix() string {
	var EnvPrefix string
	if ep.meta.EnvPrefix != "" {
		EnvPrefix = ep.meta.EnvPrefix
	} else {
		EnvPrefix = strings.ToUpper(strings.Join(GetNodeProgList(ep), "_"))
	}
	return EnvPrefix
}

// verifyConfig 验证config是否符合要求
// @generics T EndPointConfigInterface 内部`config`字段的类型
// @Returns error 校验失败时返回错误
func (ep *EndPoint[T]) verifyConfig() error {
	if ep.meta.NotVerifySchema {
		return nil
	}
	configLoader := gojsonschema.NewGoLoader(ep.config)
	schemaLoader := gojsonschema.NewBytesLoader(ep.Schema())
	result, err := gojsonschema.Validate(schemaLoader, configLoader)
	if err != nil {
		return fmt.Errorf("执行 schema 校验失败: %w", err)
	}
	if result.Valid() {
		return nil
	}
	var b strings.Builder
	for _, e := range result.Errors() {
		fmt.Fprintf(&b, "%v; ", e.Details())
	}
	return fmt.Errorf("config 未通过 schema 校验: %s", strings.TrimSuffix(b.String(), "; "))
}

type StopWatchFunc func()

// startConfigfileWatch 开始监听配置文件
// @generics T EndPointConfigInterface 内部`config`字段的类型
// @returns StopWatchFunc 停止监听函数
// @returns error 程序错误
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

// GenFSWatcher 生成文件系统监听器
// @generics T EndPointConfigInterface 内部`config`字段的类型
// @params serialize_protocol SupportedSerialization 使用的序列化协议
// @params filepath string 文件路径
// @params indocker bool 文件系统是否在docker中
// @returns StopWatchFunc 停止监听函数
// @returns error 程序错误
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

// fsWatchHandler 监听文件系统执行操作
// @generics T EndPointConfigInterface 内部`config`字段的类型
// @params serialize_protocol SupportedSerialization 使用的序列化协议
// @params filepath string 文件路径
// @params watcher filenotify.FileWatcher 文件系统监听器
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

// refreshContentProcess 根据内容刷新配置
// @params serialize_protocol SupportedSerialization 使用的序列化协议
// @params content []byte 带序列化的内容
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

// refreshFSProcess 文件系统刷新配置流程
// @params serialize_protocol SupportedSerialization 文件使用的序列化协议
// @params filepath string 配置文件路径
// @returns bool 是否跳过更新
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

// GenEtcdWatcher 生成文件系统监听器
// @generics T EndPointConfigInterface 内部`config`字段的类型
// @params serialize_protocol SupportedSerialization 文件使用的序列化协议
// @params filepath string 文件路径
// @params config clientv3.Config etcd配置
// @returns StopWatchFunc 停止监听函数
// @returns error 程序错误
func (ep *EndPoint[T]) GenEtcdWatcher(serialize_protocol SupportedSerialization, filepath string, config clientv3.Config) (StopWatchFunc, error) {
	cli, err := clientv3.New(config)
	if err != nil {
		return nil, err
	}
	go ep.etcdWatchHandler(serialize_protocol, filepath, cli)
	return func() { cli.Close() }, nil
}

// etcdWatchHandler 监听etcd系统执行操作
// @generics T EndPointConfigInterface 内部`config`字段的类型
// @params serialize_protocol SupportedSerialization 使用的序列化协议
// @params filepath string key路径
// @params cli *clientv3.Client etcd连接
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
