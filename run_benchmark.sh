#!/bin/bash
go test -bench=BenchmarkListAllBackupHistory -benchmem ./internal/store
