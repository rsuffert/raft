# :rowboat: Raft

This is an adaption of the original code for a Distributed Systems course assignment. The `raft` repository contains the incremental implementation of the Raft consensus algoritm describted in the below blog posts:

* [Part 0: Introduction](https://eli.thegreenplace.net/2020/implementing-raft-part-0-introduction/)
* [Part 1: Elections](https://eli.thegreenplace.net/2020/implementing-raft-part-1-elections/)
* [Part 2: Commands and log replication](https://eli.thegreenplace.net/2020/implementing-raft-part-2-commands-and-log-replication/)

## Running tests

You may use the below commands to run and visualize the results of the `TestElectionFollowerComesBack` test case, for instance:

```bash
$ cd raft
$ go test -v -race -run TestElectionFollowerComesBack |& tee /tmp/raftlog
... logging output
... test should PASS
$ go run ../tools/raft-testlog-viz/main.go < /tmp/raftlog
PASS TestElectionFollowerComesBack map[0:true 1:true 2:true TEST:true] ; entries: 150
... Emitted file:///tmp/TestElectionFollowerComesBack.html

PASS
```

Now open `file:///tmp/TestElectionFollowerComesBack.html` in your browser.
You should see something like this:

![Image of log browser](./raftlog-screenshot.png)

Scroll and read the logs from the servers, noticing state changes (highlighted with colors).

To simply run all test cases, use this:

```bash
cd raft
go test -v -race ./...
```

## Running a RAFT cluster

### Simulating locally

You may start a RAFT process either locally or on a remote cluster with the following command.

```bash
go run main.go -id=<NODE_ID> -listen=<IP:PORT> -peers=<PEER_LIST>
```

- `<NODE_ID>`: Unique integer ID for this node (e.g., 0, 1, 2).
- `<IP:PORT>`: The address this node should listen on (e.g., 127.0.0.1:9000).
- `<PEER_LIST>`: Comma-separated list of peer nodes in the format `id=ip:port` (e.g., `1=127.0.0.1:9001,2=127.0.0.1:9002`).

For instance, in order to run three RAFT servers locally on your machine, you can run each of the three commands below in a separate terminal window.

```bash
go run main.go -id=0 -listen=127.0.0.1:9000 -peers=1=127.0.0.1:9001,2=127.0.0.1:9002
go run main.go -id=1 -listen=127.0.0.1:9001 -peers=0=127.0.0.1:9000,2=127.0.0.1:9002
go run main.go -id=2 -listen=127.0.0.1:9002 -peers=0=127.0.0.1:9000,1=127.0.0.1:9001
```

### Running in the cloud

You may run the RAFT instances in a cluster in the cloud. As an example, we'll provide a step-by-step to do so on Azure through its CLI. We assume you already have Azure CLI installed on your environment.

#### Connecting to Azure

Use this command to authenticate to your Azure account.

```bash
az login
```

You will be redirected to the browser to follow the authentication process. Once it's done, return to your terminal and select the subscription you want to use. The command should exit with success.

#### Setting up Azure VMs

First, we'll create a resource group with the below command.

```bash
az group create --name MyRaftCloudRG --location eastus
```

Then, we'll go ahead and create a Virtual Network (VNet) and subnet for our RAFT servers.

```bash
az network vnet create \
    --resource-group MyRaftCloudRG \
    --name MyRaftCloudVNet \
    --address-prefix 10.0.0.0/16 \
    --subnet-name MyRaftCloudSubnet \
    --subnet-prefix 10.0.0.0/24
```

Next, we create a Network Security Group (NSG) and set up security rules for our three RAFT processes.

```bash
# Create NSG
az network nsg create \
    --resource-group MyRaftCloudRG \
    --name MyRaftCloudNSG

# Allow SSH (Port 22)
az network nsg rule create \
    --resource-group MyRaftCloudRG \
    --nsg-name MyRaftCloudNSG \
    --name AllowSSH \
    --protocol Tcp \
    --priority 1000 \
    --destination-port-range 22 \
    --access Allow \
    --direction Inbound

# Allow RAFT communication ports (we'll make our processes listen on port 9000)
# This rule allows traffic from within the VNet on these ports.
az network nsg rule create \
    --resource-group MyRaftCloudRG \
    --nsg-name MyRaftCloudNSG \
    --name AllowRaftComms \
    --protocol Tcp \
    --priority 1010 \
    --source-address-prefixes VirtualNetwork \
    --destination-port-ranges 9000 \
    --access Allow \
    --direction Inbound
```

Finally, we'll provision small, cost-effective VMs for each of the RAFT servers we want in our cluster. Repeat the below command for each instance you want to create (we have configured everything to create three in the previous step).

```bash
VM_NAME="raft-cloud-node0" # Change to raft-cloud-node1, raft-cloud-node2 etc.
az vm create \
    --resource-group MyRaftCloudRG \
    --name ${VM_NAME} \
    --image Ubuntu2204 \
    --size Standard_B1s \
    --vnet-name MyRaftCloudVNet \
    --subnet MyRaftCloudSubnet \
    --nsg MyRaftCloudNSG \
    --admin-username azureuser \
    --generate-ssh-keys \
    --public-ip-sku Standard
```

Note down the private IP addresses for each VM created.

#### Deploying and running the RAFT servers

For each of the VMs we created, we'll connect to them via SSH to fetch and run our code from GitHub. The following process should be applied for each of the VMs.

First, we'll connect to the VM via SSH.

```bash
ssh azureuser@<PUBLIC_IP_OF_THE_VM>
```

Once connected to the VM, install Git and Go.

```bash
sudo apt update
sudo apt install -y git
sudo snap install go --classic # or use apt, or download tarball

# verify installation
go version
git version
```

Then, clone this GitHub repository.

```bash
git clone https://github.com/rsuffert/raft.git
cd raft
```

Finally, run the RAFT application on the VM.

**NOTICE:** Be sure to use the **internal IP addresses** of the process here, *not* the public ones.

```bash
nohup go run main.go \
    -id=<sequential-id> \
    -listen=<process-private-ip>:9000 \
    -peers=<peer-sequential-id>=<peer-private-ip>:9000,[peer-sequential-id>=<peer-private-ip>:9000] \
    > raft.log 2>&1 &
```

After the RAFT nodes start running, you can check the logs for each process connected to its VM with the following command.

```bash
tail -f raft.log
```

Since RAFT will be running on the background, you may use the PID returned by the `nohup` command to kill the RAFT process running on the current node with this command:

```bash
kill -9 <pid>
```

Then, you can observe the behavior on the other nodes of the system and eventually recover the failing node by re-running the `nohup` command above.

#### Stopping the infrastructure (IMPORTANT!)

**You need to make sure you stop your VMs after your tests to avoid extra costs!**

```bash
az group delete --name MyRaftCloudRG --yes --no-wait
```