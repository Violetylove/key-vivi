package app

import (
	"math"
	"os"
	"strconv"
	"time"
)

func readyHintDuration() time.Duration {
	const fallback = 1500 * time.Millisecond
	raw := os.Getenv("KEYVIVI_READY_HINT")
	if raw == "" {
		return fallback
	}
	seconds, err := strconv.ParseFloat(raw, 64)
	if err != nil || math.IsNaN(seconds) || math.IsInf(seconds, 0) || seconds < 0 || seconds > 3600 {
		return fallback
	}
	return time.Duration(seconds * float64(time.Second))
}
