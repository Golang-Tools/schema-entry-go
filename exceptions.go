package schemaentry

import "errors"

var (
	// ErrNotAllowSetChildToEndPoint 叶子节点无法设置子节点
	ErrNotAllowSetChildToEndPoint = errors.New("not allow set child to endpoint")
	// ErrUnsupportedSchema 不支持的的schema类型
	ErrUnsupportedSchema = errors.New("unsupported schema")

	// ErrReregistCallBack 重复注册回调函数
	ErrReregistCallBack = errors.New("not allow re-regist callback")

	// ErrUnsupportedSerialization 不支持的序列化协议
	ErrUnsupportedSerialization = errors.New("unsupported serialization protocol")
	// ErrNotSetSerialize 未设置序列化协议
	ErrNotSetSerialize = errors.New("need to set serialize")

	// ErrEtcdKeyLenNotMatch etcd的key数量不匹配
	ErrEtcdKeyLenNotMatch = errors.New("etcd key len not match")

	// ErrHelp 请求帮助(-h/--help)时返回,由调用方决定退出码(通常为 0)
	ErrHelp = errors.New("help requested")
	// ErrWatchOnRefreshNotSet watchmode 下未注册 OnRefresh 回调
	ErrWatchOnRefreshNotSet = errors.New("watchmode need to set OnRefresh first")
)

// UsageError 附带命令用法文本的错误,便于调用方直接打印后再自行决定退出码
// 请求帮助时返回 UsageError{Err: ErrHelp},调用方可用 errors.Is(err, ErrHelp) 判断
// 并选择退出码(帮助通常为 0,其它解析错误为 1)。
type UsageError struct {
	Usage string
	Err   error
}

// Error 返回错误信息并附带用法文本
func (e *UsageError) Error() string {
	if e == nil {
		return ""
	}
	if e.Err == nil {
		return e.Usage
	}
	if e.Usage == "" {
		return e.Err.Error()
	}
	return e.Err.Error() + "\n\n" + e.Usage
}

// Unwrap 解包出被包装的错误
func (e *UsageError) Unwrap() error {
	return e.Err
}
