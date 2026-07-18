package supervisor

import (
	"math"
	"time"
)

type Backoff struct {
	Base   time.Duration
	Factor float64
	Max    time.Duration
}

func (b Backoff) Duration(retry int) time.Duration {
	if retry < 1 {
		retry = 1
	}
	d := float64(b.Base) * math.Pow(b.Factor, float64(retry-1))
	if d > float64(b.Max) {
		return b.Max
	}
	return time.Duration(d)
}
