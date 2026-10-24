#!/usr/bin/env bash
set -euo pipefail

usage() {
	cat <<'EOF'
Load and profile callfs by hand. Never runs in CI or as part of go test.

usage: tests/perf/profile.sh [options] [-- extra loadgen flags]

  --stack local|full|nats|external   what to run against (default: local)
       local     one instance: sqlite, local locks, local disk, split storage, trash
       full      two instances: postgres index, redis locks, minio S3, split storage
                 on S3, backup copies to S3, cross-instance proxy (uses an already
                 running docker; the script never starts docker itself)
       nats      three instances with embedded NATS JetStream: NATS index, NATS locks
                 and split storage; plain callfs processes, nothing else is started.
                 NATS_SYNC=always|interval and LOCALFS_SYNC=full|barrier|none tune durability
       external  an already running server: set CALLFS_PERF_URL, CALLFS_PERF_PPROF
                 (metrics base URL with metrics.profiling on), CALLFS_PERF_ADMIN_KEY,
                 optional CALLFS_PERF_USER_KEYS, CALLFS_PERF_PEERS, CALLFS_PERF_FEATURES
  --scenarios a,b,c     scenario names (default: all; see --list)
  --group name          only one group: files tree sharing transfers cluster admin contention mixed
  --duration 30s        measured time per scenario (max 120s)
  --concurrency 16      parallel workers
  --large 64MiB-bytes   large file size in bytes (default 67108864)
  --files 200           seeded files per scenario
  --sessions            drive user traffic through browser sessions and CSRF
  --out dir             results root (default: tests/perf/results, ignored by git)
  --keep                leave servers and containers running afterwards
  --console             build with the web console, turn it on and keep the servers
                        running (implies --keep)
  --list                list scenarios and exit

Output: <out>/<run>/summary.txt (slowest operations), hotspots.txt (top CPU,
allocation, lock and blocking functions per scenario), and per scenario the raw
profiles (*.pb.gz) plus pprof top reports. Open one with:
  go tool pprof -http=: <out>/<run>/<scenario>/node0-cpu.pb.gz
EOF
}

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
PERF="$ROOT/tests/perf"
STACK=local
SCENARIOS=all
GROUP=""
DURATION=30s
CONCURRENCY=16
LARGE=67108864
FILES=200
SESSIONS=""
OUT="$PERF/results"
KEEP=0
CONSOLE=0
EXTRA=()

while [ 0 -lt $# ]; do
	case "$1" in
	--stack) STACK="$2"; shift 2 ;;
	--scenarios) SCENARIOS="$2"; shift 2 ;;
	--group) GROUP="$2"; shift 2 ;;
	--duration) DURATION="$2"; shift 2 ;;
	--concurrency) CONCURRENCY="$2"; shift 2 ;;
	--large) LARGE="$2"; shift 2 ;;
	--files) FILES="$2"; shift 2 ;;
	--sessions) SESSIONS="-sessions"; shift ;;
	--out) OUT="$2"; shift 2 ;;
	--keep) KEEP=1; shift ;;
	--console) CONSOLE=1; KEEP=1; shift ;;
	--list) go run "$PERF/loadgen" -list; exit 0 ;;
	-h | --help) usage; exit 0 ;;
	--) shift; EXTRA=("$@"); break ;;
	*) echo "unknown option: $1" >&2; usage >&2; exit 2 ;;
	esac
done

WORK="$(mktemp -d "${TMPDIR:-/tmp}/callfs-perf.XXXXXX")"
BIN="$WORK/callfs"
LOADGEN="$WORK/loadgen"
PIDS=()
CONTAINERS=()
ADMIN_KEY="$(openssl rand -hex 24)"
USER_KEYS="$(openssl rand -hex 24),$(openssl rand -hex 24)"
INTERNAL_SECRET="$(openssl rand -hex 24)"
LINK_SECRET="$(openssl rand -hex 24)"
PG_IMAGE="postgres:17@sha256:b8faeff7712663c2773a190181e1246e37e364b887611c04658c28b44e2408fe"
REDIS_IMAGE="redis:7@sha256:44960b3100c48159a66d28c18b0588c193492e25f7551d60e51dd464fc3eca5b"
MINIO_IMAGE="cgr.dev/chainguard/minio@sha256:d88920d6a50bd0b088223b688be3b76c63e6205dbe494437f67ba1240ea12880"
MC_IMAGE="cgr.dev/chainguard/minio-client@sha256:72f9978d415ea8294d78051fb3aee2fd258d449237633f77f100b5367bb5225d"
NET="callfs-perf-$$"

free_port() {
	local port
	touch "$WORK/ports"
	while :; do
		port=$((20000 + RANDOM % 30000))
		grep -qx "$port" "$WORK/ports" && continue
		(exec 3<>"/dev/tcp/127.0.0.1/$port") 2>/dev/null && continue
		echo "$port" >>"$WORK/ports"
		echo "$port"
		return
	done
}

cleanup() {
	if [ 1 -eq "$KEEP" ]; then
		echo "kept running. work dir: $WORK  pids: ${PIDS[*]:-}  containers: ${CONTAINERS[*]:-}"
		if [ 1 -eq "$CONSOLE" ]; then
			echo "console: ${URL:-}/ui/  admin key: $ADMIN_KEY"
			echo "other servers: ${PEERS:-none}"
		fi
		return
	fi
	for pid in "${PIDS[@]:-}"; do
		[ -n "$pid" ] && kill "$pid" 2>/dev/null || true
	done
	for pid in "${PIDS[@]:-}"; do
		[ -n "$pid" ] && wait "$pid" 2>/dev/null || true
	done
	if [ 0 -lt "${#CONTAINERS[@]}" ]; then
		docker rm -f "${CONTAINERS[@]}" >/dev/null 2>&1 || true
		docker network rm "$NET" >/dev/null 2>&1 || true
	fi
	rm -rf "$WORK"
}
trap cleanup EXIT

build() {
	echo "building callfs and loadgen"
	local tags=()
	[ 1 -eq "$CONSOLE" ] && tags=(-tags ui)
	(cd "$ROOT" && go build ${tags[@]+"${tags[@]}"} -o "$BIN" ./cmd && go build -o "$LOADGEN" ./tests/perf/loadgen)
}

wait_http() {
	local url="$1" tries=120
	until curl -fsS -o /dev/null "$url" 2>/dev/null; do
		tries=$((tries - 1))
		if [ 0 -ge "$tries" ]; then
			echo "timed out waiting for $url" >&2
			return 1
		fi
		sleep 0.5
	done
}

users_yaml() {
	local i=0
	echo "  api_key_users:"
	echo "    perfadmin: \"$ADMIN_KEY\""
	IFS=, read -ra keys <<<"$USER_KEYS"
	for key in "${keys[@]}"; do
		i=$((i + 1))
		echo "    perfuser$i: \"$key\""
	done
}

common_yaml() {
	local id="$1" port="$2" mport="$3" data="$4"
	cat <<EOF
server:
  listen_addr: "127.0.0.1:$port"
  protocol: "http"
  external_url: "http://127.0.0.1:$port"
  read_timeout: "5m"
  write_timeout: "10m"
  file_op_timeout: "5m"
  metadata_op_timeout: "30s"
  rate_limit:
    download_rps: 100000
    download_burst: 100000
    link_generate_rps: 100000
    link_generate_burst: 100000
    session_rps: 100000
    session_burst: 100000
    max_active_links_per_user: 1000000
auth:
$(users_yaml)
  admin_users: ["perfadmin"]
  internal_proxy_secret: "$INTERNAL_SECRET"
  single_use_link_secret: "$LINK_SECRET"
log:
  level: "warn"
  format: "json"
metrics:
  listen_addr: "127.0.0.1:$mport"
  profiling: true
trash:
  enabled: true
ui:
  enabled: $([ 1 -eq "$CONSOLE" ] && echo true || echo false)
instance_discovery:
  instance_id: "$id"
EOF
	[ -n "$data" ] && mkdir -p "$data"
}

start_node() {
	local name="$1" config="$2"
	(cd "$WORK" && exec "$BIN" server --config "$config" >"$WORK/$name.log" 2>&1) &
	PIDS+=("$!")
}

docker_up() {
	docker info >/dev/null 2>&1 || { echo "--stack $STACK needs a docker daemon that is already running; this script never starts one" >&2; exit 1; }
	REDIS_PORT="$(free_port)"
	PG_PORT="$(free_port)"
	MINIO_PORT="$(free_port)"
	docker network create "$NET" >/dev/null
	local want="$1"
	if [[ "$want" == *redis* ]]; then
		CONTAINERS+=("$NET-redis")
		docker run -d --name "$NET-redis" --network "$NET" -p "127.0.0.1:$REDIS_PORT:6379" "$REDIS_IMAGE" >/dev/null
	fi
	if [[ "$want" == *postgres* ]]; then
		CONTAINERS+=("$NET-postgres")
		docker run -d --name "$NET-postgres" --network "$NET" -p "127.0.0.1:$PG_PORT:5432" \
			-e POSTGRES_USER=callfs -e POSTGRES_PASSWORD=perfpass -e POSTGRES_DB=callfs "$PG_IMAGE" >/dev/null
		until docker exec "$NET-postgres" pg_isready -U callfs >/dev/null 2>&1; do sleep 0.5; done
	fi
	if [[ "$want" == *minio* ]]; then
		CONTAINERS+=("$NET-minio")
		docker run -d --name "$NET-minio" --network "$NET" -p "127.0.0.1:$MINIO_PORT:9000" \
			-e MINIO_ROOT_USER=perfadmin -e MINIO_ROOT_PASSWORD=perfpass123 "$MINIO_IMAGE" server /data >/dev/null
		wait_http "http://127.0.0.1:$MINIO_PORT/minio/health/live"
		docker run --rm --network "$NET" -e "MC_HOST_local=http://perfadmin:perfpass123@$NET-minio:9000" \
			"$MC_IMAGE" mb --ignore-existing local/callfs-perf >/dev/null
	fi
}

stack_local() {
	local cfg="$WORK/node0.yaml" data="$WORK/node0" port mport
	port="$(free_port)"
	mport="$(free_port)"
	{
		common_yaml node-0 "$port" "$mport" "$data"
		cat <<EOF
backend:
  default_backend: "localfs"
  localfs_root_path: "$data/files"
metadata_store:
  type: "sqlite"
  sqlite_path: "$data/callfs.sqlite3"
dlm:
  type: "local"
erasure:
  enabled: true
  data_shards: 4
  parity_shards: 2
  shard_backend: "localfs"
EOF
	} >"$cfg"
	mkdir -p "$data/files"
	start_node node0 "$cfg"
	URL="http://127.0.0.1:$port"
	PPROF="http://127.0.0.1:$mport"
	PEERS=""
	FEATURES="erasure"
}

stack_full() {
	docker_up "redis postgres minio"
	PEERS=""
	PPROF=""
	local ports=("$(free_port)" "$(free_port)") mports=("$(free_port)" "$(free_port)")
	for i in 0 1; do
		local cfg="$WORK/node$i.yaml" data="$WORK/node$i" port="${ports[$i]}" mport="${mports[$i]}" other=$((1 - i))
		{
			common_yaml "node-$i" "$port" "$mport" "$data"
			cat <<EOF
  peer_endpoints:
    node-$other: "http://127.0.0.1:${ports[$other]}"
backend:
  default_backend: "localfs"
  localfs_root_path: "$data/files"
  s3_access_key: "perfadmin"
  s3_secret_key: "perfpass123"
  s3_region: "us-east-1"
  s3_bucket_name: "callfs-perf"
  s3_endpoint: "http://127.0.0.1:$MINIO_PORT"
  s3_server_side_encryption: ""
  s3_acl: ""
metadata_store:
  type: "postgres"
  dsn: "postgres://callfs:perfpass@127.0.0.1:$PG_PORT/callfs?sslmode=disable"
dlm:
  type: "redis"
  redis_addr: "127.0.0.1:$REDIS_PORT"
ha:
  replication_enabled: true
  replica_backend: "s3"
  require_replica_success: false
erasure:
  enabled: true
  data_shards: 4
  parity_shards: 2
  shard_backend: "s3"
EOF
		} >"$cfg"
		mkdir -p "$data/files"
		start_node "node$i" "$cfg"
		PPROF="${PPROF:+$PPROF,}http://127.0.0.1:$mport"
		[ 1 -eq "$i" ] && PEERS="http://127.0.0.1:$port"
	done
	URL="http://127.0.0.1:${ports[0]}"
	FEATURES="erasure,replication,s3,cluster"
}

stack_nats() {
	PEERS=""
	PPROF=""
	local ports=("$(free_port)" "$(free_port)" "$(free_port)") mports=("$(free_port)" "$(free_port)" "$(free_port)") cports=("$(free_port)" "$(free_port)" "$(free_port)")
	for i in 0 1 2; do
		local cfg="$WORK/node$i.yaml" data="$WORK/node$i" port="${ports[$i]}" mport="${mports[$i]}"
		local api_peers="" routes=""
		for j in 0 1 2; do
			[ "$i" -eq "$j" ] && continue
			api_peers+="    node-$j: \"http://127.0.0.1:${ports[$j]}\"\n"
			routes+="    - \"nats-route://127.0.0.1:${cports[$j]}\"\n"
		done
		{
			common_yaml "node-$i" "$port" "$mport" "$data"
			printf '  peer_endpoints:\n%b' "$api_peers"
			cat <<EOF
backend:
  default_backend: "localfs"
  localfs_root_path: "$data/files"
  localfs_sync: "${LOCALFS_SYNC:-full}"
metadata_store:
  type: "nats"
dlm:
  type: "nats"
nats:
  enabled: true
  mode: "embedded"
  server_name: "node-$i"
  cluster_name: "callfs-perf"
  cluster_listen: "127.0.0.1:${cports[$i]}"
  routes_from_peers: false
  jetstream:
    store_dir: "$data/nats"
    sync: "${NATS_SYNC:-always}"
  metadata:
    replicas: 3
  locks:
    replicas: 3
  streams:
    replicas: 3
EOF
			printf '  routes:\n%b' "$routes"
			cat <<EOF
erasure:
  enabled: true
  data_shards: 2
  parity_shards: 1
  shard_backend: "localfs"
EOF
		} >"$cfg"
		mkdir -p "$data/files" "$data/nats"
		start_node "node$i" "$cfg"
		PPROF="${PPROF:+$PPROF,}http://127.0.0.1:$mport"
		[ 0 -lt "$i" ] && PEERS="${PEERS:+$PEERS,}http://127.0.0.1:$port"
	done
	URL="http://127.0.0.1:${ports[0]}"
	FEATURES="nats,cluster,erasure"
}

stack_external() {
	URL="${CALLFS_PERF_URL:?set CALLFS_PERF_URL}"
	PPROF="${CALLFS_PERF_PPROF:-}"
	ADMIN_KEY="${CALLFS_PERF_ADMIN_KEY:?set CALLFS_PERF_ADMIN_KEY}"
	USER_KEYS="${CALLFS_PERF_USER_KEYS:-}"
	PEERS="${CALLFS_PERF_PEERS:-}"
	FEATURES="${CALLFS_PERF_FEATURES:-}"
}

analyze() {
	local run="$1" report="$1/hotspots.txt"
	: >"$report"
	for dir in "$run"/*/; do
		local scenario
		scenario="$(basename "$dir")"
		for profile in "$dir"*.pb.gz; do
			[ -e "$profile" ] || continue
			local base kind args=()
			base="$(basename "$profile" .pb.gz)"
			kind="${base#*-}"
			case "$kind" in
			allocs) args=(-sample_index=alloc_space) ;;
			heap) args=(-sample_index=inuse_space) ;;
			mutex | block) args=(-sample_index=delay) ;;
			esac
			go tool pprof -top -nodecount=40 "${args[@]}" "$profile" >"$dir$base.top.txt" 2>/dev/null || continue
			go tool pprof -top -cum -nodecount=25 "${args[@]}" "$profile" >"$dir$base.cum.txt" 2>/dev/null || true
			case "$kind" in
			cpu | allocs | mutex | block)
				{
					echo "== $scenario  $base (flat, top 12) =="
					grep -E '^\s+[0-9.]+[a-zA-Z]*\s' "$dir$base.top.txt" | head -12  || true
					echo
				} >>"$report"
				;;
			esac
		done
	done
}

case "$STACK" in
local | full | nats) build ;;
external) (cd "$ROOT" && go build -o "$LOADGEN" ./tests/perf/loadgen) ;;
*) echo "unknown stack: $STACK" >&2; exit 2 ;;
esac

"stack_$STACK"

if [ "external" != "$STACK" ]; then
	echo "waiting for callfs ($STACK)"
	wait_http "$URL/health"
	for peer in ${PEERS//,/ }; do wait_http "$peer/health"; done
	if [ "nats" = "$STACK" ]; then
		for node in "$URL" ${PEERS//,/ }; do
			tries=120
			until curl -fsS "$node/health/ready" 2>/dev/null | grep -q '"ok"'; do
				tries=$((tries - 1))
				[ 0 -ge "$tries" ] && { echo "timed out waiting for $node to become ready" >&2; exit 1; }
				sleep 0.5
			done
		done
	fi
fi

mkdir -p "$OUT"
LOG="$WORK/loadgen.out"
CALLFS_LOADGEN_ADMIN_KEY="$ADMIN_KEY" CALLFS_LOADGEN_USER_KEYS="$USER_KEYS" "$LOADGEN" \
	-url "$URL" -pprof "$PPROF" -peers "$PEERS" -features "$FEATURES" \
	-scenarios "$SCENARIOS" ${GROUP:+-group "$GROUP"} -duration "$DURATION" \
	-concurrency "$CONCURRENCY" -large "$LARGE" -files "$FILES" -out "$OUT" $SESSIONS \
	${EXTRA[@]+"${EXTRA[@]}"} | tee "$LOG"

RUN="$(sed -n 's/^results: //p' "$LOG" | tail -1)"
[ -n "$RUN" ] || { echo "loadgen produced no results" >&2; exit 1; }
for log in "$WORK"/node*.log; do
	[ -e "$log" ] && cp "$log" "$RUN/"
done
echo "analyzing profiles"
analyze "$RUN"
echo
head -40 "$RUN/summary.txt"
echo
echo "summary:  $RUN/summary.txt"
echo "hotspots: $RUN/hotspots.txt"
