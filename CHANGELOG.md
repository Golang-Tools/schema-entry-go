# v0.0.6

## 提示优化

+ 提示信息现在可以`meta`中通过`debugmod`设置是否打印一些用于debug的log

## 新增特性

+ 支持yaml格式的配置文件

## 文档优化

+ READEM中更加详细的介绍了用法模式

## 依赖更新

+ `github.com/alecthomas/jsonschema@v0.0.0-20210920000243-787cd8204a0d`

# v0.0.5

## 依赖更新

+ `github.com/Golang-Tools/loggerhelper@v0.0.4`
+ `github.com/akamensky/argparse@v1.3.1`
+ `github.com/alecthomas/jsonschema@v0.0.0-20210911035550-29dd2c10c8e9`
+ `github.com/json-iterator/go@v1.1.12`

# v0.0.4

## bug修正

+ 修复了bool型配置会被命令行强行覆盖的问题,现在只有schema中指定了`required`的布尔型配置会被命令行强行覆盖

# v0.0.3

## 提示优化

+ `EntryPointMeta`新增`Description`字段用于描述命令
+ 命令行help中会显式从环境变量中读取参数的前缀
+ 从环境变量中读到的配置将被展示出来.

# v0.0.2

## 接口变更

+ 不再需要实现schema,但需要在要注册的叶子节点结构体中定义`jsonschema`(具体看`github.com/alecthomas/jsonschema`)
+ `EntryPointMeta.ParseEnv`改为了`EntryPointMeta.NotParseEnv`
+ 改用`EntryPointMeta.NotVerifySchema`来设置是否校验json schema

# v0.0.1

## 更新依赖

+ `github.com/Golang-Tools/loggerhelper v0.0.2` -> `github.com/Golang-Tools/loggerhelper v0.0.3`

# v0.0.0

创建了项目,实现了基本功能.
