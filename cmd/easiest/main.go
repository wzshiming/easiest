package main

import (
	"context"
	"flag"
	"github.com/wzshiming/easiest"
	yaml "gopkg.in/yaml.v3"
	"log"
	"os"
)

var (
	config = ""
	dir    = ""
)

func init() {
	flag.StringVar(&config, "c", config, "route config")
	flag.StringVar(&dir, "d", dir, "tls dir")
	flag.Parse()
}

func main() {
	logger := log.New(os.Stderr, "[easiest] ", log.LstdFlags)

	data, err := os.ReadFile(config)
	if err != nil {
		logger.Println("read config: ", err)
		os.Exit(1)
	}
	var conf easiest.Config
	err = yaml.Unmarshal(data, &conf)
	if err != nil {
		logger.Println("unmarshal config: ", err)
		os.Exit(1)
	}
	data, _ = yaml.Marshal(conf)
	os.Stderr.Write(data)

	server := easiest.NewServer(conf, logger)

	err = server.Run(context.Background())
	if err != nil {
		logger.Println("run", err)
		os.Exit(1)
	}
}
