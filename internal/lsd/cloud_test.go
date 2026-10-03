package lsd

import "testing"

func TestBootstrapCredential(t *testing.T) {
	const base = "https://sunshine.example.org"
	tests := []struct {
		name, input, base, want string
	}{
		{"token", "secret-token", base, "secret-token"},
		{"trim token", " secret-token\n", base, "secret-token"},
		{"short code", "FGUHG1", base, "FGUHG1"},
		{"long link", base + "/install/u/secret-token", base, "secret-token"},
		{"online short link", base + "/a/FGUHG1", base, "FGUHG1"},
		{"trailing base slash", base + "/a/FGUHG1", base + "/", "FGUHG1"},
		{"local server", "http://localhost:3000/a/FGUHG1", "http://localhost:3000", "FGUHG1"},
		{"base path", base + "/sunshine/a/FGUHG1", base + "/sunshine", "FGUHG1"},
		{"empty", " ", base, ""},
		{"shell command", "curl -fsSL " + base + "/a/FGUHG1 | sh", base, ""},
		{"header injection", "secret\nother", base, ""},
		{"other server", "https://other.example.org/a/FGUHG1", base, ""},
		{"other scheme", "http://sunshine.example.org/a/FGUHG1", base, ""},
		{"other port", base + ":444/a/FGUHG1", base, ""},
		{"userinfo", "https://user@sunshine.example.org/a/FGUHG1", base, ""},
		{"offline link", base + "/c/FGUHG1", base, ""},
		{"offline alias", base + "/i/FGUHG1", base, ""},
		{"announce query", base + "/install/u/secret-token?mode=announce", base, ""},
		{"fragment", base + "/a/FGUHG1#fragment", base, ""},
		{"wrong path", base + "/account", base, ""},
		{"missing token", base + "/install/u/", base, ""},
		{"extra path", base + "/a/FGUHG1/extra", base, ""},
		{"encoded whitespace", base + "/a/FGU%0AHG1", base, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := bootstrapCredential(tt.input, tt.base)
			if tt.want == "" {
				if err == nil {
					t.Fatal("expected invalid credential to be rejected")
				}
			} else if err != nil || got != tt.want {
				t.Fatalf("credential = %q, error = %v; want %q", got, err, tt.want)
			}
		})
	}
}

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
