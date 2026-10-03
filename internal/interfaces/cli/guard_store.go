package cli

import (
	"context"
	"time"

	"github.com/NoraStory/GithubHot/internal/infrastructure/persistence/sqlite"
	"github.com/NoraStory/GithubHot/internal/interfaces/httpapi"
)

// guardStore 适配 sqlite 存储到 httpapi.GuardStore 端口。
type guardStore struct{ db *sqlite.DB }

func (g guardStore) AddIPEvent(ctx context.Context, ip, kind, detail string, score int) error {
	return g.db.AddIPEvent(ctx, ip, kind, detail, score)
}

func (g guardStore) RecentIPEventsScore(ctx context.Context, ip string, seconds int) (int, error) {
	return g.db.RecentIPEventsScore(ctx, ip, seconds)
}

func (g guardStore) ListIPEvents(ctx context.Context, limit int) ([]httpapi.IPEventDTO, error) {
	rows, err := g.db.ListIPEvents(ctx, limit)
	if err != nil {
		return nil, err
	}
	out := make([]httpapi.IPEventDTO, 0, len(rows))
	for _, e := range rows {
		out = append(out, httpapi.IPEventDTO{ID: e.ID, IP: e.IP, Kind: e.Kind, Detail: e.Detail, Score: e.Score, At: e.At})
	}
	return out, nil
}

func (g guardStore) ListIPEventsByIP(ctx context.Context, ip string, limit int) ([]httpapi.IPEventDTO, error) {
	rows, err := g.db.ListIPEventsByIP(ctx, ip, limit)
	if err != nil {
		return nil, err
	}
	out := make([]httpapi.IPEventDTO, 0, len(rows))
	for _, e := range rows {
		out = append(out, httpapi.IPEventDTO{ID: e.ID, IP: e.IP, Kind: e.Kind, Detail: e.Detail, Score: e.Score, At: e.At})
	}
	return out, nil
}

func (g guardStore) ListIPEventsSince(ctx context.Context, limit int, since time.Time) ([]httpapi.IPEventDTO, error) {
	rows, err := g.db.ListIPEventsSince(ctx, limit, since)
	if err != nil {
		return nil, err
	}
	out := make([]httpapi.IPEventDTO, 0, len(rows))
	for _, e := range rows {
		out = append(out, httpapi.IPEventDTO{ID: e.ID, IP: e.IP, Kind: e.Kind, Detail: e.Detail, Score: e.Score, At: e.At})
	}
	return out, nil
}

func (g guardStore) UpsertFingerprint(ctx context.Context, fp, ip, ua string, meta httpapi.FingerprintMeta) ([]string, error) {
	return g.db.UpsertFingerprint(ctx, fp, ip, ua, meta.Webrtc, meta.Components, meta.Flags)
}

func (g guardStore) ListFingerprintsByIP(ctx context.Context, ip string, limit int) ([]httpapi.FingerprintDTO, error) {
	rows, err := g.db.ListFingerprintsByIP(ctx, ip)
	if err != nil {
		return nil, err
	}
	if limit > 0 && len(rows) > limit {
		rows = rows[:limit]
	}
	out := make([]httpapi.FingerprintDTO, 0, len(rows))
	for _, f := range rows {
		out = append(out, httpapi.FingerprintDTO{
			Fingerprint: f.Fingerprint, IPs: f.IPs, Webrtc: f.Webrtc, Components: f.Components, Flags: f.Flags, UA: f.UA,
			FirstSeen: f.FirstSeen, LastSeen: f.LastSeen, Hits: f.Hits,
		})
	}
	return out, nil
}

func (g guardStore) ListFingerprints(ctx context.Context, limit int) ([]httpapi.FingerprintDTO, error) {
	rows, err := g.db.ListFingerprints(ctx, limit)
	if err != nil {
		return nil, err
	}
	out := make([]httpapi.FingerprintDTO, 0, len(rows))
	for _, f := range rows {
		out = append(out, httpapi.FingerprintDTO{
			Fingerprint: f.Fingerprint, IPs: f.IPs, Webrtc: f.Webrtc, Components: f.Components, Flags: f.Flags, UA: f.UA,
			FirstSeen: f.FirstSeen, LastSeen: f.LastSeen, Hits: f.Hits,
		})
	}
	return out, nil
}

func (g guardStore) FindBan(ctx context.Context, ip string) (*httpapi.BanDTO, error) {
	b, err := g.db.FindBan(ctx, ip)
	if b == nil || err != nil {
		return nil, err
	}
	return &httpapi.BanDTO{IP: b.IP, Strikes: b.Strikes, Level: b.Level, Reason: b.Reason, BannedAt: b.BannedAt, ExpiresAt: b.ExpiresAt}, nil
}

func (g guardStore) BannedAmong(ctx context.Context, ips []string) ([]string, error) {
	return g.db.BannedAmong(ctx, ips)
}

func (g guardStore) UpsertBan(ctx context.Context, ip string, strikes, level int, reason string, duration time.Duration) error {
	return g.db.UpsertBan(ctx, ip, strikes, level, reason, duration)
}

func (g guardStore) ListBans(ctx context.Context) ([]httpapi.BanDTO, error) {
	rows, err := g.db.ListBans(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]httpapi.BanDTO, 0, len(rows))
	for _, b := range rows {
		out = append(out, httpapi.BanDTO{IP: b.IP, Strikes: b.Strikes, Level: b.Level, Reason: b.Reason, BannedAt: b.BannedAt, ExpiresAt: b.ExpiresAt})
	}
	return out, nil
}

func (g guardStore) DeleteBan(ctx context.Context, ip string) error {
	return g.db.DeleteBan(ctx, ip)
}

func (g guardStore) TouchIPProfile(ctx context.Context, ip, ua string, reqs int) error {
	return g.db.TouchIPProfile(ctx, ip, ua, reqs)
}

func (g guardStore) FindIPProfile(ctx context.Context, ip string) (*httpapi.IPProfileDTO, error) {
	p, err := g.db.FindIPProfile(ctx, ip)
	if p == nil || err != nil {
		return nil, err
	}
	return &httpapi.IPProfileDTO{
		IP: p.IP, FirstSeen: p.FirstSeen, LastSeen: p.LastSeen,
		Reqs: p.Reqs, UASet: p.UASet, UALast: p.UALast,
	}, nil
}
