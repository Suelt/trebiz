package main

import (
	"fmt"
	"time"

	"github.com/treble-h/trebiz/config"
	"github.com/treble-h/trebiz/core"
)

func main() {

	conf, cerr := config.LoadConfig("", "config")
	if cerr != nil {
		panic(cerr)
	}
	fmt.Println("LoadConfig:")
	fmt.Printf("ReplicaID:%d\n", conf.ReplicaId)
	switch conf.NodeType {
	case 0:
		fmt.Println("Type:correct node")
		break
	case 1:
		fmt.Println("Type:Byzantine fault")
		break
	case 2:
		fmt.Println("Type:Active Byzantine Merchant")
		break
	case 3:
		fmt.Println("Type:Passive Byzantine Merchant")
		break
	default:
		fmt.Println("Error!This type is not defined!")
		break
	}
	fmt.Printf("IP:%s\n", conf.AddrStr)
	fmt.Printf("clusterAddr:%v\n", conf.ClusterAddr)

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

	go node.HandleReqBatchLoop()

	node.StartPBFT(errchan)

}
