package main

import (
	"context"
	"net/url"
	"os"

	log "github.com/Golang-Tools/loggerhelper/v2"
	s "github.com/Golang-Tools/schema-entry-go/v2"
	jsoniter "github.com/json-iterator/go"
	clientv3 "go.etcd.io/etcd/client/v3"
)

var json = jsoniter.ConfigCompatibleWithStandardLibrary

func main() {
	etcdurl := "etcd://localhost:12379/foo/bar?serialize=JSON&auto-sync-interval-ms=100"
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

	content, err := json.MarshalToString(map[string]interface{}{"WatchValue": 203})
	if err != nil {
		log.Error("json.MarshalToString wrong", log.Dict{"err": err.Error()})
		os.Exit(1)
	}
	resp, err := cli.Put(ctx, path, content)
	if err != nil {
		log.Error("cli.Put wrong", log.Dict{"err": err.Error()})
		os.Exit(1)
	}
	log.Info("put result", log.Dict{"resp": resp})
}
