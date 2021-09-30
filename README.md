# schema-entry-go

通过定义结构体同时声明jsonschem提供复杂的启动参数设置项

## 特性

+ 支持多级子命令
+ 每级子命令会提示其下一级的子命令
+ 终点节点可以通过定义一个满足接口`EntryPointConfig`的结构体来指定参数的解析行为和入口函数的执行过程
+ 可以通过默认值,指定位置文件,环境变量,命令行参数来构造配置结构,其顺序是`命令行参数->环境变量->命令行指定的配置文件->配置指定的配置文件路径->默认值`
+ 通过[定义满足接口`EntryPointConfig`的结构体中的`jsonschema`tag](https://github.com/alecthomas/jsonschema)来定义配置的校验规则
+ 支持`json`和`yaml`两种格式的配置文件

## 概念和一些规则

### 节点

我们使用节点来描述命令之间的关系,定义3类节点:

1. 根节点,一颗节点树的起始点,它没有父节点
2. 叶子节点,节点树当中没有子节点的节点
3. 枝节点,既有父节点又有子节点的节点

一个节点最多只能有一个父节点,但可以有多个子节点.节点与节点间可以使用`RegistSubNode(parent,child EntryPointNode)`函数来注册

### 参数的解析规则

1. 所有节点都会解析其`--help`命令,叶子节点会解析命令,用法,说明,以及参数,其他节点则是解析其命令,说明和支持的子命令.

    节点的名字使用`EntryPointMeta.Name`定义如果不定义则查找`config`字段有没有设置,如果设置了则使用`config`对象的结构体名,如果没有则会报错.

    命令为根节点到当前节点名字中间用空格分隔.

    说明使用`EntryPointMeta.Usage`设置.

2. 只有定义了config对象的叶子节点才会进行参数解析
    config满足接口:

    ```golang
    //EntryPointConfig 节点配置
    type EntryPointConfig interface {
        Main()
    }
    ```

    解析的流程如下:

    ```bash
    使用配置指定路径的配置替换默认值(可以通过`EntryPointMeta.DefaultConfigFilePaths`配置默认路径)
                |
                v
    使用命令行参数`--config_path`指定的配置文件替换默认值(如果不为空字符串)
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

## 使用方法

整个使用流程可以拆分为如下步骤

1. 定义一个配置结构体,并为其实现`Main()`接口,这个`Main()`接口就是业务的入口
2. 使用`schema-entry-go.New(*EntryPointMeta, ...EntryPointConfig) (*EntryPoint, error)`来构造一个解析节点,如果这个节点不作为叶子节点那可以不填`...EntryPointConfig`部分参数
3. 使用`schema-entry-go.RegistSubNode(parent *EntryPoint,child *EntryPoint)`或者`parent.RegistSubNode(child *EntryPoint)`来构造命令树结构.
4. 调用根节点的`Parse(argv []string)`方法解析配置,一般是写成`root.Parse(os.Args)`

下面是一个例子,例子中使用`github.com/alecthomas/jsonschema`在结构体构造时声明了jsonschema约束.

```golang
package main

import (
    "fmt"
    "os"

    s "github.com/Golang-Tools/schema-entry-go"
    "github.com/alecthomas/jsonschema"
    jsoniter "github.com/json-iterator/go"
)

var json = jsoniter.ConfigCompatibleWithStandardLibrary

type C struct {
    A     int   `json:"a" jsonschema:"required,title=field,description=测试列表"`
    Field []int `json:"field" jsonschema:"required,title=field,description=测试列表"`
    s     int
}

func (c *C) Main() {
    fmt.Println(c.Field)
    fmt.Println(c.A)
}

func main() {
    root, _ := s.New(&s.EntryPointMeta{Name: "foo", Usage: "foo cmd test"})
    nodeb, _ := s.New(&s.EntryPointMeta{Name: "bar", Usage: "foo bar cmd test"})
    nodec, _ := s.New(&s.EntryPointMeta{Name: "par", Usage: "foo bar par cmd test"}, &C{
        Field: []int{1, 2, 3},
    })
    s.RegistSubNode(root, nodeb)
    nodeb.RegistSubNode(nodec)
    os.Setenv("FOO_BAR_PAR_A", "123")
    root.Parse([]string{"foo", "bar", "par", "--Field=4", "--Field=5", "--Field=6"})
}
```

使用时需要注意:

+ 函数`New`,`RegistSubNode`以及节点对象的`RegistSubNode`方法中传入的都是指针而非值.

## 缺陷

+ 目前不支持命令行位置参数.(依赖的`github.com/akamensky/argparse`目前不支持)
+ 目前只支持如下几种数据类型(依赖的`github.com/akamensky/argparse`目前不支持)

    + `int`,`float64`,`bool`,`string`
    + `[]int`,`[]float64`,`[]string`
