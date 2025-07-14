# Clustering & Distribution

This document covers deploying CallFS in distributed, clustered configurations for high availability, scalability, and geographic distribution.

## Architecture Overview

### Distributed Components

CallFS supports horizontal scaling through several distributed components:

1. **Stateless Application Instances**: Multiple CallFS servers sharing common storage
2. **Shared Metadata Store**: Centralized PostgreSQL database for file metadata
3. **Distributed Locking**: Redis-based coordination for concurrent operations
4. **Internal Proxy Network**: Peer-to-peer file sharing between instances
5. **Load Balancer**: Traffic distribution across instances

### Deployment Patterns

#### Single-Region Cluster
```
          ┌─────────────┐
          │Load Balancer│
          └─────────────┘
                  │
         ┌────────┼────────┐
         │        │        │
    ┌─────────┐ ┌─────────┐ ┌─────────┐
    │CallFS-1 │ │CallFS-2 │ │CallFS-3 │
    └─────────┘ └─────────┘ └─────────┘
         │        │        │
         └────────┼────────┘
                  │
         ┌────────┼────────┐
         │        │        │
    ┌─────────┐ ┌─────────┐ ┌─────────┐
    │PostgreSQL│ │  Redis  │ │   S3    │
    └─────────┘ └─────────┘ └─────────┘
```

#### Multi-Region Distribution
```
    Region A              Region B              Region C
┌─────────────────┐  ┌─────────────────┐  ┌─────────────────┐
│   CallFS-A1     │  │   CallFS-B1     │  │   CallFS-C1     │
│   CallFS-A2     │◄─┤   CallFS-B2     │◄─┤   CallFS-C2     │
│                 │  │                 │  │                 │
│  PostgreSQL-A   │  │  PostgreSQL-B   │  │  PostgreSQL-C   │
│    Redis-A      │  │    Redis-B      │  │    Redis-C      │
└─────────────────┘  └─────────────────┘  └─────────────────┘
```

## Configuration for Clustering

### Basic Cluster Configuration

#### Instance 1 Configuration
```yaml
server:
  listen_addr: ":8443"
  external_url: "https://callfs-01.cluster.local:8443"

auth:
  api_keys:
    - "cluster-api-key-1"
  internal_proxy_secret: "shared-cluster-secret"
  single_use_link_secret: "shared-link-secret"

metadata_store:
  dsn: "postgres://callfs:password@postgres-cluster:5432/callfs?sslmode=require"

dlm:
  redis_addr: "redis-cluster:6379"
  redis_password: "cluster_redis_password"

backend:
  localfs_root_path: "/var/lib/callfs"
  s3_bucket_name: "callfs-cluster-storage"
  s3_region: "us-west-2"

instance_discovery:
  instance_id: "callfs-01"
  peer_endpoints:
    callfs-02: "https://callfs-02.cluster.local:8443"
    callfs-03: "https://callfs-03.cluster.local:8443"
```

#### Instance 2 Configuration
```yaml
server:
  listen_addr: ":8443"
  external_url: "https://callfs-02.cluster.local:8443"

# ... same auth, metadata_store, dlm, backend config ...

instance_discovery:
  instance_id: "callfs-02"
  peer_endpoints:
    callfs-01: "https://callfs-01.cluster.local:8443"
    callfs-03: "https://callfs-03.cluster.local:8443"
```

### Environment Variables for Clustering

```bash
# Common cluster configuration
export CALLFS_AUTH_INTERNAL_PROXY_SECRET="shared-cluster-secret"
export CALLFS_AUTH_SINGLE_USE_LINK_SECRET="shared-link-secret"
export CALLFS_METADATA_STORE_DSN="postgres://callfs:password@postgres-cluster:5432/callfs?sslmode=require"
export CALLFS_DLM_REDIS_ADDR="redis-cluster:6379"
export CALLFS_DLM_REDIS_PASSWORD="cluster_redis_password"

# Instance-specific configuration
export CALLFS_INSTANCE_DISCOVERY_INSTANCE_ID="callfs-01"
export CALLFS_SERVER_EXTERNAL_URL="https://callfs-01.cluster.local:8443"
export CALLFS_INSTANCE_DISCOVERY_PEER_ENDPOINTS='{"callfs-02":"https://callfs-02.cluster.local:8443","callfs-03":"https://callfs-03.cluster.local:8443"}'
```

## Database Clustering

### PostgreSQL High Availability

#### Primary-Replica Setup
```yaml
# Primary database configuration
metadata_store:
  dsn: "postgres://callfs:password@postgres-primary:5432/callfs?sslmode=require"

# Read replicas for read-heavy workloads (future enhancement)
# metadata_store:
#   read_dsn: "postgres://callfs:password@postgres-replica:5432/callfs?sslmode=require"
```

#### PostgreSQL Cluster with Patroni
```yaml
# patroni.yml
scope: callfs-cluster
namespace: /callfs/
name: postgres-01

restapi:
  listen: 0.0.0.0:8008
  connect_address: postgres-01:8008

etcd:
  host: etcd-cluster:2379

bootstrap:
  dcs:
    ttl: 30
    loop_wait: 10
    retry_timeout: 30
    maximum_lag_on_failover: 1048576
    postgresql:
      use_pg_rewind: true
      parameters:
        max_connections: 200
        shared_buffers: 256MB
        effective_cache_size: 1GB
        maintenance_work_mem: 64MB
        checkpoint_completion_target: 0.9
        wal_buffers: 16MB
        default_statistics_target: 100
        random_page_cost: 1.1
        effective_io_concurrency: 200

  initdb:
    - encoding: UTF8
    - data-checksums

postgresql:
  listen: 0.0.0.0:5432
  connect_address: postgres-01:5432
  data_dir: /var/lib/postgresql/data
  pgpass: /tmp/pgpass
  authentication:
    replication:
      username: replicator
      password: replica_password
    superuser:
      username: postgres
      password: postgres_password
  parameters:
    unix_socket_directories: '/var/run/postgresql'
```

#### Connection Pooling with PgBouncer
```ini
# pgbouncer.ini
[databases]
callfs = host=postgres-cluster port=5432 dbname=callfs

[pgbouncer]
listen_port = 5432
listen_addr = *
auth_type = md5
auth_file = /etc/pgbouncer/userlist.txt
logfile = /var/log/pgbouncer/pgbouncer.log
pidfile = /var/run/pgbouncer/pgbouncer.pid
admin_users = admin
pool_mode = transaction
server_reset_query = DISCARD ALL
max_client_conn = 1000
default_pool_size = 100
max_db_connections = 200
```

### Redis Clustering

#### Redis Cluster Setup
```bash
# Create Redis cluster
redis-cli --cluster create \
  redis-01:7000 redis-02:7000 redis-03:7000 \
  redis-04:7000 redis-05:7000 redis-06:7000 \
  --cluster-replicas 1
```

#### Redis Sentinel Configuration
```conf
# sentinel.conf
port 26379
sentinel monitor callfs-redis redis-master 6379 2
sentinel down-after-milliseconds callfs-redis 30000
sentinel parallel-syncs callfs-redis 1
sentinel failover-timeout callfs-redis 180000
sentinel auth-pass callfs-redis redis_password
```

#### CallFS Redis Cluster Configuration
```yaml
dlm:
  redis_addr: "redis-cluster-node1:7000,redis-cluster-node2:7000,redis-cluster-node3:7000"
  redis_password: "cluster_password"
```

## Load Balancing

### HAProxy Configuration

#### Basic Load Balancer
```haproxy
# haproxy.cfg
global
    daemon
    maxconn 4096

defaults
    mode http
    timeout connect 5000ms
    timeout client 50000ms
    timeout server 50000ms
    option httplog

frontend callfs_frontend
    bind *:443 ssl crt /etc/ssl/certs/callfs.pem
    bind *:80
    redirect scheme https if !{ ssl_fc }
    
    # Health check bypass
    acl health_check path /health
    use_backend callfs_health if health_check
    
    default_backend callfs_backend

backend callfs_backend
    balance roundrobin
    option ssl-hello-chk
    
    # Health checks
    option httpchk GET /health
    http-check expect status 200
    
    # Server definitions
    server callfs-01 callfs-01.cluster.local:8443 check ssl verify none
    server callfs-02 callfs-02.cluster.local:8443 check ssl verify none
    server callfs-03 callfs-03.cluster.local:8443 check ssl verify none

backend callfs_health
    option httpchk GET /health
    server callfs-01 callfs-01.cluster.local:8443 check ssl verify none
    server callfs-02 callfs-02.cluster.local:8443 check ssl verify none
    server callfs-03 callfs-03.cluster.local:8443 check ssl verify none

# Statistics
listen stats
    bind *:8404
    option httplog
    stats enable
    stats uri /stats
    stats refresh 30s
    stats admin if LOCALHOST
```

#### Advanced HAProxy with Sticky Sessions
```haproxy
frontend callfs_frontend
    bind *:443 ssl crt /etc/ssl/certs/callfs.pem
    
    # Extract API key for session affinity
    http-request set-header X-API-Key %[req.hdr(authorization)]
    
    default_backend callfs_backend

backend callfs_backend
    balance roundrobin
    
    # Sticky sessions based on API key
    stick-table type string len 64 size 10k expire 30m
    stick on req.hdr(x-api-key)
    
    server callfs-01 callfs-01.cluster.local:8443 check ssl verify none
    server callfs-02 callfs-02.cluster.local:8443 check ssl verify none
    server callfs-03 callfs-03.cluster.local:8443 check ssl verify none
```

### NGINX Load Balancer

#### Basic NGINX Configuration
```nginx
# nginx.conf
upstream callfs_cluster {
    least_conn;
    server callfs-01.cluster.local:8443 max_fails=3 fail_timeout=30s;
    server callfs-02.cluster.local:8443 max_fails=3 fail_timeout=30s;
    server callfs-03.cluster.local:8443 max_fails=3 fail_timeout=30s;
}

server {
    listen 443 ssl http2;
    server_name callfs.yourdomain.com;
    
    ssl_certificate /etc/ssl/certs/callfs.crt;
    ssl_certificate_key /etc/ssl/private/callfs.key;
    ssl_protocols TLSv1.2 TLSv1.3;
    ssl_ciphers ECDHE+AESGCM:ECDHE+CHACHA20:DHE+AESGCM:DHE+CHACHA20:!aNULL:!MD5:!DSS;
    
    # Health check endpoint
    location /health {
        access_log off;
        proxy_pass https://callfs_cluster;
        proxy_ssl_verify off;
    }
    
    # Main proxy configuration
    location / {
        proxy_pass https://callfs_cluster;
        proxy_ssl_verify off;
        
        # Headers
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        
        # Timeouts
        proxy_connect_timeout 30s;
        proxy_send_timeout 60s;
        proxy_read_timeout 60s;
        
        # Buffer settings for large uploads
        client_max_body_size 10G;
        proxy_request_buffering off;
        proxy_buffering off;
    }
}

server {
    listen 80;
    server_name callfs.yourdomain.com;
    return 301 https://$server_name$request_uri;
}
```

#### IP Hash Load Balancing
```nginx
upstream callfs_cluster {
    ip_hash;  # Route requests from same IP to same backend
    server callfs-01.cluster.local:8443;
    server callfs-02.cluster.local:8443;
    server callfs-03.cluster.local:8443;
}
```

## Container Orchestration

### Docker Swarm Deployment

#### Docker Compose Stack
```yaml
# docker-stack.yml
version: '3.8'

services:
  callfs:
    image: callfs:latest
    deploy:
      replicas: 3
      placement:
        constraints:
          - node.role == worker
      restart_policy:
        condition: on-failure
        delay: 5s
        max_attempts: 3
      resources:
        limits:
          memory: 2G
          cpus: '1.0'
        reservations:
          memory: 1G
          cpus: '0.5'
    environment:
      - CALLFS_METADATA_STORE_DSN=postgres://callfs:password@postgres:5432/callfs?sslmode=require
      - CALLFS_DLM_REDIS_ADDR=redis:6379
      - CALLFS_AUTH_INTERNAL_PROXY_SECRET=swarm-cluster-secret
    configs:
      - source: callfs_config
        target: /app/config.yaml
    secrets:
      - callfs_api_keys
      - callfs_tls_cert
      - callfs_tls_key
    networks:
      - callfs_network
    ports:
      - "8443:8443"
    volumes:
      - callfs_data:/var/lib/callfs

  postgres:
    image: postgres:14
    deploy:
      replicas: 1
      placement:
        constraints:
          - node.labels.postgres == true
    environment:
      - POSTGRES_DB=callfs
      - POSTGRES_USER=callfs
      - POSTGRES_PASSWORD_FILE=/run/secrets/postgres_password
    secrets:
      - postgres_password
    networks:
      - callfs_network
    volumes:
      - postgres_data:/var/lib/postgresql/data

  redis:
    image: redis:7-alpine
    deploy:
      replicas: 1
    command: ["redis-server", "--requirepass", "$(cat /run/secrets/redis_password)"]
    secrets:
      - redis_password
    networks:
      - callfs_network
    volumes:
      - redis_data:/data

networks:
  callfs_network:
    driver: overlay
    attachable: true

volumes:
  callfs_data:
  postgres_data:
  redis_data:

configs:
  callfs_config:
    file: ./config.yaml

secrets:
  callfs_api_keys:
    external: true
  callfs_tls_cert:
    external: true
  callfs_tls_key:
    external: true
  postgres_password:
    external: true
  redis_password:
    external: true
```

#### Deploy Stack
```bash
# Create secrets
echo "api-key-1,api-key-2" | docker secret create callfs_api_keys -
echo "postgres_password" | docker secret create postgres_password -
echo "redis_password" | docker secret create redis_password -
docker secret create callfs_tls_cert cert.pem
docker secret create callfs_tls_key key.pem

# Deploy stack
docker stack deploy -c docker-stack.yml callfs-cluster

# Scale services
docker service scale callfs-cluster_callfs=5

# Update service
docker service update --image callfs:v2.0 callfs-cluster_callfs
```

### Kubernetes Deployment

#### Namespace and ConfigMap
```yaml
# namespace.yaml
apiVersion: v1
kind: Namespace
metadata:
  name: callfs
---
# configmap.yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: callfs-config
  namespace: callfs
data:
  config.yaml: |
    server:
      listen_addr: ":8443"
      cert_file: "/etc/tls/tls.crt"
      key_file: "/etc/tls/tls.key"
    auth:
      api_keys:
        - "api-key-1"
      internal_proxy_secret: "k8s-cluster-secret"
      single_use_link_secret: "k8s-link-secret"
    metadata_store:
      dsn: "postgres://callfs:password@postgres-service:5432/callfs?sslmode=require"
    dlm:
      redis_addr: "redis-service:6379"
    backend:
      s3_bucket_name: "callfs-k8s-storage"
      s3_region: "us-west-2"
    log:
      level: "info"
      format: "json"
```

#### Secrets
```yaml
# secrets.yaml
apiVersion: v1
kind: Secret
metadata:
  name: callfs-secrets
  namespace: callfs
type: Opaque
data:
  api-keys: YXBpLWtleS0xLGFwaS1rZXktMg==  # base64 encoded
  internal-secret: azhzLWNsdXN0ZXItc2VjcmV0  # base64 encoded
  link-secret: azhzLWxpbmstc2VjcmV0  # base64 encoded
---
apiVersion: v1
kind: Secret
metadata:
  name: callfs-tls
  namespace: callfs
type: kubernetes.io/tls
data:
  tls.crt: LS0tLS1CRUdJTi... # base64 encoded cert
  tls.key: LS0tLS1CRUdJTi... # base64 encoded key
```

#### Deployment
```yaml
# deployment.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: callfs
  namespace: callfs
  labels:
    app: callfs
spec:
  replicas: 3
  strategy:
    type: RollingUpdate
    rollingUpdate:
      maxSurge: 1
      maxUnavailable: 0
  selector:
    matchLabels:
      app: callfs
  template:
    metadata:
      labels:
        app: callfs
      annotations:
        prometheus.io/scrape: "true"
        prometheus.io/port: "8443"
        prometheus.io/path: "/metrics"
    spec:
      containers:
      - name: callfs
        image: callfs:latest
        ports:
        - containerPort: 8443
          name: https
        env:
        - name: CALLFS_INSTANCE_DISCOVERY_INSTANCE_ID
          valueFrom:
            fieldRef:
              fieldPath: metadata.name
        - name: CALLFS_AUTH_API_KEYS
          valueFrom:
            secretKeyRef:
              name: callfs-secrets
              key: api-keys
        volumeMounts:
        - name: config
          mountPath: /app/config.yaml
          subPath: config.yaml
        - name: tls
          mountPath: /etc/tls
          readOnly: true
        - name: data
          mountPath: /var/lib/callfs
        livenessProbe:
          httpGet:
            path: /health
            port: 8443
            scheme: HTTPS
          initialDelaySeconds: 30
          periodSeconds: 10
        readinessProbe:
          httpGet:
            path: /health
            port: 8443
            scheme: HTTPS
          initialDelaySeconds: 5
          periodSeconds: 5
        resources:
          requests:
            memory: "1Gi"
            cpu: "500m"
          limits:
            memory: "2Gi"
            cpu: "1000m"
      volumes:
      - name: config
        configMap:
          name: callfs-config
      - name: tls
        secret:
          secretName: callfs-tls
      - name: data
        persistentVolumeClaim:
          claimName: callfs-data
---
# service.yaml
apiVersion: v1
kind: Service
metadata:
  name: callfs-service
  namespace: callfs
  labels:
    app: callfs
spec:
  selector:
    app: callfs
  ports:
  - name: https
    port: 8443
    targetPort: 8443
  type: ClusterIP
---
# ingress.yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: callfs-ingress
  namespace: callfs
  annotations:
    kubernetes.io/ingress.class: nginx
    nginx.ingress.kubernetes.io/ssl-redirect: "true"
    nginx.ingress.kubernetes.io/backend-protocol: "HTTPS"
    nginx.ingress.kubernetes.io/proxy-body-size: "10g"
spec:
  tls:
  - hosts:
    - callfs.yourdomain.com
    secretName: callfs-tls
  rules:
  - host: callfs.yourdomain.com
    http:
      paths:
      - path: /
        pathType: Prefix
        backend:
          service:
            name: callfs-service
            port:
              number: 8443
```

#### StatefulSet for Persistent Storage
```yaml
# statefulset.yaml
apiVersion: apps/v1
kind: StatefulSet
metadata:
  name: callfs
  namespace: callfs
spec:
  serviceName: callfs-headless
  replicas: 3
  selector:
    matchLabels:
      app: callfs
  template:
    metadata:
      labels:
        app: callfs
    spec:
      containers:
      - name: callfs
        image: callfs:latest
        env:
        - name: CALLFS_INSTANCE_DISCOVERY_INSTANCE_ID
          valueFrom:
            fieldRef:
              fieldPath: metadata.name
        volumeMounts:
        - name: data
          mountPath: /var/lib/callfs
  volumeClaimTemplates:
  - metadata:
      name: data
    spec:
      accessModes: ["ReadWriteOnce"]
      storageClassName: "fast-ssd"
      resources:
        requests:
          storage: 100Gi
```

## Geographic Distribution

### Multi-Region Architecture

#### Region-Specific Configuration
```yaml
# US West Region
instance_discovery:
  instance_id: "callfs-us-west-1"
  peer_endpoints:
    callfs-us-west-2: "https://callfs-us-west-2.internal:8443"
    callfs-us-east-1: "https://callfs-us-east-1.internal:8443"
    callfs-eu-west-1: "https://callfs-eu-west-1.internal:8443"

backend:
  s3_region: "us-west-1"
  s3_bucket_name: "callfs-us-west"

metadata_store:
  dsn: "postgres://callfs:password@postgres-us-west.internal:5432/callfs?sslmode=require"
```

#### Cross-Region Replication

**PostgreSQL Cross-Region Replication:**
```bash
# Primary region setup
pg_basebackup -h postgres-us-west -D /var/lib/postgresql/replica -U replicator -P -W

# Replica configuration
cat >> /var/lib/postgresql/replica/recovery.conf << EOF
standby_mode = 'on'
primary_conninfo = 'host=postgres-us-west port=5432 user=replicator'
restore_command = 'cp /var/lib/postgresql/wal_archive/%f %p'
EOF
```

**S3 Cross-Region Replication:**
```json
{
  "Role": "arn:aws:iam::123456789:role/CallFS-Replication",
  "Rules": [
    {
      "ID": "ReplicateToEurope",
      "Status": "Enabled",
      "Prefix": "",
      "Destination": {
        "Bucket": "arn:aws:s3:::callfs-eu-west",
        "StorageClass": "STANDARD_IA"
      }
    }
  ]
}
```

### DNS and Traffic Routing

#### Route 53 Configuration
```yaml
# terraform/route53.tf
resource "aws_route53_record" "callfs_us_west" {
  zone_id = aws_route53_zone.main.zone_id
  name    = "us-west.callfs.yourdomain.com"
  type    = "A"
  ttl     = 300
  
  set_identifier = "us-west"
  weighted_routing_policy {
    weight = 100
  }
  
  records = [aws_instance.callfs_us_west.public_ip]
}

resource "aws_route53_record" "callfs_eu_west" {
  zone_id = aws_route53_zone.main.zone_id
  name    = "eu-west.callfs.yourdomain.com"
  type    = "A"
  ttl     = 300
  
  set_identifier = "eu-west"
  weighted_routing_policy {
    weight = 100
  }
  
  records = [aws_instance.callfs_eu_west.public_ip]
}

# Health check
resource "aws_route53_health_check" "callfs_us_west" {
  fqdn                            = "us-west.callfs.yourdomain.com"
  port                            = 8443
  type                            = "HTTPS_STR_MATCH"
  resource_path                   = "/health"
  failure_threshold               = "3"
  request_interval                = "30"
  search_string                   = "ok"
}
```

#### GeoDNS Routing
```yaml
resource "aws_route53_record" "callfs_geo_us" {
  zone_id = aws_route53_zone.main.zone_id
  name    = "callfs.yourdomain.com"
  type    = "CNAME"
  ttl     = 60
  
  set_identifier = "US"
  geolocation_routing_policy {
    country = "US"
  }
  
  records = ["us-west.callfs.yourdomain.com"]
  health_check_id = aws_route53_health_check.callfs_us_west.id
}

resource "aws_route53_record" "callfs_geo_eu" {
  zone_id = aws_route53_zone.main.zone_id
  name    = "callfs.yourdomain.com"
  type    = "CNAME"
  ttl     = 60
  
  set_identifier = "EU"
  geolocation_routing_policy {
    continent = "EU"
  }
  
  records = ["eu-west.callfs.yourdomain.com"]
  health_check_id = aws_route53_health_check.callfs_eu_west.id
}
```

## Scaling Strategies

### Horizontal Scaling

#### Auto-scaling with Kubernetes HPA
```yaml
# hpa.yaml
apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
metadata:
  name: callfs-hpa
  namespace: callfs
spec:
  scaleTargetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: callfs
  minReplicas: 3
  maxReplicas: 20
  metrics:
  - type: Resource
    resource:
      name: cpu
      target:
        type: Utilization
        averageUtilization: 70
  - type: Resource
    resource:
      name: memory
      target:
        type: Utilization
        averageUtilization: 80
  - type: Pods
    pods:
      metric:
        name: http_requests_per_second
      target:
        type: AverageValue
        averageValue: "1000"
```

#### Cluster Auto-scaling
```yaml
# cluster-autoscaler.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: cluster-autoscaler
  namespace: kube-system
spec:
  template:
    spec:
      containers:
      - image: k8s.gcr.io/autoscaling/cluster-autoscaler:v1.21.0
        name: cluster-autoscaler
        command:
        - ./cluster-autoscaler
        - --v=4
        - --stderrthreshold=info
        - --cloud-provider=aws
        - --skip-nodes-with-local-storage=false
        - --expander=least-waste
        - --node-group-auto-discovery=asg:tag=k8s.io/cluster-autoscaler/enabled,k8s.io/cluster-autoscaler/callfs-cluster
        - --balance-similar-node-groups
        - --skip-nodes-with-system-pods=false
```

### Vertical Scaling

#### Resource Requests and Limits
```yaml
resources:
  requests:
    memory: "2Gi"
    cpu: "1000m"
  limits:
    memory: "8Gi"
    cpu: "4000m"
```

#### Vertical Pod Autoscaler
```yaml
# vpa.yaml
apiVersion: autoscaling.k8s.io/v1
kind: VerticalPodAutoscaler
metadata:
  name: callfs-vpa
  namespace: callfs
spec:
  targetRef:
    apiVersion: "apps/v1"
    kind: Deployment
    name: callfs
  updatePolicy:
    updateMode: "Auto"
  resourcePolicy:
    containerPolicies:
    - containerName: callfs
      maxAllowed:
        cpu: 4
        memory: 8Gi
      minAllowed:
        cpu: 100m
        memory: 128Mi
```

## Best Practices

### Deployment Strategy
1. **Blue-Green Deployments**: Zero-downtime updates
2. **Canary Releases**: Gradual feature rollouts
3. **Circuit Breakers**: Automatic failure isolation
4. **Health Checks**: Comprehensive service monitoring

### Data Consistency
1. **Eventual Consistency**: Accept temporary inconsistency for availability
2. **Conflict Resolution**: Handle concurrent updates appropriately
3. **Data Validation**: Ensure data integrity across regions
4. **Backup Strategy**: Regular, consistent backups

### Security
1. **Network Segmentation**: Isolate cluster components
2. **Encryption**: Encrypt data in transit and at rest
3. **Access Control**: Role-based access control
4. **Audit Logging**: Comprehensive audit trails

### Monitoring
1. **Multi-layer Monitoring**: Infrastructure, application, and business metrics
2. **Distributed Tracing**: Track requests across instances
3. **Centralized Logging**: Aggregate logs from all instances
4. **Alerting**: Proactive issue detection and notification
