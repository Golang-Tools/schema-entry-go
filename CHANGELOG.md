# 0.0.2

## 接口变更

+ 不再需要实现schema,但需要在要注册的叶子节点结构体中定义`jsonschema`(具体看`github.com/alecthomas/jsonschema`)
+ `EntryPointMeta.ParseEnv`改为了`EntryPointMeta.NotParseEnv`
+ 改用`EntryPointMeta.NotVerifySchema`来设置是否校验json schema

# 0.0.1

## 更新依赖

+ `github.com/Golang-Tools/loggerhelper v0.0.2` -> `github.com/Golang-Tools/loggerhelper v0.0.3`

# 0.0.0

创建了项目,实现了基本功能.