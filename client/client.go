package main

import (
	"log"
	"os"
	"proxy/proxy"
)

func main() {
	config, err := proxy.ParseConfig("config.json")
	if err != nil {

	}
	for _, conf := range config.Pipe {
		_, err := proxy.NewTCPClient(conf, 0)
		if err != nil {
			log.Println(err.Error())
			os.Exit(1)
		}
	}

	proxy.DialSocks(proxy.RoleClient)
}
