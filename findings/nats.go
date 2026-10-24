package findings

import (
	"context"

	"github.com/ebogdum/callfs/v2/metadata/natsmeta"
)

type MetadataLog interface {
	LogInfo(ctx context.Context) (natsmeta.LogInfo, error)
}

func CollectNATSFacts(ctx context.Context, log MetadataLog, facts *Facts) {
	info, err := log.LogInfo(ctx)
	if nil != err {
		facts.NATSUnreachable = true
		return
	}
	facts.MetadataReplicas = len(info.Replicas)
	for _, replica := range info.Replicas {
		if replica.Offline {
			facts.MetadataOffline++
		}
	}
	if info.AppliedSeq < info.LastSeq {
		facts.MetadataLogLag = info.LastSeq - info.AppliedSeq
	}
	if nil != info.LastSnapshot {
		facts.LastSnapshotAt = info.LastSnapshot.CreatedAt
	}
}
