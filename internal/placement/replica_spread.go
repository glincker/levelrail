package placement

import (
	"errors"
	"fmt"
)

// ErrSpreadStateful is returned when an app with local storage is asked to spread.
var ErrSpreadStateful = errors.New("placement: an app with volumes cannot spread its replicas")

// SpreadBlock returns why an app may not spread its replicas, or "" when it may.
// Named volumes and bind mounts live on one node, so a second replica would not share them.
func SpreadBlock(volumes, bindMounts int) string {
	if volumes > 0 || bindMounts > 0 {
		return "the app has volumes or bind mounts, which live on one node. Spread only stateless apps"
	}
	return ""
}

// PlanReplicas picks a node for each of replicas indexes. A replica keeps its
// current node while that node is still eligible, so a scale does not move the
// others. New replicas go to the eligible node holding the fewest replicas.
func PlanReplicas(cands []Candidate, req Request, replicas int, current []string) ([]string, error) {
	if replicas < 1 {
		return nil, fmt.Errorf("placement: replicas must be at least 1, got %d", replicas)
	}
	eligible := map[string]bool{}
	for _, c := range cands {
		if Evaluate(c, req).Eligible {
			eligible[c.ID] = true
		}
	}
	plan := make([]string, replicas)
	placed := make([]bool, replicas)
	count := map[string]int{}
	for i := 0; i < replicas && i < len(current); i++ {
		if current[i] != "" && eligible[current[i]] {
			plan[i], placed[i] = current[i], true
			count[current[i]]++
		}
	}
	for i := range plan {
		if placed[i] {
			continue
		}
		least := -1
		for id := range eligible {
			if least < 0 || count[id] < least {
				least = count[id]
			}
		}
		var pool []Candidate
		for _, c := range cands {
			if eligible[c.ID] && count[c.ID] == least {
				pool = append(pool, c)
			}
		}
		d, err := Spread(pool, req)
		if err != nil {
			return nil, err
		}
		plan[i] = d.NodeID
		count[d.NodeID]++
	}
	return plan, nil
}

// SpreadsNodes reports how many distinct nodes a plan uses.
func SpreadsNodes(plan []string) int {
	seen := map[string]bool{}
	for _, n := range plan {
		seen[n] = true
	}
	return len(seen)
}
