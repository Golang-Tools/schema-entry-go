# schema-entry-go/V2

通过定义结构体同时声明jsonschem提供复杂的启动参数设置项

V2版本针对1.18以上的golang,大量使用了泛型.低版本请使用[V1版本](https://github.com/Golang-Tools/schema-entry-go/tree/v1),V1版本将不再更新

## 特性

+ 支持多级子命令
+ 每级子命令会提示其下一级的子命令
+ 终点节点可以通过定义一个满足接口`EndPointConfigInterface`的结构体来指定参数的解析行为和入口函数的执行过程
+ 可以通过默认值,指定位置文件,环境变量,命令行参数来构造配置结构,其顺序是`命令行参数->环境变量->命令行指定的配置文件->配置指定的配置文件路径->默认值`
+ 通过[定义满足接口`EndPointConfigInterface`的结构体中的`jsonschema`tag](https://github.com/alecthomas/jsonschema)来定义配置的校验规则
+ 支持`json`和`yaml`两种格式的配置文件
+ 支持`watch`模式,可以通过监听指定文件更新配置

## 概念和一些规则

### 节点

我们使用节点来描述命令之间的关系,定义3类节点:

1. 根节点,一颗节点树的起始点,它没有父节点
2. 叶子节点,节点树当中没有子节点的节点
3. 枝节点,既有父节点又有子节点的节点

叶子节点或者是要执行的的根节点我们使用`func NewEndPoint[T EndPointConfigInterface](config T, opts ...optparams.Option[EntryPointMeta]) (*EndPoint[T], error)`来创建

不用执行的根节点和枝节点我们使用`func NewEntryPoint(opts ...optparams.Option[EntryPointMeta]) (*EntryPoint, error)`来创建

一个节点最多只能有一个父节点,但可以有多个子节点.节点与节点间可以使用如下几个方式来注册:

+ `func RegistSubNode(parent, child EntryPointInterface)`函数
+ `func (EntryPointInterface) SetChild(child EntryPointInterface) error`,这个方法当在叶子节点上注册子节点时会报错
+ `func (EntryPointInterface) SetParent(parent EntryPointInterface) EntryPointInterface`,这个方法一般从叶子节点开始向根节点注册.返回的是父节点,所以可以用pipeline的形式注册

### 参数的解析规则

1. 所有节点都会解析其`--help`命令,叶子节点会解析命令,用法,说明,以及参数,其他节点则是解析其命令,说明和支持的子命令.

    节点的名字使用`EntryPointMeta.Name`定义如果不定义则查找`config`字段有没有设置,如果设置了则使用`config`对象的结构体名,如果没有则会报错.

    命令为根节点到当前节点名字中间用空格分隔.

    说明使用`EntryPointMeta.Usage`设置.

2. 只有定义了config对象的叶子节点才会进行参数解析
    config满足接口:

    ```golang
    //EndPointConfigInterface 叶子节点配置接口
    type EndPointConfigInterface interface {
        Main()         //进入时执行的程序
    }
    ```

    解析的流程如下:

    ```bash
    使用配置指定路径的配置替换默认值(可以通过`EntryPointMeta.DefaultConfigFilePaths`配置默认路径)
                |
                v
    使用命令行参数`--config`指定的配置文件替换默认值(如果不为空字符串)
                |
                v

    使用环境变量替换默认值(如果设置`EntryPointMeta.NotParseEnv: false`)
    可以设置前缀`EntryPointMeta.EnvPrefix`作为环境变量的命名空间,默认前缀为根节点到当前节点间所有节点名中间以`_`分隔.
    前缀和参数间使用`_`分隔
                |
                v
    使用命令行参数替换默认值(如果对应flag有设置)
                |
                v
    解析jsonschema校验规则(如果设置`EntryPointMeta.NotVerifySchema:true`)
                |
                v
    执行`config.Main`
    ```

## watchmode

监听模式用于持续监听一个文件以保持配置最新,在更新模式下我们必须在命令行里显示的指定`-c`或者`--config`,来指向一个路径,支持两种方式指定路径:

+ path模式,即直接指定文件系统中的路径,这种方式只能针对本地文件系统,使用的路径可以为绝对路径或者相对路径
+ url模式,即使用url的形式指定路径,其形式为`schema://user:password@host:port/path?params`这种方式相对比较有扩展性,支持的获取方式有
    + 本地文件系统,使用`schema`可以为`file`, `fs`,使用的路径只能是绝对路径,比如`file:///Users/mac/WORKSPACE/GITHUB/GolangTools/schema-entry-go/watch.json`
    + docker容器中的文件系统,使用`schema`为`dockerfs`,使用的路径只能是绝对路径,比如`dockerfs:///Users/mac/WORKSPACE/GITHUB/GolangTools/schema-entry-go/watch.json`
    + etcd中的内容,使用`schema`为`etcd`,其形式如`etcd://localhost:9800/foo/bar?address=192.168.1.1:4324&address=192.168.1.2:4324&serialize=JSON`,其中
        + host:port部分以及address部分填写etcd的访问地址,也就是说至少要有一个地址
        + path部分为etcd中的内容所在的key
        + 参数中的serialize指明序列化协议,一样的目前只支持JSON和YAML.
        + 其他支持的配置项还包括:
            + `auto-sync-interval-ms`
            + `dial-timeout-ms`
            + `dial-keep-alive-time-ms`
            + `dial-keep-alive-timeout-ms`
            + `max-call-send-msg-size-bytes`
            + `max-call-recv-msg-size-bytes`
            + `reject-old-cluster`
            + `permit-without-stream`
            + `query-timeout-ms`,请求etcd的超时,默认50ms

## 使用方法

整个使用流程可以拆分为如下步骤

1. 定义一个配置结构体,并为其实现`Main()`接口,这个`Main()`接口就是业务的入口
2. 使用`NewEndPoint`来构造一个叶子节点,如果有多个叶子节点可以用`NewEntryPoint`来构造根节点和枝节点用于串联叶子节点
3. [可选]如果有根节点和枝节点则将各个节点串联成树
4. 调用根节点的`Parse(argv []string)`方法解析配置,一般是写成`root.Parse(os.Args)`

下面是一个例子,例子中使用`github.com/alecthomas/jsonschema`在结构体构造时声明了jsonschema约束.

```golang
package main

import (
    "fmt"
    "os"
    "time"

    s "github.com/Golang-Tools/schema-entry-go/v2"
    jsoniter "github.com/json-iterator/go"
)

var json = jsoniter.ConfigCompatibleWithStandardLibrary

type C struct {
    A            int   `yaml:"aa" jsonschema:"required,title=a,description=测试int,maximum=10,default=10"`
    B            int   `yaml:"b" jsonschema:"required,title=b,description=测试int,maximum=10,default=100"`
    OK           bool  `json:"ok" jsonschema:"title=o,description=测试bool"`
    Field        []int `json:"field" jsonschema:"required,title=f,description=测试列表"`
    FieldDefault []int `json:"field_default" jsonschema:"required,title=d,description=测试列表默认值,default=1,default=2,default=3,default=4,default=5"`
    WatchValue   int   `json:"WatchValue" jsonschema:"required,title=w,description=测试监听"`
    s            int
}

func (c *C) Test() {
    fmt.Println(c)
}

func (c *C) Main() {
    c.Test()
    time.Sleep(time.Duration(1) * time.Minute)
}

func main() {
    root, _ := s.NewEntryPoint(s.WithName("foo"), s.WithDescription("测试用foo"), s.WithUsage("foo cmd test"))
    nodeb, _ := s.NewEntryPoint(s.WithName("bar"), s.WithDescription("测试用foo bar"), s.WithUsage("foo bar cmd test"))
    nodec, _ := s.NewEndPoint(new(C), s.WithName("par"), s.WithNotVerifySchema(),
        s.WithDefaultConfigFilePaths("conf.json", "config.json", "testconf.yml"),
        s.WithDescription("测试用foo bar par"),
        s.WithUsage("foo bar par cmd test"),
        s.WithLoadAllConfigFile(),
        s.WithWatchMode(),
    )
    nodec.OnRefresh(func(c *C) {
        c.Test()
    })
    os.Setenv("FOO_BAR_PAR_A", "123")
    nodec.SetParent(nodeb).SetParent(root).Parse(os.Args)
}

```

## 缺陷

+ 目前不支持命令行位置参数.(依赖的`github.com/akamensky/argparse`目前不支持)
+ 目前只支持如下几种数据类型(依赖的`github.com/akamensky/argparse`目前不支持)

    + `int`,`float64`,`bool`,`string`
    + `[]int`,`[]float64`,`[]string`
    + 如果是`[]int`,`[]float64`,`[]string`这三种类型设置`default`需要像这样写

        ```golang
        FieldDefault []int `json:"field_default" jsonschema:"default=1,default=2,default=3,default=4,default=5"`
        ```

        它等价于JSONSchema中的`default:[1,2,3,4,5]`
