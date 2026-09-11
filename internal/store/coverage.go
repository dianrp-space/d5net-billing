package store

import (
	"context"
	"math"
	"sort"

	"github.com/dianrp-space/d5net-billing/internal/xid"
)

const maxCoverageRadiusKm = 50.0

// NormalizeCoverageRadiusKm clamps to (0, 50]; empty/≤0 becomes unset.
func NormalizeCoverageRadiusKm(v *float64) *float64 {
	if v == nil {
		return nil
	}
	if math.IsNaN(*v) || math.IsInf(*v, 0) || *v <= 0 {
		return nil
	}
	x := *v
	if x > maxCoverageRadiusKm {
		x = maxCoverageRadiusKm
	}
	x = math.Round(x*1000) / 1000
	if x <= 0 {
		return nil
	}
	return &x
}

// HaversineMeters is great-circle distance between two WGS84 points.
func HaversineMeters(lat1, lng1, lat2, lng2 float64) float64 {
	const R = 6371000.0
	φ1 := lat1 * math.Pi / 180
	φ2 := lat2 * math.Pi / 180
	Δφ := (lat2 - lat1) * math.Pi / 180
	Δλ := (lng2 - lng1) * math.Pi / 180
	a := math.Sin(Δφ/2)*math.Sin(Δφ/2) + math.Cos(φ1)*math.Cos(φ2)*math.Sin(Δλ/2)*math.Sin(Δλ/2)
	return 2 * R * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
}

type CoverageHit struct {
	Kind      string  `json:"kind"`
	ID        xid.ID  `json:"id"`
	Name      string  `json:"name"`
	Code      string  `json:"code"`
	DistanceM float64 `json:"distance_m"`
	RadiusKm  float64 `json:"radius_km"`
	Covered   bool    `json:"covered"`
	FreePorts *int    `json:"free_ports,omitempty"`
}

type CoverageCheckResult struct {
	Covered bool          `json:"covered"`
	Hits    []CoverageHit `json:"hits"`
}

// CheckCoverage lists POP/ODP whose stored radius covers lat/lng (nearest first).
func (s *Store) CheckCoverage(ctx context.Context, tenantID xid.ID, lat, lng float64) (CoverageCheckResult, error) {
	out := CoverageCheckResult{Hits: []CoverageHit{}}
	clusters, err := s.ListClusters(ctx, tenantID)
	if err != nil {
		return out, err
	}
	odps, err := s.ListODPs(ctx, tenantID)
	if err != nil {
		return out, err
	}
	var hits []CoverageHit
	for _, c := range clusters {
		if !c.IsActive || c.Latitude == nil || c.Longitude == nil {
			continue
		}
		km := NormalizeCoverageRadiusKm(c.CoverageRadiusKm)
		if km == nil {
			continue
		}
		dist := HaversineMeters(lat, lng, *c.Latitude, *c.Longitude)
		hits = append(hits, CoverageHit{
			Kind: "pop", ID: c.ID, Name: c.Name, Code: c.Code,
			DistanceM: dist, RadiusKm: *km, Covered: dist <= *km*1000,
		})
	}
	for _, o := range odps {
		if o.Latitude == nil || o.Longitude == nil {
			continue
		}
		km := NormalizeCoverageRadiusKm(o.CoverageRadiusKm)
		if km == nil {
			continue
		}
		dist := HaversineMeters(lat, lng, *o.Latitude, *o.Longitude)
		fp := o.FreePorts
		hits = append(hits, CoverageHit{
			Kind: "odp", ID: o.ID, Name: o.Name, Code: o.Code,
			DistanceM: dist, RadiusKm: *km, Covered: dist <= *km*1000, FreePorts: &fp,
		})
	}
	sort.Slice(hits, func(i, j int) bool {
		if hits[i].Covered != hits[j].Covered {
			return hits[i].Covered
		}
		return hits[i].DistanceM < hits[j].DistanceM
	})
	for _, h := range hits {
		if h.Covered {
			out.Covered = true
			break
		}
	}
	out.Hits = hits
	return out, nil
}

func (s *Store) UpdateClusterCoverage(ctx context.Context, tenantID, id xid.ID, km *float64) error {
	c, err := s.GetCluster(ctx, tenantID, id)
	if err != nil {
		return err
	}
	c.CoverageRadiusKm = NormalizeCoverageRadiusKm(km)
	return s.UpdateCluster(ctx, c)
}

func (s *Store) UpdateODPCoverage(ctx context.Context, tenantID, id xid.ID, km *float64) error {
	o, err := s.GetODP(ctx, tenantID, id)
	if err != nil {
		return err
	}
	o.CoverageRadiusKm = NormalizeCoverageRadiusKm(km)
	return s.UpdateODP(ctx, o)
}
