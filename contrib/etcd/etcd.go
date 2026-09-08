// Package etcd 为 schema-entry-go 提供 etcd 配置源与监听支持,是可选的 contrib 子模块。
//
// 在应用中空导入即可自动注册 "etcd" scheme 的加载器与监听器:
//
//	import _ "github.com/Golang-Tools/schema-entry-go/v4/contrib/etcd"
//
// 之后便可在 watchmode 下用 -c 指定 etcd url,例如:
//
//	etcd://user:pwd@localhost:12379/foo/bar?serialize=JSON&dial-timeout-ms=500&query-timeout-ms=200
//
// 详细参数参见 ParseEtcdUrl。
package etcd

import (
	"context"
	"net/url"
	"strconv"
	"time"

	schemaentry "github.com/Golang-Tools/schema-entry-go/v4"
	clientv3 "go.etcd.io/etcd/client/v3"
)

func init() {
	schemaentry.RegisterConfigLoader(etcdLoader{})
	schemaentry.RegisterWatcher("etcd", etcdWatcherFactory)
}

// etcdLoader 从 etcd 读取配置内容
type etcdLoader struct{}

func (etcdLoader) Schemes() []string { return []string{"etcd"} }

func (etcdLoader) Load(rawurl string) ([]byte, schemaentry.SupportedSerialization, error) {
	U, err := url.Parse(rawurl)
	if err != nil {
		return nil, 0, err
	}
	serialize, key, config, timeout, err := ParseEtcdUrl(U)
	if err != nil {
		return nil, 0, err
	}
	content, err := readEtcd(key, config, timeout)
	if err != nil {
		return nil, 0, err
	}
	return content, serialize, nil
}

// etcdWatcherFactory 创建 etcd 配置监听器
func etcdWatcherFactory(rawurl string) (schemaentry.Watcher, error) {
	U, err := url.Parse(rawurl)
	if err != nil {
		return nil, err
	}
	_, key, config, _, err := ParseEtcdUrl(U)
	if err != nil {
		return nil, err
	}
	cli, err := clientv3.New(config)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	w := &etcdWatcher{
		cli:    cli,
		key:    key,
		ctx:    ctx,
		cancel: cancel,
		events: make(chan struct{}),
	}
	go w.run()
	return w, nil
}

type etcdWatcher struct {
	cli    *clientv3.Client
	key    string
	ctx    context.Context
	cancel context.CancelFunc
	events chan struct{}
}

// Events 返回触发刷新的信号通道
func (w *etcdWatcher) Events() <-chan struct{} { return w.events }

// Close 停止监听并关闭连接
func (w *etcdWatcher) Close() error {
	w.cancel()
	w.cli.Close()
	return nil
}

func (w *etcdWatcher) run() {
	rch := w.cli.Watch(w.ctx, w.key)
	for {
		select {
		case <-w.ctx.Done():
			close(w.events)
			return
		case wresp, ok := <-rch:
			if !ok {
				close(w.events)
				return
			}
			for _, ev := range wresp.Events {
				if ev.Type == clientv3.EventTypePut && string(ev.Kv.Key) == w.key {
					select {
					case w.events <- struct{}{}:
					case <-w.ctx.Done():
						close(w.events)
						return
					}
				}
			}
		}
	}
}

// readEtcd 读取 etcd 指定 key 的内容
func readEtcd(key string, config clientv3.Config, timeout time.Duration) ([]byte, error) {
	cli, err := clientv3.New(config)
	if err != nil {
		return nil, err
	}
	defer cli.Close()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	resp, err := cli.Get(ctx, key, clientv3.WithLastRev()...)
	if err != nil {
		return nil, err
	}
	if len(resp.Kvs) != 1 {
		return nil, schemaentry.ErrEtcdKeyLenNotMatch
	}
	return resp.Kvs[0].Value, nil
}

// ParseEtcdUrl 解析 Etcd 的 URL(原 schema-entry-go 核心的同名函数已迁移至此)
// @params U *url.URL url信息
// @returns schemaentry.SupportedSerialization 序列化协议
// @returns string 路径,即key
// @returns clientv3.Config etcd连接配置
// @returns time.Duration etcd请求超时
// @returns error 解析错误
func ParseEtcdUrl(U *url.URL) (schemaentry.SupportedSerialization, string, clientv3.Config, time.Duration, error) {
	Config := clientv3.Config{}
	address := []string{U.Host}
	path := U.Path
	m := U.Query()
	serialize := m.Get("serialize")
	if serialize == "" {
		return schemaentry.SerializationJSON, "", Config, 0, schemaentry.ErrNotSetSerialize
	}
	var serialize_protocol schemaentry.SupportedSerialization
	switch serialize {
	case "JSON", "json", "Json":
		serialize_protocol = schemaentry.SerializationJSON
	case "YAML", "Yaml", "yaml", "YML", "Yml", "yml":
		serialize_protocol = schemaentry.SerializationYAML
	default:
		return serialize_protocol, "", Config, 0, schemaentry.ErrUnsupportedSerialization
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
			return schemaentry.SerializationJSON, "", Config, 0, err
		}
		Config.AutoSyncInterval = time.Duration(auto_sync_interval) * time.Millisecond
	}
	dial_timeout_s := m.Get("dial-timeout-ms")
	if dial_timeout_s != "" {
		dial_timeout, err := strconv.ParseInt(dial_timeout_s, 10, 64)
		if err != nil {
			return schemaentry.SerializationJSON, "", Config, 0, err
		}
		Config.DialTimeout = time.Duration(dial_timeout) * time.Millisecond
	}
	dial_keep_alive_time_s := m.Get("dial-keep-alive-time-ms")
	if dial_keep_alive_time_s != "" {
		dial_keep_alive_time, err := strconv.ParseInt(dial_keep_alive_time_s, 10, 64)
		if err != nil {
			return schemaentry.SerializationJSON, "", Config, 0, err
		}
		Config.DialKeepAliveTime = time.Duration(dial_keep_alive_time) * time.Millisecond
	}
	dial_keep_alive_timeout_s := m.Get("dial-keep-alive-timeout-ms")
	if dial_keep_alive_timeout_s != "" {
		dial_keep_alive_timeout, err := strconv.ParseInt(dial_keep_alive_timeout_s, 10, 64)
		if err != nil {
			return schemaentry.SerializationJSON, "", Config, 0, err
		}
		Config.DialKeepAliveTimeout = time.Duration(dial_keep_alive_timeout) * time.Millisecond
	}
	max_call_send_msg_size_bytes_s := m.Get("max-call-send-msg-size-bytes")
	if max_call_send_msg_size_bytes_s != "" {
		max_call_send_msg_size_bytes, err := strconv.Atoi(max_call_send_msg_size_bytes_s)
		if err != nil {
			return schemaentry.SerializationJSON, "", Config, 0, err
		}
		Config.MaxCallSendMsgSize = max_call_send_msg_size_bytes
	}
	max_call_recv_msg_size_bytes_s := m.Get("max-call-recv-msg-size-bytes")
	if max_call_recv_msg_size_bytes_s != "" {
		max_call_recv_msg_size_bytes, err := strconv.Atoi(max_call_recv_msg_size_bytes_s)
		if err != nil {
			return schemaentry.SerializationJSON, "", Config, 0, err
		}
		Config.MaxCallRecvMsgSize = max_call_recv_msg_size_bytes
	}
	reject_old_cluster_s := m.Get("reject-old-cluster")
	switch reject_old_cluster_s {
	case "1", "True", "TRUE", "true", "OK", "ok", "Ok":
		Config.RejectOldCluster = true
	}
	permit_without_stream_s := m.Get("permit-without-stream")
	switch permit_without_stream_s {
	case "1", "True", "TRUE", "true", "OK", "ok", "Ok":
		Config.PermitWithoutStream = true
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
			return schemaentry.SerializationJSON, "", Config, 0, err
		}
		timeout = time.Duration(timeout_ms) * time.Millisecond
	}
	return serialize_protocol, path, Config, timeout, nil
}
