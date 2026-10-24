package cluster

import (
	"context"
	"time"
)

const DefaultLeaseTTL = 30 * time.Second

type Lease interface {
	Hold(ctx context.Context, name string, ttl time.Duration) (held <-chan struct{}, release func(), err error)
}
