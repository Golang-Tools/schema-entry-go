package schemaentry

import (
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"reflect"
	"strconv"
	"strings"
	"unicode"

	"github.com/akamensky/argparse"
	jsoniter "github.com/json-iterator/go"
)

var json = jsoniter.ConfigCompatibleWithStandardLibrary

//EntryPointNode 节点接口
type EntryPointNode interface {
	Meta() *EntryPointMeta
	IsRoot() bool
	IsEndpoint() bool
	SetChild(EntryPointNode)
	SetParent(EntryPointNode)
	Parse([]string)
	PassArgsTosub(*argparse.Parser, []string)
}

//EntryPointConfig 节点配置
type EntryPointConfig interface {
	Schema() string
	Main()
}

//EntryPoint 节点类
type EntryPoint struct {
	*EntryPointMeta
	*EntryPointOption
	Config EntryPointConfig
}

//New 创建一个节点对象
//@param config interface{} 为一个定义好的struct的空对象,中间节点可以为nil
//@param meta *EntryPointMeta 为节点的元信息
//@param opt *EntryPointOption 为对解析行为的设置.可以不传表示使用默认设置
func New(meta *EntryPointMeta, config EntryPointConfig, opt ...*EntryPointOption) (*EntryPoint, error) {
	ep := new(EntryPoint)
	m := EntryPointMeta{}
	if meta != nil {
		if meta.Name != "" {
			m.Name = meta.Name
		} else {
			if config != nil {
				v := reflect.ValueOf(config)
				namel := strings.Split(v.Type().String(), ".")
				m.Name = strings.ToLower(namel[len(namel)-1])
			} else {
				return nil, errors.New("请设置meta中的name字段")
			}
		}
		m.DefaultConfigFilePaths = meta.DefaultConfigFilePaths
		m.ParseEnv = meta.ParseEnv
		m.EnvPrefix = meta.EnvPrefix
		m.Usage = meta.Usage
	}
	ep.EntryPointMeta = &m

	var option *EntryPointOption
	optlen := len(opt)
	switch optlen {
	case 0:
		{
			option = &EntryPointOption{}
		}
	case 1:
		{
			option = opt[0]
		}
	default:
		{
			return nil, errors.New("最多只能设置一种解析行为")
		}
	}
	ep.EntryPointOption = option
	ep.Config = config
	return ep, nil
}

//PassArgsTosub 将解析传导给下级
func (ep EntryPoint) PassArgsTosub(parser *argparse.Parser, argv []string) {
	if len(argv) <= 1 {
		parser.HelpFunc = func(c *argparse.Command, msg interface{}) string {
			var help string
			help += fmt.Sprintf("命令: %s <subcmd>\n", c.GetName())
			help += fmt.Sprintf("说明:\n")
			help += fmt.Sprintf("%s\n", c.GetDescription())
			help += fmt.Sprintf("支持的子命令:\n")
			for subcmd, subnode := range ep.subcmds {
				help += fmt.Sprintf(" %s %s\n", subcmd, subnode.Meta().Usage)
			}
			return help
		}
		fmt.Print(parser.Help(nil))
		os.Exit(0)
	}
	scmds := []string{}
	insubcmd := false
	for subcmd, _ := range ep.EntryPointMeta.subcmds {
		if argv[1] == subcmd {
			insubcmd = true
			break
		} else {
			scmds = append(scmds, subcmd)

		}
	}
	if insubcmd {
		args := []string{argv[0] + " " + argv[1]}
		for _, value := range argv[2:] {
			args = append(args, value)
		}
		ep.EntryPointMeta.subcmds[argv[1]].Parse(args)
	} else {
		parser.HelpFunc = func(c *argparse.Command, msg interface{}) string {
			var help string
			help += fmt.Sprintf("未知的子命令`%s`\n", argv[1])
			help += fmt.Sprintf("命令: %s <subcmd>\n", c.GetName())
			help += fmt.Sprintf("说明:\n")
			help += fmt.Sprintf("%s\n", c.GetDescription())
			help += fmt.Sprintf("支持的子命令:\n")
			for subcmd, subnode := range ep.subcmds {
				help += fmt.Sprintf(" %s %s\n", subcmd, subnode.Meta().Usage)
			}
			return help
		}
		fmt.Print(parser.Help(nil))
		os.Exit(1)
	}
}

func loadConfigFile(filename string, conf map[string]interface{}) (bool, string, error) {
	info, err := os.Stat(filename)
	var conftype string
	if err != nil {
		if os.IsNotExist(err) {
			return false, "", fmt.Errorf("find file %s not exist", filename)
		} else {
			return false, "", fmt.Errorf("find file %s error: %s", filename, err)
		}
	} else {
		if info.IsDir() {
			return false, "", fmt.Errorf("find %s is a dir", filename)
		} else {
			f, err := os.Open(filename)
			if err != nil {
				return true, "", err
			}
			defer f.Close()
			fd, err := ioutil.ReadAll(f)
			if err != nil {
				return true, "", err
			}
			// if strings.HasSuffix(filename, "json") {
			conftype = "json"
			err = json.Unmarshal(fd, &conf)
			if err != nil {
				return true, "", err
			}
			// } else {
			// 	// conftype = "yaml"
			// 	// err := yaml.Unmarshal(fd, &conf)
			// 	// if err != nil {
			// 	// 	return true, "", err
			// 	// }

			// }
			return true, conftype, nil
		}
	}
}

//ParseStruct 解析结构,构造命令行参数解析和环境变量解析
func (ep *EntryPoint) ParseStruct(parser *argparse.Parser, argv []string) (map[string]interface{}, map[string]interface{}, error) {
	if ep.Config != nil {
		var conffilepath []string
		fileConfig := map[string]interface{}{}
		var fileConfigType string

		envConf := map[string]interface{}{}
		flagConfptr := map[string]interface{}{}
		flagConf := map[string]interface{}{}
		flagFileConf := map[string]interface{}{}
		var flagFileConfType string
		if ep.DefaultConfigFilePaths == nil {
			configFileName := strings.Join(GetNodeProgList(ep), "_")
			homepath, err := os.UserHomeDir()
			if err != nil {
				fmt.Println("find home error")
				conffilepath = []string{
					fmt.Sprintf("./%s.json", configFileName),
					fmt.Sprintf("/%s/config.json", configFileName),
				}
			} else {
				conffilepath = []string{
					fmt.Sprintf("./%s.json", configFileName),
					fmt.Sprintf("%s/%s/config.json", homepath, configFileName),
					fmt.Sprintf("/%s/config.json", configFileName),
				}
			}

		} else {
			conffilepath = ep.DefaultConfigFilePaths
		}
		for _, filename := range conffilepath {
			stop, ct, err := loadConfigFile(filename, fileConfig)
			if stop {
				if err != nil {
					return nil, nil, err
				}
				fileConfigType = ct
				break
			} else {
				if err != nil {
					fmt.Println(err.Error())
				}
			}
		}
		t := reflect.TypeOf(ep.Config)
		argconfigfilepath := parser.String("c", "config_path", &argparse.Options{Required: false, Help: "指定配置文件位置"})
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			if unicode.IsLower([]rune(f.Name)[0]) || f.Tag.Get("json") == "-" {
				continue
			}
			getenvstr := ""
			if ep.Meta().ParseEnv {
				if ep.Meta().EnvPrefix != "" {
					getenvstr = os.Getenv(fmt.Sprintf("%s_%s", strings.ToUpper(ep.Meta().EnvPrefix), strings.ToUpper(f.Name)))
				} else {
					EnvPrefix := strings.Join(GetNodeProgList(ep), "_")
					getenvstr = os.Getenv(fmt.Sprintf("%s_%s", strings.ToUpper(EnvPrefix), strings.ToUpper(f.Name)))
				}
			}
			switch f.Type.Kind() {
			case reflect.String:
				{
					flagConfptr[f.Name] = parser.String("", f.Name, &argparse.Options{Required: false})
					if getenvstr != "" {
						envConf[f.Name] = getenvstr
					}
				}
			case reflect.Bool:
				{
					flagConfptr[f.Name] = parser.Flag("", f.Name, &argparse.Options{Required: false})
					if getenvstr != "" {
						if strings.ToUpper(getenvstr) == "TRUE" {
							envConf[f.Name] = true
						} else {
							envConf[f.Name] = false
						}

					}
				}
			case reflect.Int:
				{
					flagConfptr[f.Name] = parser.Int("", f.Name, &argparse.Options{Required: false})
					if getenvstr != "" {
						intv, err := strconv.Atoi(getenvstr)
						if err != nil {
							return nil, nil, err
						}
						envConf[f.Name] = intv
					}
				}

			case reflect.Float64:
				{
					flagConfptr[f.Name] = parser.Float("", f.Name, &argparse.Options{Required: false})
					if getenvstr != "" {
						fv, err := strconv.ParseFloat(getenvstr, 64)
						if err != nil {
							return nil, nil, err
						}
						envConf[f.Name] = fv
					}
				}
			case reflect.Slice:
				{
					switch f.Type.String() {
					case "[]string":
						{
							flagConfptr[f.Name] = parser.StringList("", f.Name, &argparse.Options{Required: false})
							if getenvstr != "" {
								envConf[f.Name] = strings.Split(getenvstr, ",")
							}
						}
					case "[]int":
						{
							flagConfptr[f.Name] = parser.IntList("", f.Name, &argparse.Options{Required: false})
							if getenvstr != "" {
								r := []int{}
								for _, ele := range strings.Split(getenvstr, ",") {
									intv, err := strconv.Atoi(ele)
									if err != nil {
										return nil, nil, err
									}
									r = append(r, intv)
								}
								envConf[f.Name] = r
							}
						}
					case "[]float64":
						{
							flagConfptr[f.Name] = parser.FloatList("", f.Name, &argparse.Options{Required: false})
							if getenvstr != "" {
								r := []float64{}
								for _, ele := range strings.Split(getenvstr, ",") {
									fv, err := strconv.ParseFloat(getenvstr, 64)
									if err != nil {
										return nil, nil, err
									}
									r = append(r, fv)
								}
								envConf[f.Name] = r
							}
						}
					default:
						{
							return nil, nil, fmt.Errorf("字段%s是未支持的类型%v", f.Name, f.Type)
						}
					}
				}

			default:
				{
					return nil, nil, fmt.Errorf("字段%s是未支持的类型%v", f.Name, f.Type)
				}
			}
		}
		err := parser.Parse(argv)
		if err != nil {
			// In case of error print error and print usage
			// This can also be done by passing -h or --help flags
			fmt.Print(parser.Usage(err))
			return nil, nil, err
		}
		if *argconfigfilepath != "" {
			_, cft, err := loadConfigFile(*argconfigfilepath, flagFileConf)
			if err != nil {
				return nil, nil, err
			}
			flagFileConfType = cft
		}

		//设置参数
		v := reflect.ValueOf(ep.Config)
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			if unicode.IsLower([]rune(f.Name)[0]) || f.Tag.Get("json") == "-" {
				continue
			}
			vf := v.Field(i)

			switch f.Type.Kind() {
			case reflect.String:
				{
					//设置文件配置
					if fileConfigType != "" {
						stags := f.Tag.Get(fileConfigType)
						infilename := strings.Split(stags, ",")[0]
						if infilename == "-" || infilename == "" {

						}
					}
					if v, ok := data[key]; ok {
						f.Set(reflect.ValueOf(v))
					} else {
						panic(t.Name + " not found")
					}
					//设置环境变量配置
					flagConfptr[f.Name] = parser.String("", f.Name, &argparse.Options{Required: false})
					if getenvstr != "" {
						envConf[f.Name] = getenvstr
					}
					//设置指定的文件配置

					//设置命令行配置
				}
			case reflect.Bool:
				{
					flagConfptr[f.Name] = parser.Flag("", f.Name, &argparse.Options{Required: false})
					if getenvstr != "" {
						if strings.ToUpper(getenvstr) == "TRUE" {
							envConf[f.Name] = true
						} else {
							envConf[f.Name] = false
						}

					}
				}
			case reflect.Int:
				{
					flagConfptr[f.Name] = parser.Int("", f.Name, &argparse.Options{Required: false})
					if getenvstr != "" {
						intv, err := strconv.Atoi(getenvstr)
						if err != nil {
							return nil, nil, err
						}
						envConf[f.Name] = intv
					}
				}

			case reflect.Float64:
				{
					flagConfptr[f.Name] = parser.Float("", f.Name, &argparse.Options{Required: false})
					if getenvstr != "" {
						fv, err := strconv.ParseFloat(getenvstr, 64)
						if err != nil {
							return nil, nil, err
						}
						envConf[f.Name] = fv
					}
				}
			case reflect.Slice:
				{
					switch f.Type.String() {
					case "[]string":
						{
							flagConfptr[f.Name] = parser.StringList("", f.Name, &argparse.Options{Required: false})
							if getenvstr != "" {
								envConf[f.Name] = strings.Split(getenvstr, ",")
							}
						}
					case "[]int":
						{
							flagConfptr[f.Name] = parser.IntList("", f.Name, &argparse.Options{Required: false})
							if getenvstr != "" {
								r := []int{}
								for _, ele := range strings.Split(getenvstr, ",") {
									intv, err := strconv.Atoi(ele)
									if err != nil {
										return nil, nil, err
									}
									r = append(r, intv)
								}
								envConf[f.Name] = r
							}
						}
					case "[]float64":
						{
							flagConfptr[f.Name] = parser.FloatList("", f.Name, &argparse.Options{Required: false})
							if getenvstr != "" {
								r := []float64{}
								for _, ele := range strings.Split(getenvstr, ",") {
									fv, err := strconv.ParseFloat(getenvstr, 64)
									if err != nil {
										return nil, nil, err
									}
									r = append(r, fv)
								}
								envConf[f.Name] = r
							}
						}
					default:
						{
							return nil, nil, fmt.Errorf("字段%s是未支持的类型%v", f.Name, f.Type)
						}
					}
				}

			default:
				{
					return nil, nil, fmt.Errorf("字段%s是未支持的类型%v", f.Name, f.Type)
				}
			}
			val := v.Field(i).Interface()
			fmt.Printf("%6s: %v = %v\n", f.Name, f.Type, val)
		}

	} else {
		return nil, nil, errors.New("节点没有配置项")
	}
}

//PassArgs 解析根节点
func (ep EntryPoint) PassArgs(parser *argparse.Parser, argv []string) EntryPointConfig {
	parser.String("s", "string", &argparse.Options{Help: "String argument example"})
	// Create string flag
	parser.Int("i", "int", &argparse.Options{Required: true, Help: "Integer argument example"})
	err := parser.Parse(argv)
	if err != nil {
		// In case of error print error and print usage
		// This can also be done by passing -h or --help flags
		fmt.Print(parser.Usage(err))
	}

}

//Parse 解析节点
func (ep EntryPoint) Parse(argv []string) {
	prog := GetNodeProg(ep)
	parser := argparse.NewParser(prog, ep.EntryPointMeta.Usage)
	if ep.IsEndpoint() {
		ep.PassArgs(parser, argv)
	} else {
		ep.PassArgsTosub(parser, argv)
	}
}
func (ep EntryPoint) runMain() {
	ep.Config.Main()
}
