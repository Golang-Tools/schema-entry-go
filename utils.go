package schemaentry

import (
	"net/url"
	"reflect"
	"strings"
)

type prog struct {
	result []string
}

func (p *prog) getNodeProgIter(node EntryPointInterface) {
	meta := node.Meta()
	name := meta.Name
	p.result = append(p.result, name)
	if node.IsRoot() {
		return
	}
	p.getNodeProgIter(meta.Parent())
}
func reverse(s []string) []string {
	for i, j := 0, len(s)-1; i < j; i, j = i+1, j-1 {
		s[i], s[j] = s[j], s[i]
	}
	return s
}

// GetNodeProg 获取节点的prog值
func GetNodeProg(node EntryPointInterface) string {
	p := prog{
		result: []string{},
	}
	p.getNodeProgIter(node)
	reverse(p.result)
	return strings.Join(p.result, " ")
}

// GetNodeProgList 获取节点的prog值
func GetNodeProgList(node EntryPointInterface) []string {
	p := prog{
		result: []string{},
	}
	p.getNodeProgIter(node)
	reverse(p.result)
	return p.result
}

// GetNodeEnvPrefix 获取实际的EnvPrefix
func GetNodeEnvPrefix(node EntryPointInterface) string {
	var EnvPrefix string
	if node.Meta().EnvPrefix != "" {
		EnvPrefix = node.Meta().EnvPrefix
	} else {
		EnvPrefix = strings.ToUpper(strings.Join(GetNodeProgList(node), "_"))
	}
	return EnvPrefix
}

// ReflectFieldInfo 返回字段的基础信息
// @returns string 对应名字
func ReflectFieldName(f reflect.StructField) string {
	jsonTags, exist := f.Tag.Lookup("json")
	yamlTags, yamlExist := f.Tag.Lookup("yaml")
	if !exist {
		jsonTags = yamlTags
		exist = yamlExist
	}

	jsonTagsList := strings.Split(jsonTags, ",")

	name := f.Name

	if jsonTagsList[0] != "" {
		name = jsonTagsList[0]
	}

	// field anonymous but without json tag should be inherited by current type
	if f.Anonymous && !exist {
		name = strings.ToLower(name)
	}

	return name
}

// ParseFSUrl 解析文件系统的URL
// @params U *url.URL url信息
// @returns SupportedSerialization 序列化协议
// @returns string 路径
// @returns error 解析错误
func ParseFSPath(path string) (SupportedSerialization, string, error) {
	var serialize_protocol SupportedSerialization
	if strings.HasSuffix(path, "json") {
		serialize_protocol = SerializationJSON
	} else if strings.HasSuffix(path, "yml") || strings.HasSuffix(path, "yaml") {
		serialize_protocol = SerializationYAML
	} else {
		return serialize_protocol, "", ErrUnsupportedSerialization
	}
	return serialize_protocol, path, nil
}

// ParseFSUrl 解析文件系统的URL
// @params U *url.URL url信息
// @returns SupportedSerialization 序列化协议
// @returns string 路径
// @returns error 解析错误
func ParseFSUrl(U *url.URL) (SupportedSerialization, string, error) {
	path := U.Path
	var serialize_protocol SupportedSerialization
	if strings.HasSuffix(path, "json") {
		serialize_protocol = SerializationJSON
	} else if strings.HasSuffix(path, "yml") || strings.HasSuffix(path, "yaml") {
		serialize_protocol = SerializationYAML
	} else {
		return serialize_protocol, "", ErrUnsupportedSerialization
	}
	return serialize_protocol, path, nil
}
