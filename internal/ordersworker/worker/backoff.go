package worker

import "time"

type Backoff struct{ Base, Max time.Duration }

func (b Backoff) Duration(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	d := b.Base
	for i := 1; i < attempt && d < b.Max; i++ {
		if d > b.Max/2 {
			return b.Max
		}
		d *= 2
	}
	if d > b.Max {
		return b.Max
	}
	return d
}
