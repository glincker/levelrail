package main

import (
	"os"
	"strconv"

	"github.com/glincker/levelrail/internal/changes"
	"github.com/glincker/levelrail/internal/telemetry"
)

// withLogSignatures lets the change aggregator report new error types after
// a deploy, reading at most APP_LOG_SIGNATURE_MAX_ROWS lines per window.
func withLogSignatures(agg *changes.Aggregator, src telemetry.BoundedLogQuerier) *changes.Aggregator {
	rows, _ := strconv.Atoi(os.Getenv("APP_LOG_SIGNATURE_MAX_ROWS"))
	agg.Logs = telemetry.NewSignatureDetector{Src: src, MaxRows: rows}
	return agg
}
