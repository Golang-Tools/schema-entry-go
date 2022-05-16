package schemaentry

import (
	"net/url"
	"reflect"
	"strconv"
	"strings"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
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

//GetNodeProg 获取节点的prog值
func GetNodeProg(node EntryPointInterface) string {
	p := prog{
		result: []string{},
	}
	p.getNodeProgIter(node)
	reverse(p.result)
	return strings.Join(p.result, " ")
}

//GetNodeProgList 获取节点的prog值
func GetNodeProgList(node EntryPointInterface) []string {
	p := prog{
		result: []string{},
	}
	p.getNodeProgIter(node)
	reverse(p.result)
	return p.result
}

//GetNodeEnvPrefix 获取实际的EnvPrefix
func GetNodeEnvPrefix(node EntryPointInterface) string {
	var EnvPrefix string
	if node.Meta().EnvPrefix != "" {
		EnvPrefix = node.Meta().EnvPrefix
	} else {
		EnvPrefix = strings.ToUpper(strings.Join(GetNodeProgList(node), "_"))
	}
	return EnvPrefix
}

//ReflectFieldInfo 返回字段的基础信息
//@returns string 对应名字
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

//ParseFSUrl 解析文件系统的URL
//@params U *url.URL url信息
//@returns SupportedSerialization 序列化协议
//@returns string 路径
//@returns error 解析错误
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

//ParseFSUrl 解析文件系统的URL
//@params U *url.URL url信息
//@returns SupportedSerialization 序列化协议
//@returns string 路径
//@returns error 解析错误
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

//ParseEtcdUrl 解析Etcd的URL
//@params U *url.URL url信息
//@returns SupportedSerialization 序列化协议
//@returns string 路径,即key
//@returns clientv3.Config etcd连接配置
//@returns time.Duration etcd请求超时
//@returns error 解析错误
func ParseEtcdUrl(U *url.URL) (SupportedSerialization, string, clientv3.Config, time.Duration, error) {
	Config := clientv3.Config{}
	address := []string{U.Host}
	path := U.Path
	m := U.Query()
	serialize := m.Get("serialize")
	if serialize == "" {
		return SerializationJSON, "", Config, 0, ErrNotSetSerialize
	}
	var serialize_protocol SupportedSerialization
	switch serialize {
	case "JSON", "json", "Json":
		{
			serialize_protocol = SerializationJSON
		}
	case "YAML", "Yaml", "yaml", "YML", "Yml", "yml":
		{
			serialize_protocol = SerializationYAML
		}
	default:
		{
			return serialize_protocol, "", Config, 0, ErrUnsupportedSerialization
		}
	}
	add_address := m["address"]
	if add_address != nil {
		address = append(address, add_address...)
	}
	Config.Endpoints = address
	auto_sync_interval_s := m.Get("auto-sync-interval-ms")
	if auto_sync_interval_s != "" {
		auto_sync_interval, err := strconv.ParseInt(auto_sync_interval_s, 10, 64)
		if err != nil {
			return SerializationJSON, "", Config, 0, err
		}
		Config.AutoSyncInterval = time.Duration(auto_sync_interval) * time.Millisecond
	}
	dial_timeout_s := m.Get("dial-timeout-ms")
	if dial_timeout_s != "" {
		dial_timeout, err := strconv.ParseInt(dial_timeout_s, 10, 64)
		if err != nil {
			return SerializationJSON, "", Config, 0, err
		}
		Config.DialTimeout = time.Duration(dial_timeout) * time.Millisecond
	}
	dial_keep_alive_time_s := m.Get("dial-keep-alive-time-ms")
	if dial_keep_alive_time_s != "" {
		dial_keep_alive_time, err := strconv.ParseInt(dial_keep_alive_time_s, 10, 64)
		if err != nil {
			return SerializationJSON, "", Config, 0, err
		}
		Config.DialKeepAliveTime = time.Duration(dial_keep_alive_time) * time.Millisecond
	}
	dial_keep_alive_timeout_s := m.Get("dial-keep-alive-timeout-ms")
	if dial_keep_alive_timeout_s != "" {
		dial_keep_alive_timeout, err := strconv.ParseInt(dial_keep_alive_timeout_s, 10, 64)
		if err != nil {
			return SerializationJSON, "", Config, 0, err
		}
		Config.DialKeepAliveTimeout = time.Duration(dial_keep_alive_timeout) * time.Millisecond
	}
	max_call_send_msg_size_bytes_s := m.Get("max-call-send-msg-size-bytes")
	if max_call_send_msg_size_bytes_s != "" {
		max_call_send_msg_size_bytes, err := strconv.Atoi(max_call_send_msg_size_bytes_s)
		if err != nil {
			return SerializationJSON, "", Config, 0, err
		}
		Config.MaxCallSendMsgSize = max_call_send_msg_size_bytes
	}
	max_call_recv_msg_size_bytes_s := m.Get("max-call-recv-msg-size-bytes")
	if max_call_recv_msg_size_bytes_s != "" {
		max_call_recv_msg_size_bytes, err := strconv.Atoi(max_call_recv_msg_size_bytes_s)
		if err != nil {
			return SerializationJSON, "", Config, 0, err
		}
		Config.MaxCallRecvMsgSize = max_call_recv_msg_size_bytes
	}

	reject_old_cluster_s := m.Get("reject-old-cluster")
	switch reject_old_cluster_s {
	case "1", "True", "TRUE", "true", "OK", "ok", "Ok":
		{
			Config.RejectOldCluster = true
		}
	}
	permit_without_stream_s := m.Get("permit-without-stream")
	switch permit_without_stream_s {
	case "1", "True", "TRUE", "true", "OK", "ok", "Ok":
		{
			Config.PermitWithoutStream = true
		}
	}
	if U.User.Username() != "" {
		Config.Username = U.User.Username()
	}
	pwd, ok := U.User.Password()
	if ok && pwd != "" {
		Config.Password = pwd
	}
	timeout := time.Duration(50) * time.Millisecond
	query_timeout_s := m.Get("query-timeout-ms")
	if query_timeout_s != "" {
		timeout_ms, err := strconv.ParseInt(query_timeout_s, 10, 64)
		if err != nil {
			return SerializationJSON, "", Config, 0, err
		}
		timeout = time.Duration(timeout_ms) * time.Millisecond
	}
	return serialize_protocol, path, Config, timeout, nil
}
