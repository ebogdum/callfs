# Clustering & Distribution

CallFS is designed for horizontal scalability and high availability. By deploying multiple CallFS instances in a cluster, you can distribute load, increase fault tolerance, and serve users from geographically diverse locations.

## Architecture Overview

A CallFS cluster consists of:
- **Multiple CallFS Instances**: Stateless application servers that handle API requests.
- **A Shared Metadata Store**: A centralized PostgreSQL database (which should be clustered for production) that stores all file metadata.
- **A Shared Distributed Lock Manager**: A Redis cluster that coordinates concurrent operations across all instances.
- **An Internal Proxy Network**: A peer-to-peer communication layer that allows instances to route operations to each other.
- **A Load Balancer**: Distributes incoming traffic across the CallFS instances.

```
          ┌───────────────┐
          │ Load Balancer │
          └───────────────┘
                  │
      ┌───────────┼───────────┐
      │           │           │
┌─────────┐ ┌─────────┐ ┌─────────┐
│ CallFS-1│ │ CallFS-2│ │ CallFS-3│
└─────────┘ └─────────┘ └─────────┘
      │           │           │
      └───────────┼───────────┘
                  │
      ┌───────────┴───────────┐
      │                       │
┌──────────────┐      ┌──────────────┐
│ PostgreSQL   │      │ Redis        │
│   Cluster    │      │   Cluster    │
└──────────────┘      └──────────────┘
```

## Configuration for Clustering

To configure a cluster, each CallFS node needs to know about its peers.

**Key Configuration Parameters:**
- `instance_discovery.instance_id`: A unique name for each node.
- `instance_discovery.peer_endpoints`: A map of all other nodes in the cluster, mapping their `instance_id` to their internal network address.
- `auth.internal_proxy_secret`: A strong, shared secret used for authenticating communication between nodes.
- `metadata_store.dsn` and `dlm.redis_addr`: Must point to the shared PostgreSQL and Redis clusters.

### Example Configuration

**Node 1 (`callfs-node-1`):**
```yaml
instance_discovery:
  instance_id: "callfs-node-1"
  peer_endpoints:
    "callfs-node-2": "https://callfs-node-2.internal:8443"
    "callfs-node-3": "https://callfs-node-3.internal:8443"
```

**Node 2 (`callfs-node-2`):**
```yaml
instance_discovery:
  instance_id: "callfs-node-2"
  peer_endpoints:
    "callfs-node-1": "https://callfs-node-1.internal:8443"
    "callfs-node-3": "https://callfs-node-3.internal:8443"
```
*(And so on for all other nodes)*

## Cross-Server Operations

The true power of a CallFS cluster lies in its ability to handle operations that span multiple nodes seamlessly.

- **Automatic Routing**: When a request for a file arrives at `Node A`, but the file is stored on the local filesystem of `Node B`, `Node A` will automatically proxy the request to `Node B`. This is completely transparent to the client.
- **Conflict Detection**: When creating a new file or directory (`POST`), CallFS checks the entire cluster to ensure the path does not already exist on any other node. If it does, it returns a `409 Conflict` error, preventing data overwrites.

## High Availability

- **Stateless Instances**: Since the CallFS instances are stateless, you can add or remove them from the cluster without downtime. If a node fails, the load balancer will simply redirect traffic to the healthy nodes.
- **Database and Redis**: For true high availability, your PostgreSQL and Redis instances must also be deployed in a fault-tolerant, clustered configuration (e.g., using Patroni for PostgreSQL and Redis Sentinel or Cluster).

## Geographic Distribution

You can deploy CallFS clusters in multiple geographic regions to reduce latency for users around the world.

- **Architecture**: Each region would have its own cluster of CallFS nodes, a replica of the PostgreSQL database, and a Redis cache.
- **Data Replication**: S3 backends can be configured with cross-region replication. For local filesystems, you would need a separate data replication strategy.
- **Traffic Routing**: Use a GeoDNS service (like AWS Route 53) to direct users to the nearest regional cluster.

## Scaling

- **Horizontal Scaling**: Add more CallFS nodes to the cluster to handle increased traffic. This can be automated using tools like Kubernetes' Horizontal Pod Autoscaler (HPA).
- **Vertical Scaling**: Increase the CPU and memory resources allocated to each CallFS instance.
