# v4.0.1

文档/元数据修正版:核心 API 与运行行为与 v4.0.0 **完全一致**,可平滑替换升级(仅为让模块包内的文档与 contrib 路径正确)。

## 修正

+ contrib 子模块的模块路径去掉中间的 `/v4` 段,改为 `github.com/Golang-Tools/schema-entry-go/contrib/<name>`。
  原路径 `.../schema-entry-go/v4/contrib/<name>` 会被 Go 解析为「仓库根 + 子目录 `v4/contrib/<name>`」,
  从而要求仓库内存在 `v4/contrib/<name>/go.mod`(实际不存在),导致该模块无法通过 go 模块代理获取。
  现对应标签为 `contrib/<name>/v0.1.0`(首个版本 `v0.1.0`)。
+ `README.md` 中 contrib 包的导入路径同步修正。
+ contrib 子模块仍只需 `github.com/Golang-Tools/schema-entry-go/v4 v4.0.0`,无需随本版本升级。

# v4.0.0

模块路径变更为 `github.com/Golang-Tools/schema-entry-go/v4`(破坏性改造),最低 go 版本 1.22。

## 破坏性变更

+ `EntryPointInterface.Parse([]string)` → `Parse([]string) error`;叶子节点解析成功后才执行 `config.Main`
+ 库内彻底移除 `os.Exit`:解析/加载/校验失败以 error 返回;请求帮助(`-h`)返回包装 `ErrHelp` 的 `UsageError`(其文本携带用法),退出码交由调用方决定
+ 日志后端由 `loggerhelper/v3`(logrus)切换为 `loggerhelper/v4`(标准库 `log/slog`)
+ 命令行长 flag 由结构体字段名改为小写 json/yaml 字段名(如 `--A`→`--a`、`--OK`→`--ok`)

## 架构变化(可扩展配置源与监控)

+ 核心新增 `ConfigLoader`/`Watcher` SPI 与按 scheme 注册表(`RegisterConfigLoader`/`RegisterWatcher`)
+ 核心仅内置文件系统:`""/file/fs/dockerfs` 加载 + 纯标准库轮询监听(零额外依赖)
+ 移除核心对 `docker`、`etcd` 的直接依赖与相关实现
+ 新增可选的 contrib 嵌套子模块(应用侧空导入即启用):
  + `contrib/etcd`:etcd 配置源(加载 + 监听)与 `ParseEtcdUrl`
  + `contrib/fsnotify`:基于 fsnotify 的本地监听
  + `contrib/dockerfilenotify`:基于 docker pkg/filenotify 的监听(本地事件 + dockerfs 轮询)
+ contrib 子模块独立版本化:模块路径为 `github.com/Golang-Tools/schema-entry-go/contrib/<name>`,首个版本 `v0.1.0`,标签形如 `contrib/<name>/v0.1.0`
  (注:模块路径中间不能出现 `/v4` 段——Go 会把它当作子目录 `v4/contrib/<name>` 去找 `go.mod`)

## 其它

+ `verifyConfig`/`passArgs`/`getConfigFromConfigFile` 改为返回 error;`Schema()` 等内部逻辑沿用
+ 新增 `ErrHelp`、`ErrWatchOnRefreshNotSet` 与 `UsageError`
+ `example/seed`(etcd 演示)自核心移除,etcd 相关能力改由 `contrib/etcd` 提供
+ 单元测试覆盖:轮询 watcher、fs 加载器、未注册 scheme 报错、`contrib/fsnotify`/`contrib/dockerfilenotify` watcher 事件
+ `go.mod`/`go.sum` 各模块最终以发布时联网 `go mod tidy` 收口

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
