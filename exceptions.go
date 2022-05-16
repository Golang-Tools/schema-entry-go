package schemaentry

import "errors"

var (
	//ErrNotAllowSetChildToEndPoint 叶子节点无法设置子节点
	ErrNotAllowSetChildToEndPoint = errors.New("not allow set child to endpoint")
	//ErrUnsupportedSchema 不支持的的schema类型
	ErrUnsupportedSchema = errors.New("unsupported schema")

	//ErrReregistCallBack 重复注册回调函数
	ErrReregistCallBack = errors.New("not allow re-regist callback")

	//ErrUnsupportedSerialization 不支持的序列化协议
	ErrUnsupportedSerialization = errors.New("unsupported serialization protocol")
	//ErrNotSetSerialize 未设置序列化协议
	ErrNotSetSerialize = errors.New("need to set serialize")

	//ErrEtcdKeyLenNotMatch etcd的key数量不匹配
	ErrEtcdKeyLenNotMatch = errors.New("etcd key len not match")
)
