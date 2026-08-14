package availability

import (
	"context"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

// PresenceTTL is how long a guide stays "online" without a heartbeat
// (spec §10.2 "available now" reads Redis; clients must re-POST availability
// within the TTL to remain discoverable).
const PresenceTTL = 300 * time.Second

func presenceKey(guideID string) string { return "presence:guide:" + guideID }

// Presence owns the ephemeral online flag in Redis (ADR 0003). Online state
// is never written to Postgres.
type Presence struct {
	rdb *goredis.Client
}

// NewPresence builds the presence tracker.
func NewPresence(rdb *goredis.Client) *Presence { return &Presence{rdb: rdb} }

// SetOnline marks the guide online (with TTL) or removes the marker.
func (p *Presence) SetOnline(ctx context.Context, guideID string, online bool) error {
	if online {
		return p.rdb.Set(ctx, presenceKey(guideID), "1", PresenceTTL).Err()
	}
	return p.rdb.Del(ctx, presenceKey(guideID)).Err()
}

// OnlineIDs reports which of the given guides currently hold a live online
// marker. One round trip via MGET.
func (p *Presence) OnlineIDs(ctx context.Context, ids []string) (map[string]bool, error) {
	out := map[string]bool{}
	if len(ids) == 0 {
		return out, nil
	}
	keys := make([]string, 0, len(ids))
	for _, id := range ids {
		keys = append(keys, presenceKey(id))
	}
	vals, err := p.rdb.MGet(ctx, keys...).Result()
	if err != nil {
		return nil, err
	}
	for i, v := range vals {
		if v != nil {
			out[ids[i]] = true
		}
	}
	return out, nil
}
