package main

import (
	"fmt"
	"github.com/seafooler/trebiz/config"
	"github.com/seafooler/trebiz/core"
	"time"
)

func main() {

	conf, cerr := config.LoadConfig("", "config")
	if cerr != nil {
		panic(cerr)
	}

	fmt.Println("LoadConfig:")
	fmt.Printf("%v\n", *conf)

	var node *core.Node
	node = core.NewNode(conf)

	errchan := make(chan error, 1)

	var err error

	if err = node.StartListen(); err != nil {
		panic(err)
	}

	// wait for each node to start
	time.Sleep(time.Second * 30)

	if err = node.StartConnect(); err != nil {
		panic(err)
	}

	if err = node.StartRPCListen(); err != nil {
		panic(err)
	}

	time.Sleep(time.Second * 2)

	if conf.ReplicaId == 0 {
		go node.HandleReqBatchLoop()
	}
	node.StartPBFT(errchan)

}
