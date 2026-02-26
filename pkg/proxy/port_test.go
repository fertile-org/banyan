package proxy

import "testing"

func TestParsePort(t *testing.T) {
	tests := []struct {
		name          string
		input         string
		wantHost      int
		wantContainer int
		wantErr       bool
	}{
		{name: "single port", input: "80", wantHost: 80, wantContainer: 80},
		{name: "host:container", input: "80:8080", wantHost: 80, wantContainer: 8080},
		{name: "ip:host:container", input: "127.0.0.1:80:8080", wantHost: 80, wantContainer: 8080},
		{name: "same port mapping", input: "443:443", wantHost: 443, wantContainer: 443},
		{name: "with tcp suffix", input: "80:8080/tcp", wantHost: 80, wantContainer: 8080},
		{name: "with udp suffix", input: "53:53/udp", wantHost: 53, wantContainer: 53},
		{name: "single with tcp", input: "80/tcp", wantHost: 80, wantContainer: 80},
		{name: "empty string", input: "", wantErr: true},
		{name: "non-numeric", input: "abc", wantErr: true},
		{name: "non-numeric host", input: "abc:80", wantErr: true},
		{name: "non-numeric container", input: "80:abc", wantErr: true},
		{name: "too many colons", input: "1:2:3:4", wantErr: true},
		{name: "ip with bad host port", input: "127.0.0.1:abc:8080", wantErr: true},
		{name: "ip with bad container port", input: "127.0.0.1:80:abc", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			host, container, err := ParsePort(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error for input %q, got nil", tt.input)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error for input %q: %v", tt.input, err)
			}
			if host != tt.wantHost {
				t.Errorf("host port: expected %d, got %d", tt.wantHost, host)
			}
			if container != tt.wantContainer {
				t.Errorf("container port: expected %d, got %d", tt.wantContainer, container)
			}
		})
	}
}
