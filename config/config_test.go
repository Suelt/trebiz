package config

import (
	"fmt"
	"testing"
)

func TestConfigRead(t *testing.T) {
	config, err := LoadConfig("", "testnode0")
	if err != nil {
		t.Fatal(err)
	}

	fmt.Println("id", config.ReplicaId)
	fmt.Println("address:", config.AddrStr)
	fmt.Println("clusterPort:", config.ClusterPort)
	fmt.Println("clusterAddr:", config.ClusterAddr)
	fmt.Println("P2PListenPort:", config.P2PListenPort)
	fmt.Println("RPCListenPort:", config.RPCListenPort)

	fmt.Println("BatchTimeout:", config.BatchTimeout)
	fmt.Println("BatchSize:", config.BatchSize)
	fmt.Println("CheckPointT:", config.CheckPointT)
	fmt.Println("LogL:", config.LogK)
}
