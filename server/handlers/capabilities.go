package handlers

import (
	"net/http"
	"runtime"
	"time"

	"github.com/ebogdum/callfs/config"
	"github.com/ebogdum/callfs/core"
	"github.com/ebogdum/callfs/internal/buildinfo"
)

const (
	capabilitiesMaxSplitUploadBytes int64 = 1 << 30
	capabilitiesMaxWSUploadBytes    int64 = 100000000
	capabilitiesWSFrameBytes        int64 = 1 << 20
	capabilitiesMaxPatchBodyBytes   int64 = 1 << 20
	capabilitiesUploadPartBytes     int64 = 8 << 20
	capabilitiesLinkExpiryMinS            = 1
	capabilitiesLinkExpiryMaxS            = 86400
	capabilitiesMaxTotalShards            = 64
)

type CapabilitiesSource struct {
	Config     *config.AppConfig
	InstanceID string
}

type capabilitiesFeatures struct {
	Erasure          bool `json:"erasure"`
	Cluster          bool `json:"cluster"`
	CrossServer      bool `json:"cross_server"`
	Websocket        bool `json:"websocket"`
	Replication      bool `json:"replication"`
	QUIC             bool `json:"quic"`
	MetricsListener  bool `json:"metrics_listener"`
	Trash            bool `json:"trash"`
	ResumableUploads bool `json:"resumable_uploads"`
	Search           bool `json:"search"`
	UI               bool `json:"ui"`
}

type capabilitiesLimits struct {
	MaxUploadBytes      int64 `json:"max_upload_bytes"`
	MaxSplitUploadBytes int64 `json:"max_split_upload_bytes"`
	MaxWSUploadBytes    int64 `json:"max_ws_upload_bytes"`
	WSFrameBytes        int64 `json:"ws_frame_bytes"`
	MaxPatchBodyBytes   int64 `json:"max_patch_body_bytes"`
	LinkExpiryMinS      int   `json:"link_expiry_min_s"`
	LinkExpiryMaxS      int   `json:"link_expiry_max_s"`
	ListPageMax         int   `json:"list_page_max"`
	UploadPartBytes     int64 `json:"upload_part_bytes"`
}

type capabilitiesErasureDefaults struct {
	DataShards     int   `json:"data_shards"`
	ParityShards   int   `json:"parity_shards"`
	MaxTotalShards int   `json:"max_total_shards"`
	MinFileSize    int64 `json:"min_file_size"`
}

type capabilitiesResponse struct {
	Version         string                      `json:"version"`
	Commit          string                      `json:"commit"`
	BuildDate       string                      `json:"build_date"`
	GoVersion       string                      `json:"go_version"`
	InstanceID      string                      `json:"instance_id"`
	NodeID          *string                     `json:"node_id"`
	Features        capabilitiesFeatures        `json:"features"`
	Storages        []string                    `json:"storages"`
	DefaultStorage  string                      `json:"default_storage"`
	FileIndex       string                      `json:"file_index"`
	LockType        string                      `json:"lock_type"`
	ExternalURL     string                      `json:"external_url"`
	Limits          capabilitiesLimits          `json:"limits"`
	ErasureDefaults capabilitiesErasureDefaults `json:"erasure_defaults"`
	TrashRetentionS int64                       `json:"trash_retention_s"`
}

func V1Capabilities(source CapabilitiesSource) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		SendJSONResponse(w, buildCapabilities(source))
	}
}

func buildCapabilities(source CapabilitiesSource) capabilitiesResponse {
	cfg := source.Config
	storages := []string{"localfs"}
	if "" != cfg.Backend.S3BucketName {
		storages = append(storages, "s3")
	}
	var nodeID *string
	if "" != cfg.NATS.ServerName {
		nodeID = &cfg.NATS.ServerName
	}
	return capabilitiesResponse{
		Version:    buildinfo.Version,
		Commit:     buildinfo.Commit,
		BuildDate:  buildinfo.Date,
		GoVersion:  runtime.Version(),
		InstanceID: source.InstanceID,
		NodeID:     nodeID,
		Features: capabilitiesFeatures{
			Erasure:          cfg.Erasure.Enabled,
			Cluster:          cfg.NATS.Enabled,
			CrossServer:      0 < len(cfg.InstanceDiscovery.PeerEndpoints),
			Websocket:        true,
			Replication:      cfg.HA.ReplicationEnabled,
			QUIC:             cfg.Server.EnableQUIC,
			MetricsListener:  "" != cfg.Metrics.ListenAddr,
			Trash:            cfg.Trash.Enabled,
			ResumableUploads: true,
			Search:           true,
			UI:               cfg.UI.Enabled,
		},
		Storages:       storages,
		DefaultStorage: cfg.Backend.DefaultBackend,
		FileIndex:      cfg.MetadataStore.Type,
		LockType:       cfg.DLM.Type,
		ExternalURL:    cfg.Server.ExternalURL,
		Limits: capabilitiesLimits{
			MaxUploadBytes:      core.MaxUploadBytes,
			MaxSplitUploadBytes: capabilitiesMaxSplitUploadBytes,
			MaxWSUploadBytes:    capabilitiesMaxWSUploadBytes,
			WSFrameBytes:        capabilitiesWSFrameBytes,
			MaxPatchBodyBytes:   capabilitiesMaxPatchBodyBytes,
			LinkExpiryMinS:      capabilitiesLinkExpiryMinS,
			LinkExpiryMaxS:      capabilitiesLinkExpiryMaxS,
			ListPageMax:         maxDirectoryPageLimit,
			UploadPartBytes:     capabilitiesUploadPartBytes,
		},
		ErasureDefaults: capabilitiesErasureDefaults{
			DataShards:     cfg.Erasure.DataShards,
			ParityShards:   cfg.Erasure.ParityShards,
			MaxTotalShards: capabilitiesMaxTotalShards,
			MinFileSize:    0,
		},
		TrashRetentionS: int64(cfg.Trash.Retention / time.Second),
	}
}
