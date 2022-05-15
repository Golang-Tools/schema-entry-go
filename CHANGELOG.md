# v2.0.0

该版本进行大规模接口变动,使用泛型重构本项目.

主要改动

1. 修改使用的jsonschema映射包为`github.com/invopop/jsonschema`
2. 增加watchmode用于持续更新配置
3. 可以在jsonschema中使用`default`字段规定默认值