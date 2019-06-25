package common

import (
	"os"
	"github.com/golang/glog"
	yaml "gopkg.in/yaml.v3"
)

var ProxyConfig *Config

type Config struct {
	ListenPort uint32 `yaml:"listen_port"`

	Limit struct {
		MaxGetKeys int32 `yaml:"max_get_keys"`
		MaxDelKeys int32 `yaml:"max_del_keys"`
		MaxSetKeys int32 `yaml:"max_set_keys"`
	} `yaml:"limit"`

	Tsdb struct {
		Addr string `yaml:"addr"`
		Duration uint32 `yaml:"duration_min"`
		Prefix string `yaml:"prefix"`
	} `yaml:"tsdb"`

	Server struct {
		ReadTimeout uint32 `yaml:"read_timeout_ms"`
		ReadHeaderTimeout uint32 `yaml:"read_header_timeout_ms"`
		WriteTimeout uint32 `yaml:"write_timeout_ms"`
		IdleTimeout uint32 `yaml:"idle_timeout_min"`
	} `yaml:"server"`

	Log struct {
		InfoLevel int32 `yaml:"info_level"`
		SampleRate uint32 `yaml:"sample_rate"`
	} `yaml:"log"`

	Tikv struct {
		PdAddrs []string `yaml:"pd_addrs"`
		ConnTimeout uint32 `yaml:"conn_timeout_ms"`
	} `yaml:"tikv"`
}

func (c *Config) LoadFrom(confPath string) (error) {
	//打开文件
	filePtr, err := os.Open(confPath)
	if err != nil {
		return err
	}
	defer filePtr.Close()

	decoder := yaml.NewDecoder(filePtr)
	ProxyConfig = &Config{}
	err = decoder.Decode(ProxyConfig)
	glog.V(16).Infof("config: %+v", *ProxyConfig)
	return err
}

func (c *Config) Check() bool {
	if c.ListenPort == 0 {
		return false
	}
	return true
}