# v3.0.0

模块路径变更为 `github.com/Golang-Tools/schema-entry-go/v3`,最低 go 版本提升到 1.22。

## 破坏性变更

+ 模块路径由 `/v2` 变更为 `/v3`
+ 最低 go 版本提升到 1.22

## 依赖迁移

+ `github.com/Golang-Tools/loggerhelper/v2` → `github.com/Golang-Tools/loggerhelper/v3`(logrus 后端,面向应用 API 兼容)
+ `github.com/Golang-Tools/optparams` v0.0.1 → v1.0.0
+ `github.com/json-iterator/go` 移除,改用标准库 `encoding/json`
+ `gopkg.in/yaml.v2` → `gopkg.in/yaml.v3`
+ `github.com/akamensky/argparse` 移除,命令行解析改用 `github.com/spf13/pflag`

## bug 修复

+ 修复 int/float/string 类型在命令行显式传入 `0`/`0.0`/空串时被 jsonschema 默认值覆盖的问题(改为仅当 flag 被显式传入时才覆盖,使用 `pflag` 的 presence 判断)
+ 修复 bool 类型无法通过命令行显式置 `false` 的问题:现在支持 `--Flag`(置 true)与 `--Flag=false`(置 false),不会再被 `default=true` 覆盖
+ 修复配置加载优先级与文档不符的问题:jsonschema `default` 现在作为最低优先级的基础值,在加载任何配置文件之前应用,保证 `默认值 < 默认配置文件 < --config 指定文件 < 环境变量 < 命令行`
+ 修复 `url.Parse` 解析失败后仍对可能为 nil 的 `url.URL` 取值的问题
+ 简化 `EntryPointMeta.SetChild` 冗余的 nil 检查

## 其它

+ 命令行解析重构为内部可测的 `buildConfigFlagSet`/`applyConfigDefaults`/`applyFlagsToConfig`,供单元测试直接调用
+ 新增单元测试:CLI 默认值/显式零值/bool/优先级回归、URL 与序列化解析、元数据与节点树、配置文件加载
+ 将原先 `functest`/`testhelper` 两个独立嵌套模块收敛为根模块内的 `example/watch` 与 `example/seed`,去掉嵌套 module 与 `go.work`;现在可用 `go run ./example/watch` 直接演示(默认监听本地 `watch.json`,或传入 etcd url),`go run ./example/seed` 往 etcd 写入配置

# v2.1.0

## 新增特性

+ 增加对etcd的支持,支持从etcd中获取配置,支持监听etcd中指定路径的配置

# v2.0.1

## 实现改进

+ 改用`github.com/Golang-Tools/loggerhelper/v2`替代`github.com/Golang-Tools/loggerhelper`
+ 不再使用全局log而是使用模块自己的log对象

## bug修复

+ 修复`WithConfig`选项设置`WatchMode`无效的问题

## 文档更新

+ 接口文档更新到v2.0.1版本

# v2.0.0

该版本进行大规模接口变动,使用泛型重构本项目.

主要改动

1. 修改使用的jsonschema映射包为`github.com/invopop/jsonschema`
2. 增加watchmode用于持续更新配置
3. 可以在jsonschema中使用`default`字段规定默认值
