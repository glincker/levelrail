package main

import "testing"

func TestWantsRepoURLDeploy(t *testing.T) {
	cases := []struct {
		args []string
		want bool
	}{
		{[]string{"https://github.com/acme/shop"}, true},
		{[]string{"--name", "x", "git@github.com:acme/shop.git"}, true},
		{[]string{"./site"}, false},
		{[]string{"."}, false},
		{[]string{"myapp", "--image", "reg/app:1"}, false},
		{nil, false},
	}
	for _, c := range cases {
		if got := wantsRepoURLDeploy(c.args); got != c.want {
			t.Errorf("%v = %v, want %v", c.args, got, c.want)
		}
	}
}
