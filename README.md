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