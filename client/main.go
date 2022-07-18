package main

import (
	"fmt"
	"github.com/urfave/cli"
	"log"
	"net/rpc"
	"os"
	"strconv"
)

type CMD struct {
	rpcAddress string
	rpcPort    int
	reqText    string
}

func (cmd *CMD) sendRequest() {
	client, err := rpc.DialHTTP("tcp", cmd.rpcAddress+":"+strconv.Itoa(cmd.rpcPort))
	if err != nil {
		panic(err)
	}

	req := []byte(cmd.reqText)

	var reply string

	err = client.Call("ReqHandler.ReceiveNewRequest", req, &reply)

	if err != nil {
		panic(err)
	} else {
		fmt.Println(reply)
	}
}

func (cmd *CMD) Run() {
	app := &cli.App{
		Name: "A client to communicate with trebiz node",
	}
	var requestNum int
	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name:        "rpcaddress, a",
			Usage:       "Daemon RPC `ADDRESS` to connect to",
			Required:    true,
			Destination: &cmd.rpcAddress,
		},
		cli.IntFlag{
			Name:        "rpcport, p",
			Usage:       "Daemon RPC `PORT` to connect to",
			Required:    false,
			Value:       9500,
			Destination: &cmd.rpcPort,
		},
	}

	app.Commands = []cli.Command{
		// send request CMD
		{
			Name:        "sendrequest",
			Usage:       "send a request to be executed by yimchain",
			Description: "Send a request",
			Flags: []cli.Flag{
				cli.StringFlag{
					Name:        "request, r",
					Required:    false,
					Value:       "",
					Destination: &cmd.reqText,
				},
				cli.IntFlag{
					Name:        "requestNum, n",
					Usage:       "the number of requests the client should send",
					Required:    false,
					Value:       -1,
					Destination: &requestNum,
				},
			},
			Action: func(c *cli.Context) error {
				if requestNum == -1 {
					for {
						cmd.sendRequest()
					}
				} else {
					for i := 0; i < requestNum; i++ {
						cmd.sendRequest()
					}
				}
				return nil
			},
		},
	}

	err := app.Run(os.Args)
	if err != nil {
		log.Panic(err)
	}
}

func main() {
	//when test normal case, we just let the client send requests to the leader
	cmd := new(CMD)
	cmd.Run()
}
