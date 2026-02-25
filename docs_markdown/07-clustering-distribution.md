# Clustering & Distribution

CallFS is designed for horizontal scalability and high availability. By deploying multiple CallFS instances in a cluster, you can distribute load, increase fault tolerance, and serve users from geographically diverse locations.

## Architecture Overview

A CallFS cluster consists of:
- **Multiple CallFS Instances**: application servers that handle API requests.
- **Metadata Coordination Layer** (choose one):
  - **Shared metadata store mode** (`postgres`, `sqlite`, `redis`): all nodes must point to the same metadata authority.
  - **Raft consensus mode** (`metadata_store.type=raft`): each node has local Raft state; metadata is synchronized via replicated log and snapshots.
- **A Distributed Lock Manager**: Redis (or local for non-distributed setups).
- **An Internal Proxy Network**: peer-to-peer HTTP(S) routing between nodes.
- **A Load Balancer**: distributes incoming traffic across CallFS instances.

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

**Key Configuration Parameters (shared metadata mode):**
- `instance_discovery.instance_id`: A unique name for each node.
- `instance_discovery.peer_endpoints`: A map of all other nodes in the cluster, mapping their `instance_id` to their internal network address.
- `auth.internal_proxy_secret`: A strong, shared secret used for authenticating communication between nodes.
- `metadata_store.*` and `dlm.*`: must point to shared metadata/lock infrastructure.

**Key Configuration Parameters (Raft metadata mode):**
- `metadata_store.type: raft`
- `raft.node_id`, `raft.bind_addr`, `raft.data_dir`
- `raft.peers`: node ID -> Raft transport address
- `raft.api_peer_endpoints`: node ID -> HTTP(S) API endpoint used for follower-to-leader forwarding
- `raft.bootstrap`: enable on exactly one node for first cluster bootstrap

### Easy Node Join (Raft)

After starting a new node, you can add it to the existing cluster with one command:

```bash
callfs cluster join \
  --config /etc/callfs/config.yaml \
  --leader http://callfs-node-1.internal:8443
```

The command reads `raft.node_id`, `raft.bind_addr`, `server.external_url`, and `auth.internal_proxy_secret` from the config file (or flags if provided) and calls the leader join endpoint.

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

- **Write Anywhere**: Clients can write to any node. In Raft mode, followers forward metadata mutations to the leader for consensus commit.
- **Ownership-based Data Routing**: File bytes are written on the file owner node/backend. Other nodes proxy reads/writes to that owner.
- **Automatic Routing**: Requests targeting data owned by another node are transparently proxied to that node.

## High Availability

- **Stateless Instances**: Since the CallFS instances are stateless, you can add or remove them from the cluster without downtime. If a node fails, the load balancer will simply redirect traffic to the healthy nodes.
- **Database and Redis**: For true high availability, your PostgreSQL and Redis instances must also be deployed in a fault-tolerant, clustered configuration (e.g., using Patroni for PostgreSQL and Redis Sentinel or Cluster).

## Geographic Distribution

You can deploy CallFS clusters in multiple geographic regions to reduce latency for users around the world.

- **Architecture**: Each region can run its own CallFS cluster.
- **Important**: independent clusters do not automatically merge file bytes. Metadata synchronization is available in Raft mode within one Raft cluster; cross-region byte replication still requires storage-level strategy.
- **Traffic Routing**: Use a GeoDNS service (like AWS Route 53) to direct users to the nearest regional cluster.

## Scaling

- **Horizontal Scaling**: Add more CallFS nodes to the cluster to handle increased traffic. This can be automated using tools like Kubernetes' Horizontal Pod Autoscaler (HPA).
- **Vertical Scaling**: Increase the CPU and memory resources allocated to each CallFS instance.
