package cluster

import (
	"encoding/base64"
	"strings"
)

const (
	SubjectRoot    = "callfs"
	MetaLogSubject = "callfs.meta.log"

	TopicCacheInval    = "cache.inval"
	TopicConfigRuntime = "config.runtime"
	TopicPresenceNudge = "presence.nudge"

	TelemetryEvents = "events"
	TelemetryAudit  = "audit"
	TelemetryLogs   = "logs"

	StreamMeta    = "CALLFS_META"
	StreamChanges = "CALLFS_CHANGES"
	StreamEvents  = "CALLFS_EVENTS"
	StreamAudit   = "CALLFS_AUDIT"
	StreamLogs    = "CALLFS_LOGS"
	StreamTasks   = "CALLFS_TASKS"
	StreamDLQ     = "CALLFS_DLQ"

	BucketNodes   = "CALLFS_NODES"
	BucketLocks   = "CALLFS_LOCKS"
	BucketLockQ   = "CALLFS_LOCKQ"
	BucketLeases  = "CALLFS_LEASES"
	BucketJobs    = "CALLFS_JOBS"
	BucketUploads = "CALLFS_UPLOADS"
	BucketConfig  = "CALLFS_CONFIG"
	BucketStats   = "CALLFS_STATS"

	ObjectMetaSnap = "CALLFS_META_SNAP"
)

func ValidToken(s string) bool {
	if "" == s {
		return false
	}
	for i := 0; i < len(s); i++ {
		if !tokenByte(s[i]) {
			return false
		}
	}
	return true
}

func ValidTopic(topic string) bool {
	if "" == topic {
		return false
	}
	for _, part := range strings.Split(topic, ".") {
		if !ValidToken(part) {
			return false
		}
	}
	return true
}

func tokenByte(c byte) bool {
	return ('a' <= c && 'z' >= c) || ('A' <= c && 'Z' >= c) || ('0' <= c && '9' >= c) || '_' == c || '-' == c
}

func Token(s string) string {
	if ValidToken(s) && 'x' != s[0] {
		return s
	}
	return "x" + base64.RawURLEncoding.EncodeToString([]byte(s))
}

func ParseToken(tok string) (string, bool) {
	if !ValidToken(tok) {
		return "", false
	}
	if 'x' != tok[0] {
		return tok, true
	}
	raw, err := base64.RawURLEncoding.DecodeString(tok[1:])
	if nil != err {
		return "", false
	}
	return string(raw), true
}

func NodeRPCSubject(node, service, method string) string {
	return SubjectRoot + ".node." + Token(node) + ".rpc." + service + "." + method
}

func NodeRPCServiceWildcard(node, service string) string {
	return SubjectRoot + ".node." + Token(node) + ".rpc." + service + ".*"
}

func ServiceSubject(service, method string) string {
	return SubjectRoot + ".svc." + service + "." + method
}

func ServiceWildcard(service string) string {
	return SubjectRoot + ".svc." + service + ".*"
}

func ServiceQueueGroup(service string) string {
	return "svc." + service
}

func GatherSubject(service, method string) string {
	return SubjectRoot + ".all." + service + "." + method
}

func GatherWildcard(service string) string {
	return SubjectRoot + ".all." + service + ".*"
}

func BroadcastSubject(topic string) string {
	return SubjectRoot + ".bcast." + topic
}

func ChangesSubject(mount string) string {
	return SubjectRoot + ".changes." + Token(mount)
}

func ChangesWildcard() string {
	return SubjectRoot + ".changes.>"
}

func TelemetrySubject(kind, node, sub string) string {
	return SubjectRoot + ".tel." + kind + "." + Token(node) + "." + Token(sub)
}

func TelemetryWildcard(kind string) string {
	return SubjectRoot + ".tel." + kind + ".>"
}

func TaskAnySubject(pool string) string {
	return SubjectRoot + ".tasks." + pool + ".any"
}

func TaskNodeSubject(pool, node string) string {
	return SubjectRoot + ".tasks." + pool + ".node." + Token(node)
}

func TaskSubject(pool, affinity string) string {
	if "" == affinity {
		return TaskAnySubject(pool)
	}
	return TaskNodeSubject(pool, affinity)
}

func TasksWildcard() string {
	return SubjectRoot + ".tasks.>"
}

func TaskConsumerAny(pool string) string {
	return "tasks-" + pool + "-any"
}

func TaskConsumerNode(pool, node string) string {
	return "tasks-" + pool + "-" + Token(node)
}

func DLQSubject(pool string) string {
	return SubjectRoot + ".dlq." + pool
}

func DLQWildcard() string {
	return SubjectRoot + ".dlq.>"
}

func PresenceKey(node string) string {
	return "n." + Token(node)
}

func LeaseKey(name string) string {
	return "w." + Token(name)
}
