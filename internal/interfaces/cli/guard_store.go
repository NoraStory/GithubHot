package cli

import (
	"context"
	"encoding/json"
	"time"

	"github.com/NoraStory/GithubHot/internal/domain/fpcluster"
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
	compStabilityJSON := ""
	if len(meta.CompStability) > 0 {
		b, err := json.Marshal(meta.CompStability)
		if err == nil {
			compStabilityJSON = string(b)
		}
	}
	return g.db.UpsertFingerprint(ctx, fp, ip, ua, meta.Webrtc, meta.Components, meta.Flags,
		meta.CanvasPHash, meta.MinHashSig, meta.JA4, meta.Stability, compStabilityJSON,
		meta.Behavior, meta.ClockSkewPPM, meta.Trusted)
}

func (g guardStore) UpsertLSHBands(ctx context.Context, fp string, bands []httpapi.LSHBand) error {
	rows := make([]sqlite.LSHBandRow, 0, len(bands))
	for _, b := range bands {
		rows = append(rows, sqlite.LSHBandRow{Band: b.Band, Hash: b.Hash})
	}
	return g.db.UpsertLSHBands(ctx, fp, rows)
}

func (g guardStore) ListLSHCandidates(ctx context.Context, fp string, bands []httpapi.LSHBand, limit int) ([]string, error) {
	rows := make([]sqlite.LSHBandRow, 0, len(bands))
	for _, b := range bands {
		rows = append(rows, sqlite.LSHBandRow{Band: b.Band, Hash: b.Hash})
	}
	return g.db.ListLSHCandidates(ctx, fp, rows, limit)
}

func (g guardStore) ListMinHashSigs(ctx context.Context, fps []string) ([]httpapi.MinHashSigRow, error) {
	rows, err := g.db.ListMinHashSigs(ctx, fps)
	if err != nil {
		return nil, err
	}
	out := make([]httpapi.MinHashSigRow, 0, len(rows))
	for _, r := range rows {
		out = append(out, httpapi.MinHashSigRow{FP: r.FP, Sig: r.Sig})
	}
	return out, nil
}

func (g guardStore) FindFingerprint(ctx context.Context, fp string) (*httpapi.FingerprintDTO, error) {
	row, err := g.db.FindFingerprint(ctx, fp)
	if row == nil || err != nil {
		return nil, err
	}
	dto := toFingerprintDTOs([]sqlite.FingerprintRow{*row})[0]
	return &dto, nil
}

func (g guardStore) UpdateEntropyBits(ctx context.Context, fp string, bits float64) error {
	return g.db.UpdateEntropyBits(ctx, fp, bits)
}

func (g guardStore) UpdateIPGeo(ctx context.Context, ip string, asn uint, asnType, country, tz string) error {
	return g.db.UpdateIPGeo(ctx, ip, asn, asnType, country, tz)
}

func (g guardStore) ListPHashCandidates(ctx context.Context, since time.Time, limit int) ([]httpapi.PHashRowDTO, error) {
	rows, err := g.db.ListPHashCandidates(ctx, since, limit)
	if err != nil {
		return nil, err
	}
	out := make([]httpapi.PHashRowDTO, 0, len(rows))
	for _, r := range rows {
		out = append(out, httpapi.PHashRowDTO{Fingerprint: r.Fingerprint, PHash: r.PHash, LastSeen: r.LastSeen})
	}
	return out, nil
}

func (g guardStore) UpsertFPLink(ctx context.Context, src, dst, kind string, weight float64) error {
	return g.db.UpsertFPLink(ctx, src, dst, kind, weight)
}

func (g guardStore) ListFPLinks(ctx context.Context, fp string, limit int) ([]httpapi.FPLinkDTO, error) {
	rows, err := g.db.ListFPLinks(ctx, fp, limit)
	if err != nil {
		return nil, err
	}
	out := make([]httpapi.FPLinkDTO, 0, len(rows))
	for _, r := range rows {
		out = append(out, httpapi.FPLinkDTO{
			Src: r.Src, Dst: r.Dst, Kind: r.Kind, Weight: r.Weight,
			FirstSeen: r.FirstSeen, LastSeen: r.LastSeen,
		})
	}
	return out, nil
}

func (g guardStore) ListFingerprintsByIP(ctx context.Context, ip string, limit int) ([]httpapi.FingerprintDTO, error) {
	rows, err := g.db.ListFingerprintsByIP(ctx, ip)
	if err != nil {
		return nil, err
	}
	if limit > 0 && len(rows) > limit {
		rows = rows[:limit]
	}
	return toFingerprintDTOs(rows), nil
}

func (g guardStore) ListFingerprints(ctx context.Context, limit int) ([]httpapi.FingerprintDTO, error) {
	rows, err := g.db.ListFingerprints(ctx, limit)
	if err != nil {
		return nil, err
	}
	return toFingerprintDTOs(rows), nil
}

func (g guardStore) ListFingerprintsSince(ctx context.Context, since time.Time, limit int) ([]httpapi.FingerprintDTO, error) {
	rows, err := g.db.ListFingerprintsSince(ctx, since, limit)
	if err != nil {
		return nil, err
	}
	return toFingerprintDTOs(rows), nil
}

func toFingerprintDTOs(rows []sqlite.FingerprintRow) []httpapi.FingerprintDTO {
	out := make([]httpapi.FingerprintDTO, 0, len(rows))
	for _, f := range rows {
		out = append(out, httpapi.FingerprintDTO{
			Fingerprint: f.Fingerprint, IPs: f.IPs, Webrtc: f.Webrtc, Components: f.Components, Flags: f.Flags, UA: f.UA,
			FirstSeen: f.FirstSeen, LastSeen: f.LastSeen, Hits: f.Hits,
			CanvasPHash: f.CanvasPHash, MinHashSig: f.MinHashSig,
			EntropyBits: f.EntropyBits, Stability: f.Stability, CompStability: f.CompStability,
			JA4: f.JA4, Attestation: f.AttestationJSON,
			BehaviorJSON: f.BehaviorJSON, ClockSkewPPM: f.ClockSkewPPM, ClusterID: f.ClusterID, GNNScore: f.GNNScore, AnomalyScore: f.AnomalyScore,
		})
	}
	return out
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

func (g guardStore) UpdateAttestation(ctx context.Context, fp string, attestationJSON string) error {
	return g.db.UpdateAttestation(ctx, fp, attestationJSON)
}

func (g guardStore) ReplaceClusters(ctx context.Context, clusters []httpapi.ClusterDTO) error {
	rows := make([]fpcluster.Cluster, 0, len(clusters))
	for _, c := range clusters {
		rows = append(rows, fpcluster.Cluster{Members: c.Members, Size: c.Size, Reason: c.Reason})
	}
	return g.db.ReplaceClusters(ctx, rows)
}

func (g guardStore) ListAllFPLinks(ctx context.Context, since time.Time, limit int) ([]httpapi.FPLinkDTO, error) {
	rows, err := g.db.ListAllFPLinks(ctx, since, limit)
	if err != nil {
		return nil, err
	}
	out := make([]httpapi.FPLinkDTO, 0, len(rows))
	for _, r := range rows {
		out = append(out, httpapi.FPLinkDTO{Src: r.Src, Dst: r.Dst, Kind: r.Kind, Weight: r.Weight, FirstSeen: r.FirstSeen, LastSeen: r.LastSeen})
	}
	return out, nil
}

func (g guardStore) ListClusters(ctx context.Context, limit int) ([]httpapi.ClusterDTO, error) {
	rows, err := g.db.ListClusters(ctx, limit)
	if err != nil {
		return nil, err
	}
	out := make([]httpapi.ClusterDTO, 0, len(rows))
	for _, r := range rows {
		out = append(out, httpapi.ClusterDTO{ID: r.ID, Members: r.Members, Size: r.Size, FirstSeen: r.FirstSeen, Reason: r.Reason})
	}
	return out, nil
}

func (g guardStore) UpsertFpLabel(ctx context.Context, fp, label, source string, confidence float64, notes string) error {
	return g.db.UpsertFpLabel(ctx, fp, label, source, confidence, notes)
}

func (g guardStore) ListFpLabels(ctx context.Context) ([]httpapi.FpLabelRow, error) {
	rows, err := g.db.ListFpLabels(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]httpapi.FpLabelRow, 0, len(rows))
	for _, r := range rows {
		out = append(out, httpapi.FpLabelRow{FP: r.FP, Label: r.Label, Source: r.Source,
			Confidence: r.Confidence, LabeledAt: r.LabeledAt, Notes: r.Notes})
	}
	return out, nil
}

func (g guardStore) BestFpLabels(ctx context.Context) (map[string]struct {
	Label      string
	Confidence float64
}, error) {
	return g.db.BestFpLabels(ctx)
}

func (g guardStore) DeleteFpRuleLabels(ctx context.Context) error {
	return g.db.DeleteFpRuleLabels(ctx)
}

func (g guardStore) UpdateAnomalyScore(ctx context.Context, fp string, score float64) error {
	return g.db.UpdateAnomalyScore(ctx, fp, score)
}

func (g guardStore) UpdateGNN(ctx context.Context, fp string, score float64, embeddingJSON string) error {
	return g.db.UpdateGNN(ctx, fp, score, embeddingJSON)
}

func (g guardStore) AddReviewItem(ctx context.Context, fp string, reasons []string, scores map[string]float64) error {
	return g.db.AddReviewItem(ctx, fp, reasons, scores)
}

func (g guardStore) ListReviewItems(ctx context.Context, status string, limit int) ([]httpapi.ReviewItemRow, error) {
	rows, err := g.db.ListReviewItems(ctx, status, limit)
	if err != nil {
		return nil, err
	}
	out := make([]httpapi.ReviewItemRow, 0, len(rows))
	for _, r := range rows {
		out = append(out, httpapi.ReviewItemRow{ID: r.ID, FP: r.FP, Reasons: r.Reasons,
			Scores: r.Scores, Status: r.Status, CreatedAt: r.CreatedAt,
			ResolvedAt: r.ResolvedAt, ResolvedBy: r.ResolvedBy})
	}
	return out, nil
}

func (g guardStore) ResolveReviewItems(ctx context.Context, ids []int64, action string, resolvedBy string) error {
	return g.db.ResolveReviewItems(ctx, ids, action, resolvedBy)
}

func (g guardStore) CountReviewItems(ctx context.Context, status string) (int, error) {
	return g.db.CountReviewItems(ctx, status)
}
