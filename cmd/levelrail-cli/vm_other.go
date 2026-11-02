//go:build !darwin

package main

func newVMEnv(_ func(string) (string, bool)) (*vmEnv, error) { return nil, errVMUnsupported }
