// 命令 seed 演示往 etcd 写入一份配置,配合 example/watch 联调 etcd 监听:
//
//	go run ./example/watch "etcd://localhost:12379/foo/bar?serialize=JSON"
//	go run ./example/seed
//
// seed 会把 {"WatchValue":203} 写入 etcd 的 /foo/bar 路径(或命令行指定的其它 url)。
package main

import (
	"context"
	"encoding/json"
	"net/url"
	"os"

	log "github.com/Golang-Tools/loggerhelper/v3"
	s "github.com/Golang-Tools/schema-entry-go/v3"
	clientv3 "go.etcd.io/etcd/client/v3"
)

func main() {
	etcdurl := "etcd://localhost:12379/foo/bar?serialize=JSON&auto-sync-interval-ms=100"
	if len(os.Args) > 1 {
		etcdurl = os.Args[1]
	}
	U, err := url.Parse(etcdurl)
	if err != nil {
		log.Error("get err", log.Dict{"err": err.Error()})
		os.Exit(1)
	}
	_, path, config, timeout, err := s.ParseEtcdUrl(U)
	if err != nil {
		log.Error("Parse URL wrong", log.Dict{"err": err.Error()})
		os.Exit(1)
	}
	cli, err := clientv3.New(config)
	if err != nil {
		log.Error("clientv3.New wrong", log.Dict{"err": err.Error()})
		os.Exit(1)
	}
	defer cli.Close()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	content, err := json.Marshal(map[string]interface{}{"WatchValue": 203})
	if err != nil {
		log.Error("json.Marshal wrong", log.Dict{"err": err.Error()})
		os.Exit(1)
	}
	resp, err := cli.Put(ctx, path, string(content))
	if err != nil {
		log.Error("cli.Put wrong", log.Dict{"err": err.Error()})
		os.Exit(1)
	}
	log.Info("put result", log.Dict{"resp": resp})
}
