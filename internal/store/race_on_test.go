//go:build race

package store

// raceEnabled lets slow tests skip under -short -race only.
const raceEnabled = true
