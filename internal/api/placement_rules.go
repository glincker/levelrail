package api

import (
	"context"
	"errors"

	"github.com/GLINCKER/levelrail/internal/placement"
	"github.com/GLINCKER/levelrail/internal/store"
)

// placementRules are one app's tolerations and affinity as the scheduler reads them.
type placementRules struct {
	Tolerations []placement.Toleration
	Affinity    *placement.Affinity
}

func rulesOf(s store.DesiredService) placementRules {
	t, a := placement.DecodeRules(s.Tolerations, s.Affinity)
	return placementRules{Tolerations: t, Affinity: a}
}

// rulesFor loads the stored rules of kind/name. Databases and apps not saved
// yet have none.
func (rt *Router) rulesFor(ctx context.Context, kind, name string) (placementRules, error) {
	if kind != store.NodeMoveKindApp || rt.apps == nil {
		return placementRules{}, nil
	}
	s, err := rt.apps.GetDesiredService(ctx, name)
	if errors.Is(err, store.ErrServiceNotFound) {
		return placementRules{}, nil
	}
	if err != nil {
		return placementRules{}, err
	}
	return rulesOf(*s), nil
}

// request copies the rules onto req.
func (r placementRules) request(req placement.Request) placement.Request {
	req.Tolerations = r.Tolerations
	if r.Affinity != nil {
		req.Affinity = *r.Affinity
	}
	return req
}

func (r placementRules) needsWorkloads() bool {
	return r.Affinity != nil && (len(r.Affinity.Together) > 0 || len(r.Affinity.Apart) > 0)
}

// ruledRequest returns req carrying kind/name's rules and cands with the
// workloads already on each node, built once per decision.
func (rt *Router) ruledRequest(ctx context.Context, kind, name string, cands []placement.Candidate, req placement.Request) ([]placement.Candidate, placement.Request, error) {
	rules, err := rt.rulesFor(ctx, kind, name)
	if err != nil {
		return cands, req, err
	}
	out, err := rt.withWorkloads(ctx, rules, name, cands)
	return out, rules.request(req), err
}

// withWorkloads returns cands with each node's other apps as label sets: the
// operator labels plus name=<app>. The app being placed is left out. cands is
// untouched, so a cached slice can be reused for the next app.
func (rt *Router) withWorkloads(ctx context.Context, rules placementRules, exclude string, cands []placement.Candidate) ([]placement.Candidate, error) {
	if !rules.needsWorkloads() || rt.apps == nil {
		return cands, nil
	}
	services, err := rt.apps.ListDesiredServices(ctx)
	if err != nil {
		return cands, err
	}
	var labels map[string]map[string]string
	if ls := rt.resourceLabels(); ls != nil {
		if labels, err = ls.ListResourceLabels(ctx, store.LabelKindApp); err != nil {
			return cands, err
		}
	}
	byNode := make(map[string][]map[string]string, len(cands))
	for _, s := range services {
		if s.Name == exclude {
			continue
		}
		node := s.NodeID
		if node == "" {
			node = rt.localNodeID
		}
		l := make(map[string]string, len(labels[s.Name])+1)
		for k, v := range labels[s.Name] {
			l[k] = v
		}
		l["name"] = s.Name
		byNode[node] = append(byNode[node], l)
	}
	out := make([]placement.Candidate, len(cands))
	copy(out, cands)
	for i := range out {
		out[i].Workloads = byNode[out[i].ID]
	}
	return out, nil
}

// saveRules applies the rules a placement body carried and keeps the ones it
// left out.
func (rt *Router) saveRules(ctx context.Context, name string, body placementBody) error {
	cur, err := rt.rulesFor(ctx, store.NodeMoveKindApp, name)
	if err != nil {
		return err
	}
	if body.Tolerations != nil {
		cur.Tolerations = *body.Tolerations
	}
	if body.Affinity != nil {
		cur.Affinity = normalizeAffinity(body.Affinity)
	}
	tj, aj := placement.EncodeRules(cur.Tolerations, cur.Affinity)
	return rt.multiNode.store.UpdateServicePlacementRules(ctx, name, tj, aj)
}
