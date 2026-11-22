package main

import "testing"

func TestThroughputFlagsApply(t *testing.T) {
	var res serviceResources
	if err := (throughputFlags{ioRead: "50Mi", ioWrite: "1Mi", netMbit: 100}).apply(&res); err != nil {
		t.Fatal(err)
	}
	if res.IOReadBps != 50<<20 || res.IOWriteBps != 1<<20 || res.NetMbit != 100 {
		t.Fatalf("got %+v", res)
	}
	if err := (throughputFlags{netMbit: -1}).apply(&res); err == nil {
		t.Fatal("negative net-mbit must fail")
	}
	if err := (throughputFlags{ioRead: "bogus"}).apply(&res); err == nil {
		t.Fatal("bad size must fail")
	}
}
