package hostprovider

import (
	"context"
	"errors"
	"net/http"
	"testing"
)

func deps(dmi map[string]string, meta map[string]string) Deps {
	return Deps{
		ReadFile: func(p string) ([]byte, error) {
			if v, ok := dmi[p]; ok {
				return []byte(v + "\n"), nil
			}
			return nil, errors.New("absent")
		},
		Get: func(_ context.Context, url string, _ http.Header) (int, []byte, error) {
			if v, ok := meta[url]; ok {
				return 200, []byte(v), nil
			}
			return 0, nil, errors.New("timeout")
		},
	}
}

func TestDetect(t *testing.T) {
	const dmiDir = "/sys/class/dmi/id/"
	cases := []struct {
		name string
		dmi  map[string]string
		meta map[string]string
		want ID
		src  string
	}{
		{"digitalocean dmi", map[string]string{dmiDir + "sys_vendor": "DigitalOcean"}, nil, DigitalOcean, SourceDMI},
		{"aws dmi", map[string]string{dmiDir + "sys_vendor": "Amazon EC2"}, nil, AWS, SourceDMI},
		{"gcp dmi", map[string]string{dmiDir + "product_name": "Google Compute Engine"}, nil, GCP, SourceDMI},
		{"hetzner dmi", map[string]string{dmiDir + "sys_vendor": "Hetzner"}, nil, Hetzner, SourceDMI},
		{"azure asset tag", map[string]string{dmiDir + "chassis_asset_tag": "7783-7084-3265-9085-8269-3286-77"}, nil, Azure, SourceDMI},
		{"oracle asset tag", map[string]string{dmiDir + "chassis_asset_tag": "OracleCloud.com"}, nil, Oracle, SourceDMI},
		{"vultr dmi", map[string]string{dmiDir + "sys_vendor": "Vultr"}, nil, Vultr, SourceDMI},
		{"linode dmi", map[string]string{dmiDir + "sys_vendor": "Linode"}, nil, Linode, SourceDMI},
		{"metadata only", nil, map[string]string{"http://169.254.169.254/hetzner/v1/metadata": "instance-id: 1"}, Hetzner, SourceMetadata},
		{"vultr metadata needs its field", nil, map[string]string{"http://169.254.169.254/v1.json": `{"instanceid":"x"}`}, Vultr, SourceMetadata},
		{"unrelated metadata body is ignored", nil, map[string]string{"http://169.254.169.254/v1.json": `{}`}, "", ""},
		{"bare metal", map[string]string{dmiDir + "sys_vendor": "Dell Inc."}, nil, "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			g, ok := Detect(context.Background(), deps(tc.dmi, tc.meta))
			if tc.want == "" {
				if ok {
					t.Fatalf("unexpected %+v", g)
				}
				return
			}
			if !ok || g.ID != tc.want || g.Source != tc.src || g.DocsURL == "" || len(g.Steps) == 0 || len(g.Ports) != 2 {
				t.Fatalf("got %+v ok=%v", g, ok)
			}
		})
	}
}
