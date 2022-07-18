## Description
An implementation for PBFT (Practical Byzantine Fault Tolerance) and Trebiz. 

The function implementing the inter-node communications utilizes the code from the project `https://github.com/hashicorp/raft`. Many thanks for this project's developers.

## Contents

[TOC]

## Usage
### 1. Machine types
Machines are divided into two types:
- *Workcomputer*: just configure `servers` and `clients` at the initial stage, particularly via `ansible` tool 
- *Servers*: run daemons of `trebiz/pbft`, communicate with each other via P2P model
- *Clients*: run `client`, communicate with `server` via RPC model 

### 2. Precondition
- Recommended OS releases: Ubuntu 18.04 (other releases may also be OK)
- Go version: 1.17+ (with Go module enabled)
- Python version: 3.6.9+

### 3. Steps to run yimchain

#### 3.1 Install ansible on the work computer
Commands below are run on the *work computer*.
```shell script
sudo apt install python3-pip
sudo pip3 install --upgrade pip
pip3 install ansible
# add ~/.local/bin to your $PATH
echo 'export PATH=$PATH:~/.local/bin:/usr/local/go/bin' >> ~/.bashrc
source ~/.bashrc
```

#### 3.2 Login without passwords
Enable workcomputer to login in servers and clients without passwords.

Commands below are run on the *work computer*.
```shell script
# Generate the ssh keys (if not generated before)
ssh-keygen
ssh-copy-id -i ~/.ssh/id_rsa.pub $IP_ADDR_OF_EACH_SERVER
```

#### 3.3 Generate configurations
Generate configurations for each server.

Commands below are run on the *work computer*.

- Change `IPs`, `peers_p2p_port`,  `rpc_listen_port`and other parameters  in file `config_gen/config_template.yaml`
  - Some important parameters in `config_template.yaml` are as follows:

    - `IP1s` the private IP of the nodes.
    - `IP2s` the public IP of the nodes.
    - `peers_p2p_port` the P2P transport port that each node listens on for communication with other nodes.
    - `rpc_listen_port` the port on which the node listens for client requests.
    - `batchSize` the number of client commands that should be batched together in a block.
    - `batchtimeout` the rate at which the leader produces a block, in milliseconds.
    - `fast_path_timeout` the fast path duration, if a request can not commit in `fast_path_timeout`, it will follow the steps of normal case in PBFT. So when we test PBFT, we can set this value very small (like 10 microseconds). And when we test trebiz, we can set it to a reasonable value according to the network situation.
    - `evilpr` the probability of a node doing evil(not responding).
    - `bgnum` the number of Byzantine nodes.
    - `abmnum` the number of active Byzantine Merchants.
    - `pbmnum` the number of passive Byzantine Merchants.

- Enter the directory `config_gen`, and run `go run main.go`

#### 3.4 Configure servers via Ansible tool
Change the `hosts` file in the directory `ansible`, the hostnames and IPs should be consistent with `config_gen/config_template.yaml`.

And commands below are run on the *work computer*.

```shell script
# Enter the directory `trebiz`
go build
# Enter the directory `ansible`
ansible-playbook -i ./hosts conf-server.yaml
```

#### 3.5 Run yimchain servers via Ansible tool
```shell script
# run trebiz/pbft servers
ansible-playbook -i ./hosts run-server.yaml
```

You can stop servers by using command

```shell
# stop trebiz/pbft servers
ansible-playbook -i ./hosts kill-server.yaml
```

#### 3.6 Run Client

You can use the workserver as the client or run client on a remote host

```shell
#Enter the directory `client`
go run main.go 1000
```

`go run main.go 100` means the client will send 100 requests, you can send any number of requests.