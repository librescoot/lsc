package lsd

import "testing"

func TestParseConfigIdentityRadioGaga(t *testing.T) {
	yaml := `scooter:
  identifier: WUNU2S3B7MZ000147
  token: "secret"
  name: Deep Blue
environment: development
mqtt:
  broker_url: ssl://mqtt2.sunshine.rescoot.org:8883
`
	ci := parseConfigIdentity(yaml, "sunshine.rescoot.org")
	if ci.Identifier != "WUNU2S3B7MZ000147" {
		t.Errorf("identifier = %q", ci.Identifier)
	}
	if ci.Backend != "sunshine" {
		t.Errorf("backend = %q, want sunshine (host %q)", ci.Backend, hostOf(ci.ServerURL))
	}
}

func TestParseConfigIdentityUplinkCustom(t *testing.T) {
	yaml := "# comment: not a key\nuplink:\n  server_url: wss://uplink.example.org/ws\nscooter:\n  identifier: mdb-12345678\n  token: x\n"
	ci := parseConfigIdentity(yaml, "sunshine.rescoot.org")
	if ci.Identifier != "mdb-12345678" || ci.Backend != "custom" {
		t.Errorf("got %+v", ci)
	}
	if parseConfigIdentity("", "sunshine.rescoot.org").Backend != "" {
		t.Error("empty config must have no backend")
	}
}

func TestConfigPathFromUnit(t *testing.T) {
	tests := []struct{ in, want string }{
		{"ExecStart=/usr/bin/radio-gaga -config /data/radio-gaga/config.yaml -buffer-persist-path /x.json", "/data/radio-gaga/config.yaml"},
		{"ExecStartPre=/bin/mkdir -p /data/uplink-service\nExecStart=/usr/bin/uplink-service -config /data/uplink-service/uplink.yaml", "/data/uplink-service/uplink.yaml"},
		{"ExecStart=/usr/bin/svc --config=/etc/svc.yaml", "/etc/svc.yaml"},
		{"ExecStart=/usr/bin/svc", ""},
	}
	for _, tt := range tests {
		if got := configPathFromUnit(tt.in); got != tt.want {
			t.Errorf("configPathFromUnit(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
