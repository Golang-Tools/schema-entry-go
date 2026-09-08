package schemaentry

import (
	"net/url"
	"reflect"
	"testing"
)

func TestParseFSPath(t *testing.T) {
	cases := []struct {
		path string
		want SupportedSerialization
		err  bool
	}{
		{"a.json", SerializationJSON, false},
		{"a.yml", SerializationYAML, false},
		{"a.yaml", SerializationYAML, false},
		{"/x/y/b.json", SerializationJSON, false},
		{"a.txt", 0, true},
	}
	for _, c := range cases {
		got, _, err := ParseFSPath(c.path)
		if c.err {
			if err == nil {
				t.Fatalf("ParseFSPath(%q):期望出错,实际无错(%v)", c.path, got)
			}
			continue
		}
		if err != nil {
			t.Fatalf("ParseFSPath(%q):%v", c.path, err)
		}
		if got != c.want {
			t.Fatalf("ParseFSPath(%q):期望 %v,实际 %v", c.path, c.want, got)
		}
	}
}

func TestParseFSUrl(t *testing.T) {
	U, err := url.Parse("file:///etc/foo/config.yml")
	if err != nil {
		t.Fatal(err)
	}
	got, path, err := ParseFSUrl(U)
	if err != nil {
		t.Fatal(err)
	}
	if got != SerializationYAML {
		t.Fatalf("期望 YAML,实际 %v", got)
	}
	if path != "/etc/foo/config.yml" {
		t.Fatalf("期望 path=/etc/foo/config.yml,实际 %q", path)
	}
}

func TestReflectFieldName(t *testing.T) {
	type sample struct {
		A     int    `json:"aa"`
		B     string `json:"-"`
		NoTag string
	}
	typ := reflect.TypeOf(sample{})
	f0, _ := typ.FieldByName("A")
	f1, _ := typ.FieldByName("B")
	f2, _ := typ.FieldByName("NoTag")
	if got := ReflectFieldName(f0); got != "aa" {
		t.Fatalf("期望 aa,实际 %q", got)
	}
	if got := ReflectFieldName(f1); got != "-" {
		t.Fatalf("期望 -(json tag),实际 %q", got)
	}
	if got := ReflectFieldName(f2); got != "NoTag" {
		t.Fatalf("期望 NoTag(无 tag 用字段名),实际 %q", got)
	}
}
