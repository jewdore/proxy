package main

import (
	"log"
	"os"
	"github.com/prosejazz/proxy/proxy"
)

func main() {
	config, err := proxy.ParseConfig("config.json")
	if err != nil {
		log.Println(err.Error())
		os.Exit(1)
	}
	for _, conf := range config.Pipe {
		tcp, err := proxy.NewTCPListener(conf)
		if err != nil {
			log.Println(err.Error())
			os.Exit(1)
		}
		go tcp.Accept()
	}

	for _, conf := range config.Server {
		tcp, err := proxy.NewTCPListener(conf)
		if err != nil {
			log.Println(err.Error())
			os.Exit(1)
		}
		go tcp.Accept()
	}
	proxy.DialSocks(proxy.RoleServer)
}
