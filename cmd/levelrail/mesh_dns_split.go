package main

import "github.com/GLINCKER/levelrail/internal/network"

// meshLocalAnswer is the split horizon hook slot; nil until that feature fills it.
func meshLocalAnswer() network.LocalAnswerFunc { return nil }
