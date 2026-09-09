package photos

// The ordered stream performs the same successive time-window passes as the
// browser route. Each pass retains one pending cluster, emitting completed
// clusters to the next pass. Memory is independent of the input count.
type routeStream struct {
	radius  int
	pending [len(routeAggregationWindows)]routeCluster
	used    [len(routeAggregationWindows)]bool
	emit    func(routeCluster) error
}

func newRouteStream(radius int, emit func(routeCluster) error) *routeStream {
	return &routeStream{radius: NormalizeRouteClusterRadiusMeters(radius), emit: emit}
}

func (s *routeStream) add(cluster routeCluster) error { return s.pass(0, cluster) }

func (s *routeStream) pass(stage int, cluster routeCluster) error {
	if stage == len(routeAggregationWindows) {
		return s.emit(cluster)
	}
	last := &s.pending[stage]
	if !s.used[stage] {
		*last = cluster
		s.used[stage] = true
		return nil
	}
	if routeDistanceMeters(last.lat, last.lon, cluster.lat, cluster.lon) <= float64(s.radius) &&
		cluster.started.Sub(last.ended) <= routeAggregationWindows[stage] {
		mergeRouteCluster(last, cluster)
		return nil
	}
	if err := s.pass(stage+1, *last); err != nil {
		return err
	}
	*last = cluster
	return nil
}

func (s *routeStream) finish() error {
	for stage := range routeAggregationWindows {
		if s.used[stage] {
			if err := s.pass(stage+1, s.pending[stage]); err != nil {
				return err
			}
			s.used[stage] = false
		}
	}
	return nil
}
