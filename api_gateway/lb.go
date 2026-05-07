package api_gateway

import (
	"fmt"
	"hash/fnv"
	"math"
	"math/rand"
	"net/http"
	"sort"
	"sync"
	"sync/atomic"
)

// Strategy + Sticky mode constants. Empty Strategy → round_robin.
const (
	StrategyRoundRobin = "round_robin"
	StrategyWeighted   = "weighted"
	StrategyRandom     = "random"
	StrategyLeastConn  = "least_conn"

	StickyIPHash = "ip_hash"
	StickyCookie = "cookie"
	StickyHeader = "header"

	defaultStickyCookieName = "redock_lb"
)

// upstreamRuntime holds per-upstream state that does not live in JSON config:
// round-robin counter and per-service in-flight counts (for least_conn).
type upstreamRuntime struct {
	rrCounter uint64
	inflight  sync.Map // serviceID -> *int64
}

func (u *upstreamRuntime) inflightFor(serviceID string) *int64 {
	if v, ok := u.inflight.Load(serviceID); ok {
		return v.(*int64)
	}
	var zero int64
	actual, _ := u.inflight.LoadOrStore(serviceID, &zero)
	return actual.(*int64)
}

// runtime returns (creating if necessary) the runtime state for an upstream.
func (g *Gateway) runtime(upstreamID string) *upstreamRuntime {
	g.upstreamRuntimeMu.Lock()
	defer g.upstreamRuntimeMu.Unlock()
	rt, ok := g.upstreamRuntimes[upstreamID]
	if !ok {
		rt = &upstreamRuntime{}
		g.upstreamRuntimes[upstreamID] = rt
	}
	return rt
}

// pickBackend resolves a Route's upstream to a healthy *Service using its
// load-balancing strategy and optional session affinity. Returns the service,
// the resolved upstream (for caller-side concerns like sticky-cookie writes),
// and a non-nil "release" callback the caller MUST invoke once the request
// finishes (it decrements the in-flight counter for least_conn).
//
// On error (no upstream, all unhealthy, etc.) the release callback is a no-op.
func (g *Gateway) pickBackend(route *Route, r *http.Request) (*Service, *Upstream, func(), error) {
	noop := func() {}

	g.mu.RLock()
	upstream, ok := g.upstreams[route.UpstreamID]
	g.mu.RUnlock()
	if !ok || upstream == nil {
		return nil, nil, noop, fmt.Errorf("upstream %q not found", route.UpstreamID)
	}
	if !upstream.Enabled {
		return nil, upstream, noop, fmt.Errorf("upstream %q is disabled", upstream.ID)
	}

	healthy, weights := g.healthyTargets(upstream)
	if len(healthy) == 0 {
		return nil, upstream, noop, fmt.Errorf("no healthy targets in upstream %q", upstream.ID)
	}

	rt := g.runtime(upstream.ID)
	svc := selectTarget(upstream, healthy, weights, rt, r)
	if svc == nil {
		return nil, upstream, noop, fmt.Errorf("upstream %q failed to select a target", upstream.ID)
	}

	if upstream.Strategy == StrategyLeastConn {
		ctr := rt.inflightFor(svc.ID)
		atomic.AddInt64(ctr, 1)
		return svc, upstream, func() { atomic.AddInt64(ctr, -1) }, nil
	}
	return svc, upstream, noop, nil
}

// applyStickyCookie writes a sticky-binding cookie on the outgoing response
// when an upstream uses cookie-based affinity. Refreshes TTL on every request.
func applyStickyCookie(w http.ResponseWriter, upstream *Upstream, serviceID string) {
	if upstream == nil || upstream.Sticky == nil || upstream.Sticky.Mode != StickyCookie {
		return
	}
	name := upstream.Sticky.CookieName
	if name == "" {
		name = defaultStickyCookieName
	}
	c := &http.Cookie{
		Name:     name,
		Value:    serviceID,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	}
	if upstream.Sticky.TTLSeconds > 0 {
		c.MaxAge = upstream.Sticky.TTLSeconds
	}
	http.SetCookie(w, c)
}

// healthyTargets returns the subset of an upstream's targets that resolve to
// enabled, currently-healthy services, plus a parallel weights slice.
func (g *Gateway) healthyTargets(upstream *Upstream) ([]*Service, []int) {
	g.mu.RLock()
	defer g.mu.RUnlock()

	out := make([]*Service, 0, len(upstream.Targets))
	weights := make([]int, 0, len(upstream.Targets))
	for _, t := range upstream.Targets {
		svc, ok := g.services[t.ServiceID]
		if !ok || svc == nil || !svc.Enabled {
			continue
		}
		if h, ok := g.serviceHealth[t.ServiceID]; ok && h != nil && !h.Healthy {
			continue
		}
		w := t.Weight
		if w <= 0 {
			w = 1
		}
		out = append(out, svc)
		weights = append(weights, w)
	}
	return out, weights
}

func selectTarget(upstream *Upstream, healthy []*Service, weights []int, rt *upstreamRuntime, r *http.Request) *Service {
	// Sticky resolution short-circuits the strategy where applicable.
	if upstream.Sticky != nil {
		switch upstream.Sticky.Mode {
		case StickyIPHash:
			return hrwSelect(healthy, weights, getClientIP(r), upstream.Strategy == StrategyWeighted)
		case StickyHeader:
			key := r.Header.Get(upstream.Sticky.HeaderName)
			if key == "" {
				key = getClientIP(r)
			}
			return hrwSelect(healthy, weights, key, upstream.Strategy == StrategyWeighted)
		case StickyCookie:
			name := upstream.Sticky.CookieName
			if name == "" {
				name = defaultStickyCookieName
			}
			if c, err := r.Cookie(name); err == nil && c.Value != "" {
				if svc := findService(healthy, c.Value); svc != nil {
					return svc
				}
			}
			// fall through: distribute by Strategy and let the proxy set cookie on response
		}
	}
	return distribute(upstream.Strategy, healthy, weights, rt)
}

func distribute(strategy string, healthy []*Service, weights []int, rt *upstreamRuntime) *Service {
	switch strategy {
	case StrategyRandom:
		return healthy[rand.Intn(len(healthy))]
	case StrategyWeighted:
		return weightedRandom(healthy, weights)
	case StrategyLeastConn:
		return leastConnPick(healthy, rt)
	case StrategyRoundRobin, "":
		idx := atomic.AddUint64(&rt.rrCounter, 1) - 1
		return healthy[idx%uint64(len(healthy))]
	}
	return healthy[0]
}

func weightedRandom(healthy []*Service, weights []int) *Service {
	total := 0
	for _, w := range weights {
		total += w
	}
	if total <= 0 {
		return healthy[rand.Intn(len(healthy))]
	}
	x := rand.Intn(total)
	for i, w := range weights {
		if x < w {
			return healthy[i]
		}
		x -= w
	}
	return healthy[len(healthy)-1]
}

func leastConnPick(healthy []*Service, rt *upstreamRuntime) *Service {
	var best *Service
	var bestN int64 = math.MaxInt64
	// Stable order so ties pick deterministically (helps tests + behavior).
	idx := make([]int, len(healthy))
	for i := range idx {
		idx[i] = i
	}
	sort.SliceStable(idx, func(i, j int) bool { return healthy[idx[i]].ID < healthy[idx[j]].ID })
	for _, i := range idx {
		n := atomic.LoadInt64(rt.inflightFor(healthy[i].ID))
		if n < bestN {
			bestN = n
			best = healthy[i]
		}
	}
	return best
}

// hrwSelect implements Highest Random Weight (Rendezvous) hashing.
// For each target it computes h = hash(key|serviceID); if weighted is true
// the score is -ln(h/maxUint64) / weight (standard weighted HRW), else the
// score is the raw hash. The maximum-score target wins.
func hrwSelect(healthy []*Service, weights []int, key string, weighted bool) *Service {
	if len(healthy) == 1 {
		return healthy[0]
	}
	var best *Service
	var bestScore float64 = -math.MaxFloat64
	var bestUnweighted uint64
	for i, svc := range healthy {
		h := hash64(key + "|" + svc.ID)
		if weighted {
			w := weights[i]
			if w <= 0 {
				w = 1
			}
			u := float64(h) / float64(math.MaxUint64)
			if u <= 0 {
				u = 1e-12
			}
			score := -math.Log(u) / float64(w)
			if score > bestScore {
				bestScore = score
				best = svc
			}
		} else {
			if best == nil || h > bestUnweighted {
				bestUnweighted = h
				best = svc
			}
		}
	}
	return best
}

func findService(healthy []*Service, id string) *Service {
	for _, s := range healthy {
		if s.ID == id {
			return s
		}
	}
	return nil
}

func hash64(s string) uint64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(s))
	return h.Sum64()
}
