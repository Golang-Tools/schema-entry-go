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
