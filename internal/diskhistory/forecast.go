package diskhistory

import (
	"math"
	"time"
)

// Defaults from the storage page spec: fit the last 30 days, and stay
// hidden until a week of data exists so the number is never a guess.
const (
	DefaultWindowDays = 30
	DefaultMinDays    = 7
	secondsPerDay     = 24 * 60 * 60
	maxForecastDays   = 36500.0
)

// Result is a days-until-full estimate and how many days of data it rests on.
type Result struct {
	DaysUntilFull float64
	BasisDays     int
}

// Forecast fits used bytes over the last DefaultWindowDays with least squares.
// It returns nil when under DefaultMinDays of data exists or usage is not growing.
func Forecast(samples []Sample) *Result {
	return ForecastWith(samples, DefaultWindowDays, DefaultMinDays)
}

// ForecastWith is Forecast with explicit window and minimum day counts.
func ForecastWith(samples []Sample, windowDays, minDays int) *Result {
	usable := make([]Sample, 0, len(samples))
	for _, s := range samples {
		if s.TotalBytes > 0 {
			usable = append(usable, s)
		}
	}
	if len(usable) < 2 {
		return nil
	}
	latest := usable[0].At
	for _, s := range usable {
		if s.At.After(latest) {
			latest = s.At
		}
	}
	cutoff := latest.Add(-time.Duration(windowDays) * 24 * time.Hour)
	var win []Sample
	days := map[string]struct{}{}
	for _, s := range usable {
		if s.At.Before(cutoff) {
			continue
		}
		win = append(win, s)
		days[s.At.UTC().Format("2006-01-02")] = struct{}{}
	}
	if len(days) < minDays || len(win) < 2 {
		return nil
	}
	origin := win[0].At
	for _, s := range win {
		if s.At.Before(origin) {
			origin = s.At
		}
	}
	var n, sx, sy, sxx, sxy float64
	var free int64
	for _, s := range win {
		x := s.At.Sub(origin).Seconds() / secondsPerDay
		y := float64(s.TotalBytes - s.FreeBytes)
		n++
		sx += x
		sy += y
		sxx += x * x
		sxy += x * y
		if s.At.Equal(latest) {
			free = s.FreeBytes
		}
	}
	den := n*sxx - sx*sx
	if den == 0 {
		return nil
	}
	slope := (n*sxy - sx*sy) / den
	if slope <= 0 {
		return nil
	}
	return &Result{DaysUntilFull: math.Min(float64(free)/slope, maxForecastDays), BasisDays: len(days)}
}

// BasisDays counts the distinct UTC days that have a sample.
func BasisDays(samples []Sample) int {
	days := map[string]struct{}{}
	for _, s := range samples {
		days[s.At.UTC().Format("2006-01-02")] = struct{}{}
	}
	return len(days)
}
