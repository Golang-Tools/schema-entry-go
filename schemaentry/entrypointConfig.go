package schemaentry

//EntryPointConfig 节点配置类
type EntryPointConfig struct {
	Epilog                string
	Usage                 string
	ArgparseCheckRequired bool

	DefaultConfigFilePaths []string
	ParseEnv               bool
	EnvPrefix              string

	VerifySchema bool
	Schema       string
	Config       map[string]interface{}
}
