package natstest

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"testing"
	"time"

	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

type Node struct {
	Server *server.Server
	Conn   *nats.Conn
}

func StartCluster(t testing.TB, n int) []*Node {
	t.Helper()
	ports := freePorts(t, n)
	servers := make([]*server.Server, 0, n)
	nodes := make([]*Node, 0, n)
	storeDirs := make([]string, n)
	for i := range storeDirs {
		storeDirs[i] = t.TempDir()
	}
	t.Cleanup(func() {
		for _, node := range nodes {
			node.Conn.Close()
		}
		for _, ns := range servers {
			ns.Shutdown()
			ns.WaitForShutdown()
		}
	})
	for i := 0; i < n; i++ {
		opts := &server.Options{
			ServerName: fmt.Sprintf("n%d", i),
			Host:       "127.0.0.1",
			Port:       -1,
			NoSigs:     true,
			NoLog:      true,
			JetStream:  true,
			StoreDir:   storeDirs[i],
		}
		if 1 < n {
			opts.Cluster.Name = "callfs-test"
			opts.Cluster.Host = "127.0.0.1"
			opts.Cluster.Port = ports[i]
			for j, port := range ports {
				if j == i {
					continue
				}
				routeURL, err := url.Parse(fmt.Sprintf("nats-route://127.0.0.1:%d", port))
				if nil != err {
					t.Fatalf("build cluster route url: %v", err)
				}
				opts.Routes = append(opts.Routes, routeURL)
			}
		}
		ns, err := startServer(opts)
		if nil != err {
			t.Fatalf("start nats server n%d: %v", i, err)
		}
		servers = append(servers, ns)
	}
	waitForRoutes(t, servers, n-1)
	for _, ns := range servers {
		nc, err := nats.Connect(ns.ClientURL())
		if nil != err {
			t.Fatalf("connect to %s: %v", ns.Name(), err)
		}
		nodes = append(nodes, &Node{Server: ns, Conn: nc})
	}
	return nodes
}

func freePorts(t testing.TB, n int) []int {
	t.Helper()
	listeners := make([]net.Listener, 0, n)
	ports := make([]int, 0, n)
	for i := 0; i < n; i++ {
		l, err := net.Listen("tcp", "127.0.0.1:0")
		if nil != err {
			t.Fatalf("reserve cluster port: %v", err)
		}
		listeners = append(listeners, l)
		ports = append(ports, l.Addr().(*net.TCPAddr).Port)
	}
	for _, l := range listeners {
		if err := l.Close(); nil != err {
			t.Fatalf("release cluster port: %v", err)
		}
	}
	return ports
}

func WaitJetStream(t testing.TB, nc *nats.Conn) {
	t.Helper()
	js, err := jetstream.New(nc)
	if nil != err {
		t.Fatalf("create jetstream context: %v", err)
	}
	deadline := time.Now().Add(20 * time.Second)
	for {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		_, err = js.AccountInfo(ctx)
		cancel()
		if nil == err {
			return
		}
		if !time.Now().Before(deadline) {
			t.Fatalf("jetstream not ready within 20s: %v", err)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func startServer(opts *server.Options) (*server.Server, error) {
	ns, err := server.NewServer(opts)
	if nil != err {
		return nil, fmt.Errorf("new server: %w", err)
	}
	ns.Start()
	if !ns.ReadyForConnections(10 * time.Second) {
		ns.Shutdown()
		ns.WaitForShutdown()
		return nil, fmt.Errorf("server %s not ready for connections within 10s", opts.ServerName)
	}
	return ns, nil
}

func waitForRoutes(t testing.TB, servers []*server.Server, minRoutes int) {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for {
		ready := true
		for _, ns := range servers {
			if ns.NumRoutes() < minRoutes {
				ready = false
				break
			}
		}
		if ready {
			return
		}
		if !time.Now().Before(deadline) {
			t.Fatalf("cluster routes not formed within 15s: want at least %d routes per node", minRoutes)
		}
		time.Sleep(100 * time.Millisecond)
	}
}
