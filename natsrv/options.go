package natsrv

import (
	"fmt"
	"net"
	"net/url"
	"strconv"

	"github.com/ebogdum/callfs/v2/config"
	"github.com/nats-io/nats-server/v2/server"
)

func BuildOptions(cfg config.NATSConfig, peers map[string]string) (*server.Options, error) {
	tags := make([]string, 0, len(cfg.ServerTags))
	for key, value := range cfg.ServerTags {
		tags = append(tags, key+":"+value)
	}

	opts := &server.Options{
		ServerName: cfg.ServerName,
		NoSigs:     true,

		JetStream:          true,
		StoreDir:           cfg.JetStream.StoreDir,
		JetStreamMaxMemory: cfg.JetStream.MaxMemory.Int64(),
		JetStreamMaxStore:  cfg.JetStream.MaxStore.Int64(),
		JetStreamUniqueTag: cfg.JetStream.UniqueTag,
		MaxPayload:         int32(cfg.Metadata.MaxMsgSize.Int64()),
		Tags:               tags,
	}

	if "" != cfg.MonitorListen {
		host, port, err := splitListen(cfg.MonitorListen)
		if nil != err {
			return nil, fmt.Errorf("nats.monitor_listen: %w", err)
		}
		opts.HTTPHost = host
		opts.HTTPPort = port
	}

	if config.NATSSyncAlways == cfg.JetStream.Sync {
		opts.SyncAlways = true
	} else {
		opts.SyncInterval = cfg.JetStream.SyncInterval
	}

	if "" == cfg.ClientListen {
		opts.DontListen = true
	} else {
		host, port, err := splitListen(cfg.ClientListen)
		if nil != err {
			return nil, fmt.Errorf("nats.client_listen: %w", err)
		}
		opts.Host = host
		opts.Port = port
	}

	clusterHost, clusterPortNumber, err := splitListen(cfg.ClusterListen)
	if nil != err {
		return nil, fmt.Errorf("nats.cluster_listen: %w", err)
	}
	opts.Cluster.Name = cfg.ClusterName
	opts.Cluster.Host = clusterHost
	opts.Cluster.Port = clusterPortNumber

	routes := make([]string, 0, len(cfg.Routes)+len(peers))
	routes = append(routes, cfg.Routes...)
	if cfg.RoutesFromPeers {
		for name, endpoint := range peers {
			if name == cfg.ServerName {
				continue
			}
			parsed, err := url.Parse(endpoint)
			if nil != err {
				return nil, fmt.Errorf("nats.routes: %w", err)
			}
			routes = append(routes, "nats-route://"+parsed.Hostname()+":"+strconv.Itoa(clusterPortNumber))
		}
	}
	for _, route := range routes {
		parsed, err := url.Parse(route)
		if nil != err {
			return nil, fmt.Errorf("nats.routes: %w", err)
		}
		opts.Routes = append(opts.Routes, parsed)
	}
	if 0 == len(opts.Routes) {
		opts.Cluster = server.ClusterOpts{}
	}
	if opts.DontListen && 0 < len(opts.Routes) {
		opts.DontListen = false
		opts.Host = "127.0.0.1"
		opts.Port = server.RANDOM_PORT
	}

	if "" != cfg.TLS.CertFile {
		clientTLS, err := clientPortTLSConfig(cfg.TLS)
		if nil != err {
			return nil, fmt.Errorf("nats.tls: %w", err)
		}
		opts.TLS = true
		opts.TLSConfig = clientTLS
		opts.TLSTimeout = tlsHandshakeTimeout
		if 0 < len(opts.Routes) {
			routeTLS, err := clusterTLSConfig(cfg.TLS)
			if nil != err {
				return nil, fmt.Errorf("nats.tls: %w", err)
			}
			opts.Cluster.TLSConfig = routeTLS
			opts.Cluster.TLSTimeout = tlsHandshakeTimeout
		}
	}

	return opts, nil
}

func splitListen(listen string) (string, int, error) {
	host, portText, err := net.SplitHostPort(listen)
	if nil != err {
		return "", 0, err
	}
	port, err := strconv.Atoi(portText)
	if nil != err {
		return "", 0, err
	}
	return host, port, nil
}
