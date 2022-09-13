## Description
An implementation for PBFT (Practical Byzantine Fault Tolerance) and Trebiz. 

The function implementing the inter-node communications utilizes the code from the project `https://github.com/hashicorp/raft`. Many thanks for this project's developers.

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
- Ansible version: 2.5.1+

### 3. Steps to run trebiz

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

#### 3.3 Install Go-related modules/packages

```shell
# Enter the directory `trebiz`
go mod tidy
```

#### 3.4 Generate configurations

Generate configurations for each server.

Operations below are done on the *work computer*.

- Change `ips`, `peers_p2p_port`,  `rpc_listen_port`and other parameters  in file `config_gen/config_template.yaml`
- Enter the directory `config_gen`, and run `go run main.go`

#### 3.5 Configure servers via ansible tool
Change the `hosts` file in the directory `ansible`, the hostnames and IPs should be consistent with `config_gen/config_template.yaml`.

And commands below are run on the *work computer*.

```shell script
# Enter the directory `trebiz`
go build
# Enter the directory `ansible`
ansible-playbook -i ./hosts conf-server.yaml
```

#### 3.6 Run trebiz servers via ansible tool
```shell script
# run trebiz servers
ansible-playbook -i ./hosts run-server.yaml
```

You can stop servers by using command

```shell
# stop trebiz/pbft servers
ansible-playbook -i ./hosts kill-server.yaml
```

#### 3.7 Run Client

For stable testing, we let the primary node to package a batch by itself if it does not receive any request from the client within the `batchtimeout` period. And you can also run a client to send request.

You can use the workserver as the client or run a client on a remote host. The following command will start a client and the client will keep sending requests to  the primary node until you stop it:

```shell
#Enter the directory `client`
go run main.go -rpcaddress $IP_ADDR_OF_Leader sendrequest
```

If you want to send exact number of requests, you can use：

```shell
#Enter the directory `client`
go run main.go -rpcaddress $IP_ADDR_OF_Leader sendrequest -n $number
```

For the full list of flags, run `go run main.go -rpcaddress $IP_ADDR_OF_Leader hlep sendrequest`.



