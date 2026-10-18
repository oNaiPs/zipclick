package main

import (
	"fmt"
	"time"
)

// detailText is "12.3 MB of 40.0 MB — about 8 seconds left"; the estimate
// waits for two seconds of history so it does not jump around at the start.
func detailText(done, total int64, elapsed time.Duration) string {
	s := fmt.Sprintf("%s of %s", humanBytes(done), humanBytes(total))
	if elapsed < 2*time.Second || done == 0 || done >= total {
		return s
	}
	left := time.Duration(float64(elapsed) * float64(total-done) / float64(done))
	switch {
	case left < 5*time.Second:
		return s + " — a few seconds left"
	case left < time.Minute:
		return s + fmt.Sprintf(" — about %d seconds left", int(left.Seconds()+0.5)/5*5)
	case left < 2*time.Minute:
		return s + " — about a minute left"
	default:
		return s + fmt.Sprintf(" — about %d minutes left", int(left.Minutes()+0.5))
	}
}

func humanBytes(n int64) string {
	const unit = 1024 // as Explorer counts them, so the numbers match the file sizes beside them
	if n < unit {
		return fmt.Sprintf("%d bytes", n)
	}
	v, units := float64(n)/unit, "KMGT"
	i := 0
	for v >= unit && i < len(units)-1 {
		v /= unit
		i++
	}
	return fmt.Sprintf("%.1f %cB", v, units[i])
}
