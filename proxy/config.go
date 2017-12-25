package proxy

import (
	"encoding/json"
	"io/ioutil"
	"os"
)

const (
	TypePipe = iota + 1
	TypeServer
)

type ConfigSingle struct {
	ID       int    `json:"id"`
	PID      int    `json:"pid"`
	IP       string `json:"ip"`
	Port     int    `json:"port"`
	Protocol string `json:"protocol"`
	Type     int    `json:"type"` // 1(pipe) 2(server)
}

type Config struct {
	Pipe   []*ConfigSingle `json:"pipe"`
	Server []*ConfigSingle `json:"server"`
}

var Configs *Config

func ParseConfig(path string) (config *Config, err error) {
	file, err := os.Open(path) // For read access.
	if err != nil {
		return
	}
	defer file.Close()

	data, err := ioutil.ReadAll(file)
	if err != nil {
		return
	}

	config = &Config{}
	if err = json.Unmarshal(data, config); err != nil {
		return nil, err
	}

	for _, conf := range config.Pipe {
		conf.Type = TypePipe
	}

	for _, conf := range config.Server {
		conf.Type = TypeServer
	}

	Configs = config
	return
}

func (config *Config) getServer(id int) (serverConfig *ConfigSingle) {
	for _, serverConfig := range config.Server {
		if serverConfig.ID == id {
			return serverConfig
		}
	}
	return nil
}

func (config *Config) getPipe(id int) (pipeConfig *ConfigSingle) {
	for _, pipeConfig := range config.Pipe {
		if pipeConfig.ID == id {
			return pipeConfig
		}
	}
	return nil
}
