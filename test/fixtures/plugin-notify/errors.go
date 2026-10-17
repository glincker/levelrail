package main

import (
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

var errUnsupportedContract = status.Error(codes.FailedPrecondition, "no common contract version")
