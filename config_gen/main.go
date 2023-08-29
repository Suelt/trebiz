package main

import (
	"encoding/hex"
	"fmt"
	"math/rand"
	"sort"
	"strconv"
	"time"

	"github.com/spf13/viper"
	"github.com/treble-h/trebiz/sign"
)

func judgeNodeType(i int, b []int) bool {
	for _, v := range b {
		if i == v {
			return true
		}
	}
	return false
}

func generateRandomNumber(start int, end int, count int) []int {

	if end < start || (end-start) < count {
		return nil
	}

	nums := make([]int, 0)

	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	for len(nums) < count {

		num := r.Intn((end - start)) + start

		exist := false
		for _, v := range nums {
			if v == num {
				exist = true
				break
			}
		}

		if !exist {
			nums = append(nums, num)
		}
	}
	return nums
}

func main() {

	viperRead := viper.New()

	viperRead.SetConfigName("config_template") // name of config file (without extension)
	viperRead.AddConfigPath(".")               // path to look for the config file in
	err := viperRead.ReadInConfig()            // Find and read the config file
	if err != nil {                            // Handle errors reading the config file
		panic(fmt.Errorf("Fatal error config file: %s \n", err))
	}

	clusterInnerAddr := viperRead.GetStringMap("ips")
	ProcessCount := viperRead.GetInt("processcount")
	//IP
	tempClusterMapInterface := viperRead.GetStringMap("ips")
	clusterMapInterface := make(map[string]string)
	for name, addr := range tempClusterMapInterface {
		rs := []rune(name)
		ipIndex, _ := strconv.Atoi(string(rs[4:]))
		if addrAsString, ok := addr.(string); ok {

			for j := 0; j < ProcessCount; j++ {
				if ipIndex == 0 {
					suScript := strconv.Itoa(0)
					clusterMapInterface["node"+suScript] = addrAsString
					break
				}
				suScript := strconv.Itoa((ipIndex-1)*ProcessCount + j + 1)
				clusterMapInterface["node"+suScript] = addrAsString
			}

		} else {
			panic("cluster in the config file cannot be decoded correctly")
		}
	}

	nodeNumber := len(clusterInnerAddr)
	clusterMapString := make(map[string]string, nodeNumber)

	clusterName := make([]string, nodeNumber)
	sort.Strings(clusterName)
	i := 0
	for name, addr := range clusterInnerAddr {
		if addrAsString, ok := addr.(string); ok {
			clusterMapString[name] = addrAsString
			clusterName[i] = name
			i++
		} else {
			panic("cluster in the config file cannot be decoded correctly")
		}
	}

	tempP2pPortMapInterface := viperRead.GetStringMap("peers_p2p_port")
	if nodeNumber != len(tempP2pPortMapInterface) {
		panic("p2p_listen_port does not match with cluster")
	}
	//handle listen port
	p2pPortMapInterface := make(map[string]int)

	mapNameToP2PPort := make(map[string]int, nodeNumber)
	for name, _ := range clusterMapString {
		portAsInterface, ok := tempP2pPortMapInterface[name]
		if !ok {
			panic("p2p_listen_port does not match with cluster")
		}
		if portAsInt, ok := portAsInterface.(int); ok {

			mapNameToP2PPort[name] = portAsInt
			rs := []rune(name)
			ipIndex, _ := strconv.Atoi(string(rs[4:]))
			for j := 0; j < ProcessCount; j++ {
				if ipIndex == 0 {
					subScript := strconv.Itoa(0)
					p2pPortMapInterface["node"+subScript] = portAsInt + j*10
					break
				}
				subScript := strconv.Itoa((ipIndex-1)*ProcessCount + j + 1)
				p2pPortMapInterface["node"+subScript] = portAsInt + j*10
			}

		} else {
			panic("p2p_listen_port contains a non-int value")
		}
	}

	//generate ed keys,map name to key
	privateKeysRsa := make(map[string]string)
	publicKeysRsa := make(map[string]string)

	for i := 0; i < nodeNumber; i++ {
		for j := 0; j < ProcessCount; j++ {
			privateKey, publicKey, err := sign.GenKeys()
			if err != nil {
				panic(err)
			}
			if i == 0 {
				subScript := strconv.Itoa(0)
				publicKeysRsa["node"+subScript] = hex.EncodeToString(publicKey)
				privateKeysRsa["node"+subScript] = hex.EncodeToString(privateKey)
				break
			}
			subScript := strconv.Itoa((i-1)*ProcessCount + j + 1)
			publicKeysRsa["node"+subScript] = hex.EncodeToString(publicKey)
			privateKeysRsa["node"+subScript] = hex.EncodeToString(privateKey)
		}
	}

	simLatencyMap := make(map[string]float64)
	simLatencyInterface := viperRead.GetStringMap("sim_latency")
	for name, latency := range simLatencyInterface {
		if latencyAsFloat, ok := latency.(float64); ok {
			simLatencyMap[name] = latencyAsFloat
		} else {
			panic("sim_latency contains a non-float value")
		}
	}
	var latencySum float64
	for _, value := range simLatencyMap {

		latencySum += value
	}

	latencySum /= float64(3*ProcessCount + 1)

	latency_mean := viperRead.GetFloat64("latency_mean")
	for name, value := range simLatencyMap {
		simLatencyMap[name] = value * latency_mean / latencySum
	}

	//generate threshold keys
	TotalNodeNum := (nodeNumber-1)*ProcessCount + 1
	numT := TotalNodeNum - TotalNodeNum/3 - 1
	shares, pubPoly := sign.GenTSKeys(numT, TotalNodeNum)

	bg := viperRead.GetInt("bgnum")

	viewChangeQuorum := 2*(TotalNodeNum/3) + 1

	rpcListenPort := viperRead.GetInt("rpc_listen_port")

	evilNode := generateRandomNumber(1, TotalNodeNum, bg)

	fmt.Println("EVILNODES", evilNode)

	bgnodes := evilNode[0:bg]
	//abmnodes := evilNode[bg : bg+abm]
	//pbmodes := evilNode[bg+abm:]

	fmt.Println("bgnodes", bgnodes)
	// fmt.Println("abmnodes", abmnodes)
	// fmt.Println("pbmodes", pbmodes)

	for _, name := range clusterName {

		viperWrite := viper.New()
		for j := 0; j < ProcessCount; j++ {
			index := strconv.Itoa(j)

			rs := []rune(name)

			ipIndex, err := strconv.Atoi(string(rs[4:]))

			if err != nil {
				panic("get replicaid failed")
			}

			var replicaId int

			if ipIndex == 0 {
				replicaId = 0
			} else {
				//calculate ID
				replicaId = (ipIndex-1)*ProcessCount + j + 1
			}

			viperWrite.SetConfigFile(fmt.Sprintf("%s_%s.yaml", name, index))
			viperWrite.Set("sim_latency", simLatencyMap["node"+fmt.Sprintf("%d", replicaId)])
			shareAsBytes, err := sign.EncodeTSPartialKey(shares[replicaId])
			if err != nil {
				panic("encode the share")
			}

			tsPubKeyAsBytes, err := sign.EncodeTSPublicKey(pubPoly)
			if err != nil {
				panic("encode the share")
			}

			viperWrite.Set("name", "node"+strconv.Itoa(replicaId))
			viperWrite.Set("replicaId", replicaId)

			viperWrite.Set("address", clusterMapString[name])

			viperWrite.Set("p2p_listen_port", mapNameToP2PPort[name]+j*10)

			viperWrite.Set("peers_p2p_port", p2pPortMapInterface)

			viperWrite.Set("rpc_listen_port", rpcListenPort+j)

			viperWrite.Set("cluster_ips", clusterMapInterface)

			viperWrite.Set("ed_prikey", privateKeysRsa["node"+strconv.Itoa(replicaId)])
			viperWrite.Set("cluster_ed_pubkey", publicKeysRsa)

			viperWrite.Set("tsShare", hex.EncodeToString(shareAsBytes))
			viperWrite.Set("tsPubKey", hex.EncodeToString(tsPubKeyAsBytes))

			// viperWrite.Set("fasttsShare", hex.EncodeToString(fastShareAsBytes))
			// viperWrite.Set("fasttsPubKey", hex.EncodeToString(fastTsPubKeyAsBytes))

			viperWrite.Set("batchtimeout", viperRead.GetInt("batchtimeout"))
			viperWrite.Set("viewchangetimeout", viperRead.GetInt("viewchangetimeout"))
			viperWrite.Set("batchsize", viperRead.GetInt("batchsize"))
			viperWrite.Set("checkPoint_t", viperRead.GetInt("checkPoint_t"))
			viperWrite.Set("log_k", viperRead.GetInt("log_k"))
			viperWrite.Set("maxpool", viperRead.GetInt("maxpool"))
			viperWrite.Set("autoviewchange", viperRead.GetInt("autoviewchange"))

			//viperWrite.Set("fastqcquorum", fastNum)
			viperWrite.Set("viewChangeQuorum", viewChangeQuorum)
			viperWrite.Set("commitquorum", 2*ProcessCount)
			viperWrite.Set("prepquorum", 2*ProcessCount+1)

			// viperWrite.Set("prePrepareSubsetCount", prePrepareSubsetCount)
			if judgeNodeType(replicaId, bgnodes) {
				viperWrite.Set("nodetype", 1)
			} else {
				viperWrite.Set("nodetype", 0)
			}

			err = viperWrite.WriteConfig()
			if err != nil {
				return
			}
			if ipIndex == 0 {
				break
			}
		}
	}
}
