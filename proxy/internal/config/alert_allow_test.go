package config

import (
	"strings"
	"testing"
)

func TestParseAlertAllowPrivate(t *testing.T) {
	nets, hosts, err := ParseAlertAllowPrivate(" 10.20.0.0/16 , splunk.internal,QRadar.Corp.Example. ,192.168.7.9, fd00:1::/64,::ffff:10.30.0.0/112,10.40.5.6/24 ,, ")
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, n := range nets {
		got = append(got, n.String())
	}
	if want := "10.20.0.0/16 192.168.7.9/32 fd00:1::/64 10.30.0.0/16 10.40.5.0/24"; strings.Join(got, " ") != want {
		t.Errorf("ranges = %v, want %s", got, want)
	}
	if want := "splunk.internal qradar.corp.example"; strings.Join(hosts, " ") != want {
		t.Errorf("hosts = %v, want %s", hosts, want)
	}

	if nets, hosts, err := ParseAlertAllowPrivate(""); err != nil || nets != nil || hosts != nil {
		t.Errorf("empty: %v %v %v, want nothing allowed", nets, hosts, err)
	}

	for _, bad := range []string{
		"0.0.0.0/0", "::/0", "::ffff:0.0.0.0/96", "*", "*.internal", "true", "10.0.0.0/33", "10.0.0/8",
		"http://splunk.internal", "splunk.internal:8088", "splunk.internal/hec", "splunk..internal", ".internal",
		"2130706433", "0x7f.1", "127.1", "host name", "fe80::1%eth0/64",
	} {
		if bad == "true" {
			// A single label is a valid host name; "true" is not a switch.
			nets, hosts, err := ParseAlertAllowPrivate(bad)
			if err != nil || len(nets) != 0 || len(hosts) != 1 {
				t.Errorf("%q: %v %v %v, want one host name and no ranges", bad, nets, hosts, err)
			}
			continue
		}
		if _, _, err := ParseAlertAllowPrivate("10.20.0.0/16," + bad); err == nil {
			t.Errorf("%q was accepted", bad)
		} else if !strings.Contains(err.Error(), "TAMGA_ALERT_ALLOW_PRIVATE") {
			t.Errorf("%q: the error does not name the variable: %v", bad, err)
		}
	}
}

func TestLoad_AlertAllowPrivate(t *testing.T) {
	t.Setenv("TAMGA_ALERT_ALLOW_PRIVATE", "")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.AlertAllowNets) != 0 || len(cfg.AlertAllowHosts) != 0 {
		t.Fatalf("the default allows %v %v", cfg.AlertAllowNets, cfg.AlertAllowHosts)
	}
	t.Setenv("TAMGA_ALERT_ALLOW_PRIVATE", "10.20.0.0/16,splunk.internal")
	if cfg, err = Load(); err != nil || len(cfg.AlertAllowNets) != 1 || len(cfg.AlertAllowHosts) != 1 {
		t.Fatalf("Load: %+v %v", cfg, err)
	}
	t.Setenv("TAMGA_ALERT_ALLOW_PRIVATE", "0.0.0.0/0")
	if _, err = Load(); err == nil {
		t.Fatal("Load accepted an allow-everything range")
	}
}
