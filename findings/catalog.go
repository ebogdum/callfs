package findings

import (
	"math"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/ebogdum/callfs/v2/config"
)

const (
	SeverityCritical = "critical"
	SeverityWarning  = "warning"
	SeverityInfo     = "info"
)

const (
	AreaSecurity = "security"
	AreaStorage  = "storage"
	AreaCluster  = "cluster"
	AreaConfig   = "config"
	AreaRuntime  = "runtime"
)

const (
	certWarnWindow                = 21 * 24 * time.Hour
	instanceDownGrace             = 60 * time.Second
	largeUploadReadTimeout        = 5 * time.Minute
	minSecretLength               = 32
	stagingByteLimit       uint64 = 1 << 30
	stagingCountLimit             = 100
	diskWarningRatio              = 0.10
	diskCriticalRatio             = 0.05
	sharedClientShare             = 0.9
	authFailureLimit              = 100
	defaultInstanceID             = "callfs-instance-1"
	defaultDBPassword             = "callfs"
	logLagLimit            uint64 = 1000
	snapshotStaleFactor           = 3
)

var placeholderSecrets = []string{
	"your-internal-secret-here-16ch",
	"your-link-secret-here-16chars",
}

type hit struct {
	severity string
	n        int
	id       string
	setting  string
	route    string
	affected []Entity
}

type input struct {
	cfg   *config.AppConfig
	facts Facts
	now   time.Time
}

type rule struct {
	id       string
	area     string
	severity string
	title    string
	body     string
	label    string
	route    string
	setting  string
	check    func(in input) *hit
}

func hitIf(condition bool) *hit {
	if condition {
		return &hit{}
	}
	return nil
}

func (entry rule) finding(h *hit) Finding {
	severity := entry.severity
	if "" != h.severity {
		severity = h.severity
	}
	setting := entry.setting
	if "" != h.setting {
		setting = h.setting
	}
	route := entry.route
	if "" != h.route {
		route = h.route
	}
	fix := Fix{Label: entry.label, Route: route}
	if "" != setting {
		key := setting
		fix.SettingKey = &key
	}
	affected := h.affected
	if nil == affected {
		affected = []Entity{}
	}
	replacer := strings.NewReplacer("{n}", strconv.Itoa(h.n), "{id}", h.id)
	return Finding{
		ID:       entry.id,
		Severity: severity,
		Area:     entry.area,
		Title:    replacer.Replace(entry.title),
		Body:     replacer.Replace(entry.body),
		Affected: affected,
		Fix:      fix,
	}
}

func isPlaceholder(secret string) bool {
	for _, placeholder := range placeholderSecrets {
		if placeholder == secret {
			return true
		}
	}
	return false
}

func hostOf(addr string) string {
	host, _, err := net.SplitHostPort(addr)
	if nil != err {
		return addr
	}
	return host
}

func isLoopbackAddr(addr string) bool {
	host := hostOf(addr)
	if "localhost" == host {
		return true
	}
	ip := net.ParseIP(host)
	return nil != ip && ip.IsLoopback()
}

func isPlainHTTP(endpoint string) bool {
	return strings.HasPrefix(strings.ToLower(endpoint), "http://")
}

func hasPlainPeerEndpoint(cfg *config.AppConfig) bool {
	for _, endpoint := range cfg.InstanceDiscovery.PeerEndpoints {
		if isPlainHTTP(endpoint) {
			return true
		}
	}
	return false
}

func hasDefaultDBPassword(dsn string) bool {
	if parsed, err := url.Parse(dsn); nil == err && nil != parsed.User {
		if password, set := parsed.User.Password(); set && defaultDBPassword == password {
			return true
		}
	}
	for _, field := range strings.Fields(dsn) {
		if "password="+defaultDBPassword == field {
			return true
		}
	}
	return false
}

func severityFromRatio(free, total uint64) (string, bool) {
	if 0 == total {
		return "", false
	}
	ratio := float64(free) / float64(total)
	if diskCriticalRatio > ratio {
		return SeverityCritical, true
	}
	if diskWarningRatio > ratio {
		return SeverityWarning, true
	}
	return "", false
}

var catalog = []rule{
	{
		id: "tls.plain_http", area: AreaSecurity, severity: SeverityCritical,
		title: "Connections are not encrypted",
		body:  "Access keys and files travel without encryption. Choose HTTPS as the connection type.",
		label: "Change setting", route: "/settings/network", setting: "server.protocol",
		check: func(in input) *hit {
			return hitIf("http" == in.cfg.Server.Protocol || ("auto" == in.cfg.Server.Protocol && in.facts.AutoFellBack))
		},
	},
	{
		id: "tls.cert_unreadable", area: AreaSecurity, severity: SeverityCritical,
		title: "Certificate file cannot be read",
		body:  "HTTPS cannot start without a readable certificate. Check the file path and its permissions.",
		label: "Open Security", route: "/settings/security",
		check: func(in input) *hit {
			return hitIf(in.facts.CertUnreadable)
		},
	},
	{
		id: "tls.cert_expiring", area: AreaSecurity, severity: SeverityWarning,
		title: "Certificate expires in {n} days",
		body:  "Browsers and servers reject the certificate after it expires. Renew it before then.",
		label: "Open Security", route: "/settings/security",
		check: func(in input) *hit {
			if in.facts.CertNotAfter.IsZero() {
				return nil
			}
			remaining := in.facts.CertNotAfter.Sub(in.now)
			if 0 >= remaining || certWarnWindow <= remaining {
				return nil
			}
			return &hit{n: int(math.Ceil(remaining.Hours() / 24))}
		},
	},
	{
		id: "tls.cert_expired", area: AreaSecurity, severity: SeverityCritical,
		title: "Certificate has expired",
		body:  "Browsers refuse to connect over HTTPS. Install a new certificate now.",
		label: "Open Security", route: "/settings/security",
		check: func(in input) *hit {
			return hitIf(!in.facts.CertNotAfter.IsZero() && in.now.After(in.facts.CertNotAfter))
		},
	},
	{
		id: "tls.key_world_readable", area: AreaSecurity, severity: SeverityWarning,
		title: "Private key file is readable by everyone",
		body:  "Any user on this machine can copy the key. Limit the file to the server account only.",
		label: "Open Security", route: "/settings/security",
		check: func(in input) *hit {
			return hitIf(in.facts.KeyWorldReadable)
		},
	},
	{
		id: "secret.default", area: AreaSecurity, severity: SeverityCritical,
		title: "Default secret in use",
		body:  "Anyone who knows the example text can guess this secret. Replace it with a long random value.",
		label: "Change setting", route: "/settings/security",
		check: func(in input) *hit {
			if isPlaceholder(in.cfg.Auth.InternalProxySecret) {
				return &hit{setting: "auth.internal_proxy_secret"}
			}
			if isPlaceholder(in.cfg.Auth.SingleUseLinkSecret) {
				return &hit{setting: "auth.single_use_link_secret"}
			}
			return nil
		},
	},
	{
		id: "secret.short", area: AreaSecurity, severity: SeverityWarning,
		title: "Secret is short",
		body:  "Secrets should be at least 32 characters. Use a long random value.",
		label: "Change setting", route: "/settings/security",
		check: func(in input) *hit {
			if minSecretLength > len(in.cfg.Auth.InternalProxySecret) {
				return &hit{setting: "auth.internal_proxy_secret"}
			}
			if minSecretLength > len(in.cfg.Auth.SingleUseLinkSecret) {
				return &hit{setting: "auth.single_use_link_secret"}
			}
			return nil
		},
	},
	{
		id: "secret.same", area: AreaSecurity, severity: SeverityWarning,
		title: "Two secrets have the same value",
		body:  "The link secret and the internal secret must differ. Set a new value for one of them.",
		label: "Open Security", route: "/settings/security",
		check: func(in input) *hit {
			return hitIf("" != in.cfg.Auth.SingleUseLinkSecret && in.cfg.Auth.SingleUseLinkSecret == in.cfg.Auth.InternalProxySecret)
		},
	},
	{
		id: "keys.positional", area: AreaSecurity, severity: SeverityWarning,
		title: "Some keys share a numbered user",
		body:  "Keys listed by position lose their owner if the list changes. Move them to named users.",
		label: "Open Users and keys", route: "/settings/access",
		check: func(in input) *hit {
			return hitIf(0 < len(in.cfg.Auth.APIKeys))
		},
	},
	{
		id: "admin.none", area: AreaSecurity, severity: SeverityInfo,
		title: "No admin user is set",
		body:  "Only the root account can manage this console. Add an admin user to share the work.",
		label: "Open Users and keys", route: "/settings/access",
		check: func(in input) *hit {
			return hitIf(0 == len(in.cfg.Auth.AdminUsers))
		},
	},
	{
		id: "metrics.public_port", area: AreaSecurity, severity: SeverityWarning,
		title: "Metrics port has no sign-in",
		body:  "Anyone who can reach this port can read server metrics. Bind it to a private address.",
		label: "Change setting", route: "/settings/network", setting: "metrics.listen_addr",
		check: func(in input) *hit {
			return hitIf("" != in.cfg.Metrics.ListenAddr && !isLoopbackAddr(in.cfg.Metrics.ListenAddr))
		},
	},
	{
		id: "peers.http", area: AreaCluster, severity: SeverityWarning,
		title: "Servers talk without encryption",
		body:  "Server-to-server traffic uses plain HTTP. Switch these addresses to HTTPS.",
		label: "Open Servers", route: "/cluster/servers",
		check: func(in input) *hit {
			return hitIf(hasPlainPeerEndpoint(in.cfg))
		},
	},
	{
		id: "peers.skip_verify", area: AreaSecurity, severity: SeverityCritical,
		title: "Server certificates are not checked",
		body:  "Servers accept any certificate from each other. Turn on certificate checks.",
		label: "Change setting", route: "/settings/security", setting: "backend.internal_proxy_skip_tls_verify",
		check: func(in input) *hit {
			return hitIf(in.cfg.Backend.InternalProxySkipTLSVerify)
		},
	},
	{
		id: "instance.default_id", area: AreaCluster, severity: SeverityWarning,
		title: "Server ID is the default",
		body:  "Every server needs its own ID. Set a unique server ID.",
		label: "Change setting", route: "/settings/coordination", setting: "instance_discovery.instance_id",
		check: func(in input) *hit {
			return hitIf(defaultInstanceID == in.cfg.InstanceDiscovery.InstanceID && 0 < len(in.cfg.InstanceDiscovery.PeerEndpoints))
		},
	},
	{
		id: "instance.unreachable", area: AreaCluster, severity: SeverityWarning,
		title: "Server {id} cannot be reached",
		body:  "This server has not answered for over a minute. Check the network and the address.",
		label: "Open Servers", route: "/cluster/servers",
		check: func(in input) *hit {
			var first string
			var affected []Entity
			for _, instance := range in.facts.InstancesDown {
				if instance.DownSince.IsZero() || instanceDownGrace >= in.now.Sub(instance.DownSince) {
					continue
				}
				if "" == first {
					first = instance.ID
				}
				affected = append(affected, Entity{Kind: "server", ID: instance.ID, Href: "/cluster/servers"})
			}
			if 0 == len(affected) {
				return nil
			}
			return &hit{id: first, affected: affected}
		},
	},
	{
		id: "nats.plain_transport", area: AreaSecurity, severity: SeverityWarning,
		title: "Cluster messaging is not encrypted",
		body:  "Servers exchange file index changes and locks without TLS. Set the NATS certificate files.",
		label: "Change setting", route: "/settings/coordination", setting: "nats.tls.cert_file",
		check: func(in input) *hit {
			return hitIf(natsPlainTransport(in.cfg))
		},
	},
	{
		id: "nats.not_connected", area: AreaCluster, severity: SeverityCritical,
		title: "This server is not connected to NATS",
		body:  "Shared file data, locks and server messages cannot be reached. Check the NATS settings and the network.",
		label: "Open Servers", route: "/cluster/servers",
		check: func(in input) *hit {
			return hitIf(in.cfg.NATS.Enabled && in.facts.NATSUnreachable)
		},
	},
	{
		id: "nats.no_quorum", area: AreaCluster, severity: SeverityCritical,
		title: "File index has lost its majority",
		body:  "Too many copies of the file index are offline, so changes cannot be saved. Bring the offline servers back.",
		label: "Open Servers", route: "/cluster/servers",
		check: func(in input) *hit {
			if !in.cfg.NATS.Enabled || 0 == in.facts.MetadataOffline || in.facts.MetadataReplicas > 2*in.facts.MetadataOffline {
				return nil
			}
			return &hit{n: in.facts.MetadataOffline}
		},
	},
	{
		id: "nats.replica_offline", area: AreaCluster, severity: SeverityWarning,
		title: "{n} file index copies are offline",
		body:  "The file index still works but has less protection. Bring the offline servers back or remove them.",
		label: "Open Servers", route: "/cluster/servers",
		check: func(in input) *hit {
			if !in.cfg.NATS.Enabled || 0 == in.facts.MetadataOffline || in.facts.MetadataReplicas <= 2*in.facts.MetadataOffline {
				return nil
			}
			return &hit{n: in.facts.MetadataOffline}
		},
	},
	{
		id: "nats.log_lag", area: AreaCluster, severity: SeverityWarning,
		title: "This server is behind on changes",
		body:  "It has not yet applied {n} recent file changes, so it may show old data. Check its network and disk speed.",
		label: "Open Servers", route: "/cluster/servers",
		check: func(in input) *hit {
			if !in.cfg.NATS.Enabled || logLagLimit >= in.facts.MetadataLogLag {
				return nil
			}
			return &hit{n: int(in.facts.MetadataLogLag)}
		},
	},
	{
		id: "nats.snapshot_stale", area: AreaCluster, severity: SeverityWarning,
		title: "File index snapshot is out of date",
		body:  "No snapshot was taken in the last {n} hours, so a new or restarted server takes longer to catch up. Take a snapshot now.",
		label: "Open Servers", route: "/cluster/servers", setting: "nats.metadata.snapshot_interval",
		check: func(in input) *hit {
			interval := in.cfg.NATS.Metadata.SnapshotInterval
			if !in.cfg.NATS.Enabled || 0 >= interval || in.facts.LastSnapshotAt.IsZero() {
				return nil
			}
			age := in.now.Sub(in.facts.LastSnapshotAt)
			if snapshotStaleFactor*interval >= age {
				return nil
			}
			return &hit{n: int(math.Ceil(age.Hours()))}
		},
	},
	{
		id: "index.sslmode_disable", area: AreaSecurity, severity: SeverityWarning,
		title: "File index connection is not encrypted",
		body:  "The database connection does not use TLS. Set sslmode to require or verify-full.",
		label: "Change setting", route: "/settings/index", setting: "metadata_store.dsn",
		check: func(in input) *hit {
			return hitIf("postgres" == in.cfg.MetadataStore.Type && strings.Contains(in.cfg.MetadataStore.DSN, "sslmode=disable"))
		},
	},
	{
		id: "index.default_password", area: AreaSecurity, severity: SeverityWarning,
		title: "File index uses the default password",
		body:  "The database password is the example value. Set a unique password.",
		label: "Change setting", route: "/settings/index", setting: "metadata_store.dsn",
		check: func(in input) *hit {
			return hitIf("postgres" == in.cfg.MetadataStore.Type && hasDefaultDBPassword(in.cfg.MetadataStore.DSN))
		},
	},
	{
		id: "redis.no_password", area: AreaSecurity, severity: SeverityWarning,
		title: "Redis has no password",
		body:  "Anyone who can reach Redis can change file data. Set a Redis password.",
		label: "Change setting", route: "/settings/index", setting: "metadata_store.redis_password",
		check: func(in input) *hit {
			if "redis" == in.cfg.MetadataStore.Type && "" == in.cfg.MetadataStore.RedisPassword {
				return &hit{setting: "metadata_store.redis_password"}
			}
			if "redis" == in.cfg.DLM.Type && "" == in.cfg.DLM.RedisPassword {
				return &hit{setting: "dlm.redis_password", route: "/settings/coordination"}
			}
			return nil
		},
	},
	{
		id: "locks.local_multi", area: AreaCluster, severity: SeverityCritical,
		title: "Edit locks do not cover all servers",
		body:  "Local locks only work on one server. Use Redis locks for several servers.",
		label: "Change setting", route: "/settings/coordination", setting: "dlm.type",
		check: func(in input) *hit {
			return hitIf("local" == in.cfg.DLM.Type && 0 < len(in.cfg.InstanceDiscovery.PeerEndpoints))
		},
	},
	{
		id: "s3.public_acl", area: AreaSecurity, severity: SeverityCritical,
		title: "New S3 files are public",
		body:  "Anyone with the link can read new files. Set the bucket access to private.",
		label: "Change setting", route: "/settings/storage", setting: "backend.s3_acl",
		check: func(in input) *hit {
			return hitIf(strings.HasPrefix(in.cfg.Backend.S3ACL, "public-"))
		},
	},
	{
		id: "s3.no_encryption", area: AreaSecurity, severity: SeverityInfo,
		title: "S3 files are not encrypted at rest",
		body:  "Files are stored in plain form in the bucket. Turn on server-side encryption.",
		label: "Change setting", route: "/settings/storage", setting: "backend.s3_server_side_encryption",
		check: func(in input) *hit {
			return hitIf("" == in.cfg.Backend.S3ServerSideEncryption)
		},
	},
	{
		id: "erasure.tolerance", area: AreaStorage, severity: SeverityWarning,
		title: "One server failure can break split files",
		body:  "One server holds more pieces than the spare count allows. Spread pieces across more servers.",
		label: "Open Durability", route: "/cluster/durability",
		check: func(in input) *hit {
			return hitIf(in.cfg.Erasure.Enabled && in.facts.ErasureMaxOnOneInstance > in.cfg.Erasure.ParityShards)
		},
	},
	{
		id: "erasure.damaged", area: AreaStorage, severity: SeverityWarning,
		title: "{n} split files need repair",
		body:  "Some pieces are missing or damaged. Repair these files to restore full protection.",
		label: "Open Durability", route: "/cluster/durability",
		check: func(in input) *hit {
			total := in.facts.ErasureReduced + in.facts.ErasureCritical
			if 0 == total {
				return nil
			}
			severity := SeverityWarning
			if 0 < in.facts.ErasureCritical {
				severity = SeverityCritical
			}
			return &hit{severity: severity, n: total}
		},
	},
	{
		id: "backup.failing", area: AreaStorage, severity: SeverityWarning,
		title: "Backup copies are failing",
		body:  "Recent backup copies did not finish. Check the backup storage.",
		label: "Open Backup", route: "/cluster/backup",
		check: func(in input) *hit {
			return hitIf(in.cfg.HA.ReplicationEnabled && 0 < in.facts.ReplicationFailures1h)
		},
	},
	{
		id: "disk.low", area: AreaStorage, severity: SeverityWarning,
		title: "Disk is almost full",
		body:  "Free space on this disk is running out. Delete files or add storage.",
		label: "Open Storage", route: "/cluster/storage",
		check: func(in input) *hit {
			level, low := severityFromRatio(in.facts.DiskFreeBytes, in.facts.DiskTotalBytes)
			if !low {
				return nil
			}
			return &hit{severity: level}
		},
	},
	{
		id: "hygiene.staging", area: AreaStorage, severity: SeverityInfo,
		title: "Leftover temporary files",
		body:  "Old temporary files use disk space. Clean them up.",
		label: "Open Hygiene", route: "/cluster/hygiene",
		check: func(in input) *hit {
			return hitIf(stagingByteLimit < in.facts.StagingBytes || stagingCountLimit < in.facts.StagingCount)
		},
	},
	{
		id: "hygiene.no_owner", area: AreaStorage, severity: SeverityInfo,
		title: "{n} files have no owner",
		body:  "Files without an owner cannot be assigned to a user. Review them in Hygiene.",
		label: "Open Hygiene", route: "/cluster/hygiene",
		check: func(in input) *hit {
			if 0 == in.facts.FilesNoOwner {
				return nil
			}
			return &hit{n: in.facts.FilesNoOwner}
		},
	},
	{
		id: "hygiene.no_etag", area: AreaStorage, severity: SeverityInfo,
		title: "{n} files have no version tag",
		body:  "Version tags help detect conflicting edits. Review these files in Hygiene.",
		label: "Open Hygiene", route: "/cluster/hygiene",
		check: func(in input) *hit {
			if 0 == in.facts.FilesNoETag {
				return nil
			}
			return &hit{n: in.facts.FilesNoETag}
		},
	},
	{
		id: "migrations.dirty", area: AreaStorage, severity: SeverityCritical,
		title: "File index update did not finish",
		body:  "The index is in a partial state. Open Hygiene for the next step.",
		label: "Open Hygiene", route: "/cluster/hygiene",
		check: func(in input) *hit {
			return hitIf(in.facts.MigrationsDirty)
		},
	},
	{
		id: "timeouts.zero", area: AreaRuntime, severity: SeverityInfo,
		title: "Requests have no time limit",
		body:  "Slow requests can hold connections forever. Set read and write timeouts.",
		label: "Change setting", route: "/settings/limits", setting: "server.read_timeout",
		check: func(in input) *hit {
			return hitIf(0 == in.cfg.Server.ReadTimeout || 0 == in.cfg.Server.WriteTimeout)
		},
	},
	{
		id: "timeouts.short_for_uploads", area: AreaRuntime, severity: SeverityInfo,
		title: "Large uploads may stop early",
		body:  "The read timeout is shorter than large uploads need. Use the Large files preset.",
		label: "Change setting", route: "/settings/limits", setting: "server.read_timeout",
		check: func(in input) *hit {
			return hitIf(0 < in.cfg.Server.ReadTimeout && largeUploadReadTimeout > in.cfg.Server.ReadTimeout)
		},
	},
	{
		id: "proxy.shared_bucket", area: AreaRuntime, severity: SeverityInfo,
		title: "All users may share one request limit",
		body:  "Most requests come from one address. Trust your proxy address so limits apply per user.",
		label: "Open Limits", route: "/cluster/limits",
		check: func(in input) *hit {
			return hitIf(sharedClientShare < in.facts.TopClientShare)
		},
	},
	{
		id: "links.failed_opens", area: AreaRuntime, severity: SeverityWarning,
		title: "Many download links fail",
		body:  "More than 1 in 5 link opens failed in the last hour. Check the link settings and expiry.",
		label: "Open Sharing", route: "/sharing",
		check: func(in input) *hit {
			return hitIf(in.facts.LinkOpens1h < 5*in.facts.LinkFailedOpens1h)
		},
	},
	{
		id: "auth.failures_spike", area: AreaSecurity, severity: SeverityWarning,
		title: "Many failed sign-ins",
		body:  "More than 100 sign-ins failed in 10 minutes. Check for a bad key or an attacker.",
		label: "Open Sign-ins", route: "/activity/sign-ins",
		check: func(in input) *hit {
			return hitIf(authFailureLimit < in.facts.AuthFailures10m)
		},
	},
	{
		id: "config.pending_restart", area: AreaConfig, severity: SeverityWarning,
		title: "{n} saved changes need a restart",
		body:  "Saved settings do not run until the server restarts. Restart when it is safe.",
		label: "Open Config file", route: "/settings/file",
		check: func(in input) *hit {
			if 0 == in.facts.PendingRestart {
				return nil
			}
			return &hit{n: in.facts.PendingRestart}
		},
	},
}

func findRule(id string) (rule, bool) {
	for _, entry := range catalog {
		if id == entry.id {
			return entry, true
		}
	}
	return rule{}, false
}

func natsPlainTransport(cfg *config.AppConfig) bool {
	if !cfg.NATS.Enabled {
		return false
	}
	if config.NATSModeExternal == cfg.NATS.Mode {
		for _, raw := range cfg.NATS.External.URLs {
			if !strings.HasPrefix(strings.ToLower(strings.TrimSpace(raw)), "tls://") && "" == cfg.NATS.External.TLSCAFile {
				return true
			}
		}
		return false
	}
	clustered := "" != cfg.NATS.ClusterListen || 0 < len(cfg.NATS.Routes) || cfg.NATS.RoutesFromPeers
	return clustered && "" == cfg.NATS.TLS.CertFile
}
