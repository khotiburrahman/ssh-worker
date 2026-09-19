package health

type State int32

const (
	StateHealthy State = iota
	StateDegraded
	StateUnhealthy
	StateRecovering
)

func (s State) String() string {
	switch s {
	case StateHealthy:
		return "healthy"
	case StateDegraded:
		return "degraded"
	case StateUnhealthy:
		return "unhealthy"
	case StateRecovering:
		return "recovering"
	}
	return "unknown"
}

func (s State) Healthy() bool { return s == StateHealthy }