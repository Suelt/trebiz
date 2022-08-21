package config

import (
	"crypto/ed25519"
	"encoding/hex"
	"errors"
	"github.com/spf13/viper"
	"github.com/treble-h/trebiz/sign"
	"go.dedis.ch/kyber/v3/share"
	"strconv"
	"strings"
)

type Config struct {
	AddrStr     string
	ClusterAddr map[uint32]string // Addresses of all the nodes in the cluster

	ClusterPort map[uint32]int // Addresses of all the nodes in the cluster

	ReplicaId    uint32
	MyPrivateKey ed25519.PrivateKey
	PublicKeyMap map[uint32]ed25519.PublicKey //PublicKeyMap of all the nodes,replicaid to pubkey

	TsPriKey   *share.PriShare
	TsPubKey   *share.PubPoly
	FastPriKey *share.PriShare
	FastPubKey *share.PubPoly

	P2PListenPort int
	RPCListenPort int

	BatchTimeout int
	BatchSize    int
	CheckPointT  uint32
	LogK         uint32
	MaxPool      int

	NodeType              int //0bc  1bg   2abm  3pbm
	EvilPR                int
	FastTimeout           int //fast path timeout
	FastQcQuorum          int //fast path timeout
	ViewChangeQuorum      int //number of viewchangeMsg for ViewChange Protocol
	PrePrepareSubsetCount int
	SameIpTimeout         int

	ViewChangeTimeout int
	LastNewVewTimeout int
	AutoViewChange    int
}

func New(addrStr string, clusterAddr map[uint32]string, clusterPort map[uint32]int, replicaId uint32,
	myPrivateKey ed25519.PrivateKey, publicKeyMap map[uint32]ed25519.PublicKey, tsPriKey *share.PriShare,
	tsPubKey *share.PubPoly, fastPriKey *share.PriShare, fastPubKey *share.PubPoly, p2PListenPort int, rPCListenPort int, batchTimeout int, batchSize int,
	checkPointT uint32, logL uint32, maxPool int, pr int, fastTimeout int, fastQcQuorum int, viewChangeQuorum,
	prePrepareSubsetCount, sameIpTimeout int, viewChangeTimeout int, lastNewVewTimeout int, autoViewChange int) *Config {
	return &Config{
		AddrStr:     addrStr,
		ClusterAddr: clusterAddr,
		ClusterPort: clusterPort,
		ReplicaId:   replicaId,

		MyPrivateKey: myPrivateKey,
		PublicKeyMap: publicKeyMap,
		TsPriKey:     tsPriKey,
		TsPubKey:     tsPubKey,
		FastPriKey:   fastPriKey,
		FastPubKey:   fastPubKey,

		P2PListenPort:         p2PListenPort,
		RPCListenPort:         rPCListenPort,
		BatchTimeout:          batchTimeout,
		BatchSize:             batchSize,
		CheckPointT:           checkPointT,
		LogK:                  logL,
		MaxPool:               maxPool,
		EvilPR:                pr,
		FastTimeout:           fastTimeout,
		FastQcQuorum:          fastQcQuorum,
		ViewChangeQuorum:      viewChangeQuorum,
		PrePrepareSubsetCount: prePrepareSubsetCount,
		SameIpTimeout:         sameIpTimeout,
		ViewChangeTimeout:     viewChangeTimeout,
		LastNewVewTimeout:     lastNewVewTimeout,
		AutoViewChange:        autoViewChange,
	}
}

func LoadConfig(configPrefix, configName string) (*Config, error) {
	viperConfig := viper.New()
	// for environment variables
	viperConfig.SetEnvPrefix(configPrefix)
	viperConfig.AutomaticEnv()
	replacer := strings.NewReplacer(".", "_")
	viperConfig.SetEnvKeyReplacer(replacer)

	viperConfig.SetConfigName(configName)
	viperConfig.AddConfigPath("./")

	err := viperConfig.ReadInConfig()
	if err != nil {
		return nil, err
	}

	privKeyEDAsString := viperConfig.GetString("ed_prikey")
	privKeyED, err := hex.DecodeString(privKeyEDAsString)
	if err != nil {
		return nil, err
	}

	tsPubKeyAsString := viperConfig.GetString("tspubkey")
	tsPubKeyAsBytes, err := hex.DecodeString(tsPubKeyAsString)
	if err != nil {
		return nil, err
	}
	tsPubKey, err := sign.DecodeTSPublicKey(tsPubKeyAsBytes)
	if err != nil {
		return nil, err
	}

	tsShareAsString := viperConfig.GetString("tsshare")
	tsShareAsBytes, err := hex.DecodeString(tsShareAsString)
	if err != nil {
		return nil, err
	}
	tsShareKey, err := sign.DecodeTSPartialKey(tsShareAsBytes)
	if err != nil {
		return nil, err
	}

	fastPubKeyAsString := viperConfig.GetString("fasttspubkey")
	fastPubKeyAsBytes, err := hex.DecodeString(fastPubKeyAsString)
	if err != nil {
		return nil, err
	}
	fastPubKey, err := sign.DecodeTSPublicKey(fastPubKeyAsBytes)
	if err != nil {
		return nil, err
	}

	fastShareAsString := viperConfig.GetString("fasttsshare")
	fastShareAsBytes, err := hex.DecodeString(fastShareAsString)
	if err != nil {
		return nil, err
	}
	fastShareKey, err := sign.DecodeTSPartialKey(fastShareAsBytes)
	if err != nil {
		return nil, err
	}

	conf := &Config{
		AddrStr:      viperConfig.GetString("address"),
		ReplicaId:    uint32(viperConfig.GetInt("replicaid")),
		MyPrivateKey: privKeyED,
		TsPriKey:     tsShareKey,
		TsPubKey:     tsPubKey,
		FastPriKey:   fastShareKey,
		FastPubKey:   fastPubKey,

		RPCListenPort:         viperConfig.GetInt("rpc_listen_port"),
		P2PListenPort:         viperConfig.GetInt("p2p_listen_port"),
		BatchTimeout:          viperConfig.GetInt("batchtimeout"),
		BatchSize:             viperConfig.GetInt("batchsize"),
		CheckPointT:           uint32(viperConfig.GetInt("checkpoint_t")),
		LogK:                  uint32(viperConfig.GetInt("log_k")),
		MaxPool:               viperConfig.GetInt("maxpool"),
		NodeType:              viperConfig.GetInt("nodetype"), //0bc  1bg   2abm  3pbm
		EvilPR:                viperConfig.GetInt("evilpr"),
		FastTimeout:           viperConfig.GetInt("fastpathtimeout"), //fast path timeout
		FastQcQuorum:          viperConfig.GetInt("fastqcquorum"),
		PrePrepareSubsetCount: viperConfig.GetInt("prePrepareSubsetCount"),
		ViewChangeQuorum:      viperConfig.GetInt("viewChangeQuorum"),
		SameIpTimeout:         viperConfig.GetInt("sameiptimeout"),
		ViewChangeTimeout:     viperConfig.GetInt("viewchangetimeout"),
		LastNewVewTimeout:     viperConfig.GetInt("lastnewvewtimeout"),
		AutoViewChange:        viperConfig.GetInt("autoviewchange"),
	}

	peersP2PPortMapString := viperConfig.GetStringMap("peers_p2p_port")
	peersIPsMapString := viperConfig.GetStringMap("cluster_ips")
	pubKeyMapString := viperConfig.GetStringMap("cluster_ed_pubkey")

	pubKeyMap := make(map[uint32]ed25519.PublicKey, len(pubKeyMapString))
	clusterAddr := make(map[uint32]string, len(pubKeyMapString))
	clusterPort := make(map[uint32]int, len(pubKeyMapString))

	for name, pkAsInterface := range pubKeyMapString {

		rs := []rune(name)
		id, _ := strconv.Atoi(string(rs[4:]))

		clusterPort[uint32(id)] = peersP2PPortMapString[name].(int)
		addr := peersIPsMapString[name].(string)
		clusterAddr[uint32(id)] = addr

		if pkAsString, ok := pkAsInterface.(string); ok {
			pubKey, err := hex.DecodeString(pkAsString)
			if err != nil {
				return nil, err
			}
			pubKeyMap[uint32(id)] = pubKey
		} else {
			return nil, errors.New("public key in the config file cannot be decoded correctly")
		}
	}

	conf.PublicKeyMap = pubKeyMap
	conf.ClusterPort = clusterPort
	conf.ClusterAddr = clusterAddr

	return conf, nil
}
