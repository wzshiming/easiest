package main

import (
	"context"
	"flag"
	"log"
	"os"
	"strings"

	"github.com/wzshiming/easiest"
)

var (
	rule = "localhost=example.org:80"
	dir  = ""
)

func init() {
	flag.StringVar(&rule, "r", rule, "mapping rules")
	flag.StringVar(&dir, "d", dir, "tls dir")
	flag.Parse()
}

func main() {
	logger := log.New(os.Stderr, "[easiest] ", log.LstdFlags)
	r := map[string]string{}
	for _, o := range strings.Split(rule, ",") {
		l := strings.SplitN(o, "=", 3)
		if len(l) >= 2 {
			k := strings.TrimSpace(l[0])
			v := strings.TrimSpace(l[1])
			r[k] = v
			logger.Println(k, "->", v)
		}
	}

	server := easiest.NewServer(r, "", logger)

	err := server.Run(context.Background())
	if err != nil {
		logger.Println("run", err)
	}
}
