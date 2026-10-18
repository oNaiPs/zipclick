package main

import (
	"fmt"
	"strings"
	"time"
)

// detailText is "12.3 MB of 40.0 MB, about 8 seconds left" in the current
// language; the estimate waits for two seconds of history so it does not
// jump around at the start.
func detailText(done, total int64, elapsed time.Duration) string {
	s := fmt.Sprintf(msg.Of, humanBytes(done), humanBytes(total))
	if elapsed < 2*time.Second || done == 0 || done >= total {
		return s
	}
	left := time.Duration(float64(elapsed) * float64(total-done) / float64(done))
	switch {
	case left < 5*time.Second:
		return s + ", " + msg.FewSeconds
	case left < time.Minute:
		return s + ", " + fmt.Sprintf(msg.Seconds, int(left.Seconds()+0.5)/5*5)
	case left < 2*time.Minute:
		return s + ", " + msg.Minute
	default:
		return s + ", " + fmt.Sprintf(msg.Minutes, int(left.Minutes()+0.5))
	}
}

func humanBytes(n int64) string {
	const unit = 1024 // as Explorer counts them, so the numbers match the file sizes beside them
	if n < unit {
		return fmt.Sprintf("%d %s", n, msg.Bytes)
	}
	v, i := float64(n)/unit, 0
	for v >= unit && i < len(msg.Units)-1 {
		v /= unit
		i++
	}
	return strings.Replace(fmt.Sprintf("%.1f", v), ".", msg.Decimal, 1) + " " + msg.Units[i]
}
