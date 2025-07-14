# Backend Configuration

This document provides comprehensive guidance for configuring CallFS storage backends, including local filesystem, Amazon S3, and distributed peer networks.

## Overview

CallFS supports multiple storage backends that can be configured independently:

- **LocalFS Backend**: Direct local filesystem access
- **S3 Backend**: Amazon S3 or S3-compatible storage
- **Internal Proxy Backend**: Distributed peer-to-peer file sharing
- **NoOp Backend**: Placeholder for disabled backends

Each backend can be enabled or disabled based on configuration. CallFS automatically selects the appropriate backend based on file location and availability.

## Local Filesystem Backend

### Configuration

The LocalFS backend provides direct access to the local filesystem.

#### Basic Configuration
```yaml
backend:
  localfs_root_path: "/var/lib/callfs"
```

#### Environment Variable
```bash
export CALLFS_BACKEND_LOCALFS_ROOT_PATH="/var/lib/callfs"
```

### Setup Requirements

1. **Create Root Directory**
   ```bash
   sudo mkdir -p /var/lib/callfs
   sudo chown callfs:callfs /var/lib/callfs
   sudo chmod 755 /var/lib/callfs
   ```

2. **Set Permissions**
   ```bash
   # Ensure CallFS user can read/write
   sudo chown -R callfs:callfs /var/lib/callfs
   sudo find /var/lib/callfs -type d -exec chmod 755 {} \;
   sudo find /var/lib/callfs -type f -exec chmod 644 {} \;
   ```

3. **Mount Options (Optional)**
   ```bash
   # Mount with performance optimizations
   mount -o noatime,nodiratime /dev/disk /var/lib/callfs
   ```

### Performance Tuning

#### Filesystem Selection
- **ext4**: Good general performance, mature
- **xfs**: Better for large files, concurrent access
- **zfs**: Advanced features, compression, snapshots
- **btrfs**: Copy-on-write, snapshots, compression

#### Mount Options
```bash
# Performance optimizations
mount -o noatime,nodiratime,data=writeback /dev/disk /var/lib/callfs

# For SSD storage
mount -o noatime,discard /dev/disk /var/lib/callfs
```

#### I/O Scheduling
```bash
# Set I/O scheduler for better performance
echo mq-deadline > /sys/block/sda/queue/scheduler
```

### Security Considerations

1. **Directory Permissions**
   ```bash
   # Secure the root directory
   chmod 750 /var/lib/callfs
   chown callfs:callfs /var/lib/callfs
   ```

2. **SELinux/AppArmor**
   ```bash
   # SELinux context
   semanage fcontext -a -t httpd_exec_t "/var/lib/callfs(/.*)?"
   restorecon -R /var/lib/callfs
   ```

3. **File System Encryption**
   ```bash
   # LUKS encryption setup
   cryptsetup luksFormat /dev/disk
   cryptsetup luksOpen /dev/disk callfs-storage
   mkfs.ext4 /dev/mapper/callfs-storage
   mount /dev/mapper/callfs-storage /var/lib/callfs
   ```

### Backup Strategies

1. **Filesystem Snapshots**
   ```bash
   # LVM snapshots
   lvcreate -L 10G -s -n callfs-snap /dev/vg/callfs-volume
   
   # ZFS snapshots
   zfs snapshot pool/callfs@backup-$(date +%Y%m%d)
   ```

2. **Rsync Backup**
   ```bash
   # Incremental backup
   rsync -av --delete /var/lib/callfs/ backup-server:/backups/callfs/
   ```

3. **Database Backup Coordination**
   ```bash
   # Coordinated backup script
   pg_dump callfs > /backup/metadata-$(date +%Y%m%d).sql
   rsync -av /var/lib/callfs/ /backup/files/
   ```

## Amazon S3 Backend

### Configuration

The S3 backend provides scalable object storage with support for AWS S3 and S3-compatible services.

#### Basic Configuration
```yaml
backend:
  s3_access_key: "AKIAIOSFODNN7EXAMPLE"
  s3_secret_key: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY"
  s3_region: "us-west-2"
  s3_bucket_name: "callfs-storage"
  s3_endpoint: ""  # Leave empty for AWS S3
```

#### Advanced Configuration
```yaml
backend:
  s3_access_key: "access_key"
  s3_secret_key: "secret_key"
  s3_region: "us-west-2"
  s3_bucket_name: "callfs-storage"
  s3_endpoint: "https://s3.amazonaws.com"  # Custom endpoint
  s3_server_side_encryption: "aws:kms"     # AES256 or aws:kms
  s3_acl: "private"                        # Object ACL
  s3_kms_key_id: "arn:aws:kms:us-west-2:123456789:key/12345678-1234-1234-1234-123456789012"
```

#### Environment Variables
```bash
export CALLFS_BACKEND_S3_ACCESS_KEY="AKIAIOSFODNN7EXAMPLE"
export CALLFS_BACKEND_S3_SECRET_KEY="wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY"
export CALLFS_BACKEND_S3_REGION="us-west-2"
export CALLFS_BACKEND_S3_BUCKET_NAME="callfs-storage"
export CALLFS_BACKEND_S3_SERVER_SIDE_ENCRYPTION="aws:kms"
export CALLFS_BACKEND_S3_ACL="private"
```

### AWS S3 Setup

1. **Create S3 Bucket**
   ```bash
   aws s3 mb s3://callfs-storage --region us-west-2
   ```

2. **Configure Bucket Policy**
   ```json
   {
     "Version": "2012-10-17",
     "Statement": [
       {
         "Sid": "CallFSAccess",
         "Effect": "Allow",
         "Principal": {
           "AWS": "arn:aws:iam::123456789:user/callfs"
         },
         "Action": [
           "s3:GetObject",
           "s3:PutObject",
           "s3:DeleteObject",
           "s3:ListBucket"
         ],
         "Resource": [
           "arn:aws:s3:::callfs-storage",
           "arn:aws:s3:::callfs-storage/*"
         ]
       }
     ]
   }
   ```

3. **Create IAM User**
   ```bash
   # Create user
   aws iam create-user --user-name callfs
   
   # Attach policy
   aws iam attach-user-policy --user-name callfs \
     --policy-arn arn:aws:iam::123456789:policy/CallFSS3Access
   
   # Create access keys
   aws iam create-access-key --user-name callfs
   ```

4. **Configure Encryption**
   ```bash
   # Enable default encryption
   aws s3api put-bucket-encryption \
     --bucket callfs-storage \
     --server-side-encryption-configuration '{
       "Rules": [{
         "ApplyServerSideEncryptionByDefault": {
           "SSEAlgorithm": "aws:kms",
           "KMSMasterKeyID": "arn:aws:kms:us-west-2:123456789:key/12345678-1234-1234-1234-123456789012"
         }
       }]
     }'
   ```

### S3-Compatible Services

#### MinIO Configuration
```yaml
backend:
  s3_access_key: "minioadmin"
  s3_secret_key: "minioadmin"
  s3_region: "us-east-1"
  s3_bucket_name: "callfs"
  s3_endpoint: "https://minio.yourdomain.com"
```

#### DigitalOcean Spaces
```yaml
backend:
  s3_access_key: "DO_ACCESS_KEY"
  s3_secret_key: "DO_SECRET_KEY"
  s3_region: "nyc3"
  s3_bucket_name: "callfs-space"
  s3_endpoint: "https://nyc3.digitaloceanspaces.com"
```

#### Google Cloud Storage
```yaml
backend:
  s3_access_key: "GOOG_ACCESS_KEY"
  s3_secret_key: "GOOG_SECRET_KEY"
  s3_region: "auto"
  s3_bucket_name: "callfs-bucket"
  s3_endpoint: "https://storage.googleapis.com"
```

### S3 Performance Optimization

1. **Parallel Uploads**
   - CallFS automatically handles multipart uploads for large files
   - Configurable part size and concurrency

2. **Request Rate Optimization**
   ```bash
   # Use request rate prefix patterns
   # Avoid sequential naming patterns like:
   # - timestamp-based prefixes
   # - alphabetical sequences
   
   # Prefer randomized or hash-based prefixes
   ```

3. **Transfer Acceleration**
   ```bash
   # Enable transfer acceleration
   aws s3api put-bucket-accelerate-configuration \
     --bucket callfs-storage \
     --accelerate-configuration Status=Enabled
   ```

### S3 Security

1. **Bucket Access Control**
   ```json
   {
     "Version": "2012-10-17",
     "Statement": [
       {
         "Sid": "DenyInsecureConnections",
         "Effect": "Deny",
         "Principal": "*",
         "Action": "s3:*",
         "Resource": [
           "arn:aws:s3:::callfs-storage",
           "arn:aws:s3:::callfs-storage/*"
         ],
         "Condition": {
           "Bool": {
             "aws:SecureTransport": "false"
           }
         }
       }
     ]
   }
   ```

2. **VPC Endpoint**
   ```bash
   # Create VPC endpoint for S3
   aws ec2 create-vpc-endpoint \
     --vpc-id vpc-12345678 \
     --service-name com.amazonaws.us-west-2.s3 \
     --route-table-ids rtb-12345678
   ```

3. **Access Logging**
   ```bash
   # Enable access logging
   aws s3api put-bucket-logging \
     --bucket callfs-storage \
     --bucket-logging-status file://logging.json
   ```

## Internal Proxy Backend

### Configuration

The Internal Proxy backend enables distributed file sharing between CallFS instances.

#### Basic Configuration
```yaml
auth:
  internal_proxy_secret: "secure-shared-secret"

instance_discovery:
  instance_id: "callfs-instance-1"
  peer_endpoints:
    callfs-instance-2: "https://callfs-02.internal:8443"
    callfs-instance-3: "https://callfs-03.internal:8443"
```

#### Environment Variables
```bash
export CALLFS_AUTH_INTERNAL_PROXY_SECRET="secure-shared-secret"
export CALLFS_INSTANCE_DISCOVERY_INSTANCE_ID="callfs-instance-1"
export CALLFS_INSTANCE_DISCOVERY_PEER_ENDPOINTS='{"callfs-instance-2":"https://callfs-02.internal:8443","callfs-instance-3":"https://callfs-03.internal:8443"}'
```

### Cluster Setup

1. **Network Configuration**
   ```bash
   # Ensure instances can communicate
   telnet callfs-02.internal 8443
   telnet callfs-03.internal 8443
   ```

2. **Shared Secret Distribution**
   ```bash
   # Generate shared secret
   openssl rand -hex 32
   
   # Deploy to all instances
   ansible-playbook -i inventory deploy-secret.yml
   ```

3. **Certificate Management**
   ```bash
   # Ensure all instances have valid certificates
   # Or use internal CA for mutual TLS
   ```

### Load Balancing

1. **HAProxy Configuration**
   ```haproxy
   global
       daemon
   
   defaults
       mode http
       timeout connect 5000ms
       timeout client 50000ms
       timeout server 50000ms
   
   frontend callfs_frontend
       bind *:8443 ssl crt /etc/ssl/certs/callfs.pem
       default_backend callfs_backend
   
   backend callfs_backend
       balance roundrobin
       option httpchk GET /health
       server callfs1 callfs-01.internal:8443 check ssl verify none
       server callfs2 callfs-02.internal:8443 check ssl verify none
       server callfs3 callfs-03.internal:8443 check ssl verify none
   ```

2. **NGINX Configuration**
   ```nginx
   upstream callfs_cluster {
       least_conn;
       server callfs-01.internal:8443 max_fails=3 fail_timeout=30s;
       server callfs-02.internal:8443 max_fails=3 fail_timeout=30s;
       server callfs-03.internal:8443 max_fails=3 fail_timeout=30s;
   }
   
   server {
       listen 443 ssl http2;
       server_name callfs.yourdomain.com;
       
       ssl_certificate /etc/ssl/certs/callfs.crt;
       ssl_certificate_key /etc/ssl/private/callfs.key;
       
       location / {
           proxy_pass https://callfs_cluster;
           proxy_ssl_verify off;
           proxy_set_header Host $host;
           proxy_set_header X-Real-IP $remote_addr;
           proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
           proxy_set_header X-Forwarded-Proto $scheme;
       }
   }
   ```

### High Availability

1. **Database Clustering**
   ```yaml
   # PostgreSQL cluster configuration
   metadata_store:
     dsn: "postgres://callfs:password@postgres-cluster:5432/callfs?sslmode=require"
   ```

2. **Redis Clustering**
   ```yaml
   # Redis cluster configuration
   dlm:
     redis_addr: "redis-cluster-node1:7000,redis-cluster-node2:7000,redis-cluster-node3:7000"
     redis_password: "cluster_password"
   ```

3. **Storage Replication**
   ```bash
   # DRBD for local storage replication
   # Or use shared storage like NFS/GlusterFS
   ```

## Backend Selection Logic

### File Location Strategy

CallFS uses the following logic to determine which backend to use:

1. **Metadata Check**: Check if file exists in metadata store
2. **Instance Location**: Determine which instance has the file
3. **Backend Selection**: Choose appropriate backend based on:
   - File location metadata
   - Backend availability
   - Performance characteristics

### Backend Priority

1. **LocalFS**: Highest priority for files on local instance
2. **Internal Proxy**: For files on peer instances
3. **S3**: For files stored in object storage
4. **NoOp**: Fallback for disabled backends

### Configuration Examples

#### Local-Only Deployment
```yaml
backend:
  localfs_root_path: "/var/lib/callfs"
  # S3 backend disabled (no bucket configured)
  s3_bucket_name: ""

instance_discovery:
  instance_id: "callfs-single"
  # No peer endpoints
  peer_endpoints: {}
```

#### S3-Only Deployment
```yaml
backend:
  # LocalFS backend disabled
  localfs_root_path: ""
  s3_bucket_name: "callfs-storage"
  s3_region: "us-west-2"
  s3_access_key: "ACCESS_KEY"
  s3_secret_key: "SECRET_KEY"

instance_discovery:
  instance_id: "callfs-s3-only"
  peer_endpoints: {}
```

#### Hybrid Deployment
```yaml
backend:
  localfs_root_path: "/var/lib/callfs"
  s3_bucket_name: "callfs-storage"
  s3_region: "us-west-2"
  s3_access_key: "ACCESS_KEY"
  s3_secret_key: "SECRET_KEY"

instance_discovery:
  instance_id: "callfs-hybrid-1"
  peer_endpoints:
    callfs-hybrid-2: "https://callfs-02.internal:8443"
```

## Monitoring and Troubleshooting

### Backend Metrics

Monitor backend performance through Prometheus metrics:

```prometheus
# Backend operation counts
callfs_backend_ops_total{backend_type="localfs",operation="read"}
callfs_backend_ops_total{backend_type="s3",operation="write"}

# Backend operation duration
callfs_backend_op_duration_seconds{backend_type="localfs",operation="read"}
callfs_backend_op_duration_seconds{backend_type="s3",operation="write"}
```

### Health Checks

1. **LocalFS Health**
   ```bash
   # Check disk space
   df -h /var/lib/callfs
   
   # Check I/O performance
   iostat -x 1
   
   # Check file system errors
   dmesg | grep -i error
   ```

2. **S3 Health**
   ```bash
   # Test S3 connectivity
   aws s3 ls s3://callfs-storage --region us-west-2
   
   # Check S3 performance
   aws s3 cp test-file s3://callfs-storage/test/ --region us-west-2
   ```

3. **Network Health**
   ```bash
   # Test peer connectivity
   curl -k https://callfs-02.internal:8443/health
   
   # Check network latency
   ping callfs-02.internal
   ```

### Common Issues

#### LocalFS Issues
```bash
# Permission denied errors
sudo chown -R callfs:callfs /var/lib/callfs
sudo chmod -R 755 /var/lib/callfs

# Disk full errors
df -h /var/lib/callfs
# Clean up or expand storage

# I/O errors
dmesg | grep -i "i/o error"
# Check hardware/filesystem
```

#### S3 Issues
```bash
# Credential errors
aws sts get-caller-identity

# Network connectivity
telnet s3.amazonaws.com 443

# Permission errors
aws s3 ls s3://callfs-storage --debug
```

#### Clustering Issues
```bash
# Certificate validation
openssl s_client -connect callfs-02.internal:8443

# Secret mismatch
# Check internal_proxy_secret on all instances

# Network partitioning
# Check network connectivity between instances
```

### Backup and Recovery

#### LocalFS Backup
```bash
# Full backup
tar -czf callfs-backup-$(date +%Y%m%d).tar.gz -C /var/lib/callfs .

# Incremental backup with rsync
rsync -av --delete /var/lib/callfs/ backup-server:/backups/callfs/
```

#### S3 Backup
```bash
# Cross-region replication
aws s3api put-bucket-replication \
  --bucket callfs-storage \
  --replication-configuration file://replication.json

# Backup to different bucket
aws s3 sync s3://callfs-storage s3://callfs-backup
```

#### Metadata Backup
```bash
# PostgreSQL backup
pg_dump -h localhost -U callfs callfs > callfs-metadata-$(date +%Y%m%d).sql

# Automated backup script
#!/bin/bash
DATE=$(date +%Y%m%d)
pg_dump callfs > /backup/metadata-$DATE.sql
aws s3 cp /backup/metadata-$DATE.sql s3://callfs-backup/metadata/
```

## Best Practices

### Performance
1. **Use appropriate backend for workload**
   - LocalFS for hot data and low latency
   - S3 for cold storage and scalability
   - Hybrid approach for mixed workloads

2. **Monitor and optimize**
   - Track backend performance metrics
   - Optimize based on access patterns
   - Regular performance testing

3. **Capacity planning**
   - Monitor storage usage trends
   - Plan for growth
   - Implement alerting

### Security
1. **Encrypt data at rest**
   - Enable S3 encryption
   - Use filesystem encryption for LocalFS
   - Protect backup data

2. **Secure network communications**
   - Use TLS for all communications
   - Implement network segmentation
   - Regular security audits

3. **Access control**
   - Implement least privilege
   - Regular access reviews
   - Monitor access patterns

### Reliability
1. **Implement redundancy**
   - Multiple availability zones
   - Cross-region replication
   - Regular disaster recovery testing

2. **Monitor health**
   - Comprehensive monitoring
   - Automated alerting
   - Regular health checks

3. **Backup strategy**
   - Regular automated backups
   - Test restoration procedures
   - Document recovery processes
