package diagnose

import (
	"net/netip"
	"reflect"
	"slices"
	"testing"
)

func TestParseListeningPorts(t *testing.T) {
	tcp := `  sl  local_address rem_address   st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode
   0: 00000000:1F90 00000000:0000 0A 00000000:00000000 00:00000000 00000000     0        0 12345 1 0000000000000000 100 0 0 10 0
   1: 0100007F:0BB8 00000000:0000 0A 00000000:00000000 00:00000000 00000000     0        0 12346 1 0000000000000000 100 0 0 10 0
   2: 0F00A8C0:01BB 0100A8C0:D2F4 01 00000000:00000000 00:00000000 00000000     0        0 12347 1 0000000000000000 100 0 0 10 0
`
	tcp6 := `  sl  local_address                         remote_address                        st
   0: 00000000000000000000000000000000:1F90 00000000000000000000000000000000:0000 0A
   1: 00000000000000000000000000000000:0050 00000000000000000000000000000000:0000 0A
`
	got := ParseListeningPorts(tcp, tcp6)
	want := []int{80, 8080, 3000}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
	if got := ParseListeningPorts("garbage\n"); len(got) != 0 {
		t.Fatalf("garbage = %v", got)
	}
}

func TestParseBoundUDP(t *testing.T) {
	v4 := "  sl  local_address rem_address   st\n" +
		"  1: 3500007F:0035 00000000:0000 07 00000000:00000000\n" +
		"  2: 00000000:CA6C 00000000:0000 07 00000000:00000000\n" +
		"  3: 0100007F:E0A1 0100007F:0035 01 00000000:00000000\n"
	v6 := "  4: 00000000000000000000000001000000:14E9 00000000000000000000000000000000:0000 07 0\n"
	got := ParseBoundUDP(v4, v6)
	want := []BoundSocket{
		{Addr: netip.MustParseAddr("127.0.0.53"), Port: 53},
		{Addr: netip.MustParseAddr("0.0.0.0"), Port: 51820},
		{Addr: netip.MustParseAddr("::1"), Port: 5353},
	}
	if !slices.Equal(got, want) {
		t.Fatalf("ParseBoundUDP() = %v, want %v", got, want)
	}
}
