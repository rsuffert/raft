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

You may start a RAFT process either locally or on a remote cluster with the following command.

```bash
go run main.go -id=<NODE_ID> -listen=<IP:PORT> -peers=<PEER_LIST>
```

- `<NODE_ID>`: Unique integer ID for this node (e.g., 0, 1, 2).
- `<IP:PORT>`: The address this node should listen on (e.g., 127.0.0.1:9000).
- `<PEER_LIST>`: Comma-separated list of peer nodes in the format `id=ip:port` (e.g., `1=127.0.0.1:9001,2=127.0.0.1:9002`).

For instance, in order to run three RAFT servers locally on your machine, you can run each of the three comands below in a separate terminal window.

```bash
go run main.go -id=0 -listen=127.0.0.1:9000 -peers=1=127.0.0.1:9001,2=127.0.0.1:9002
go run main.go -id=1 -listen=127.0.0.1:9001 -peers=0=127.0.0.1:9000,2=127.0.0.1:9002
go run main.go -id=2 -listen=127.0.0.1:9002 -peers=0=127.0.0.1:9000,1=127.0.0.1:9001
```