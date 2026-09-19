package sshclient

type State int32

const (
	StateDown State = iota
	StateConnecting
	StateUp
)

func (s State) String() string {
	switch s {
	case StateDown:
		return "down"
	case StateConnecting:
		return "connecting"
	case StateUp:
		return "up"
	}
	return "unknown"
}