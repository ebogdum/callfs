package natsrv

import (
	"context"
	"fmt"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/ebogdum/callfs/config"
	"go.uber.org/zap"
)

func TestStartThreeNodeCluster(t *testing.T) {
	ports := make([]int, 3)
	for i := range ports {
		l, err := net.Listen("tcp", "127.0.0.1:0")
		if nil != err {
			t.Fatal(err)
		}
		ports[i] = l.Addr().(*net.TCPAddr).Port
		if err := l.Close(); nil != err {
			t.Fatal(err)
		}
	}
	servers := make([]*Server, 3)
	errs := make([]error, 3)
	var wg sync.WaitGroup
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			cfg := config.NATSConfig{Enabled: true, Mode: config.NATSModeEmbedded, ServerName: fmt.Sprintf("n%d", i), ClusterName: "callfs-test", ClusterListen: fmt.Sprintf("127.0.0.1:%d", ports[i])}
			for j, port := range ports {
				if j != i {
					cfg.Routes = append(cfg.Routes, fmt.Sprintf("nats-route://127.0.0.1:%d", port))
				}
			}
			cfg.JetStream = config.NATSJetStreamConfig{StoreDir: t.TempDir(), Sync: config.NATSSyncAlways}
			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()
			servers[i], errs[i] = Start(ctx, cfg, "secret-0123456789abcdef", nil, zap.NewNop())
		}(i)
	}
	wg.Wait()
	t.Cleanup(func() {
		for _, s := range servers {
			if nil != s {
				_ = s.Shutdown(context.Background())
			}
		}
	})
	for i, err := range errs {
		if nil != err {
			t.Fatalf("n%d start: %v", i, err)
		}
	}
}
