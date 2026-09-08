package etcd

import (
	"net/url"
	"testing"

	schemaentry "github.com/Golang-Tools/schema-entry-go/v4"
)

func TestParseEtcdUrl(t *testing.T) {
	etcdurl := "etcd://user:pwd@localhost:12379/foo/bar?serialize=YAML&address=192.168.1.1:4324&dial-timeout-ms=500&query-timeout-ms=200"
	U, err := url.Parse(etcdurl)
	if err != nil {
		t.Fatal(err)
	}
	got, path, config, timeout, err := ParseEtcdUrl(U)
	if err != nil {
		t.Fatal(err)
	}
	if got != schemaentry.SerializationYAML {
		t.Fatalf("期望 YAML,实际 %v", got)
	}
	if path != "/foo/bar" {
		t.Fatalf("期望 path=/foo/bar,实际 %q", path)
	}
	if len(config.Endpoints) != 2 {
		t.Fatalf("期望 2 个 endpoints,实际 %v", config.Endpoints)
	}
	if config.Username != "user" || config.Password != "pwd" {
		t.Fatalf("期望 user/pwd,实际 %q/%q", config.Username, config.Password)
	}
	if config.DialTimeout.Milliseconds() != 500 {
		t.Fatalf("期望 dial-timeout 500ms,实际 %v", config.DialTimeout)
	}
	if timeout.Milliseconds() != 200 {
		t.Fatalf("期望 query-timeout 200ms,实际 %v", timeout)
	}
}
