package render

import (
	"context"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/oarkflow/interpreter/pkg/object"
	"github.com/oarkflow/interpreter/pkg/security"
)

func TestResolveDataURIHonorsByteCap(t *testing.T) {
	payload := base64.StdEncoding.EncodeToString([]byte("hello"))
	_, err := Resolve(context.Background(), &object.Environment{RenderConfig: &object.RenderConfig{MaxBytes: 4}}, &object.RenderArtifact{
		Kind:      "text",
		SourceTyp: "data",
		Source:    "data:text/plain;base64," + payload,
	})
	if err == nil || !strings.Contains(err.Error(), "exceeds 4 bytes") {
		t.Fatalf("expected byte-cap error, got %v", err)
	}
}

func TestResolvePathDeniedByPolicy(t *testing.T) {
	file := t.TempDir() + "/secret.txt"
	t.Setenv("SPL_RENDER_ALLOW_URLS", "0")
	if err := os.WriteFile(file, []byte("secret"), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	_, err := security.WithSecurityPolicyOverride(&object.SecurityPolicy{
		StrictMode: true,
	}, func() (any, error) {
		return Resolve(context.Background(), &object.Environment{RenderConfig: object.DefaultRenderConfig()}, &object.RenderArtifact{
			Kind:   "file",
			Source: file,
		})
	})
	if err == nil || !strings.Contains(err.Error(), "file read denied") {
		t.Fatalf("expected file-read denial, got %v", err)
	}
}

func TestResolveURLRequiresRenderURLConfig(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("hello"))
	}))
	defer server.Close()

	_, err := Resolve(context.Background(), &object.Environment{RenderConfig: object.DefaultRenderConfig()}, &object.RenderArtifact{
		Kind:      "text",
		SourceTyp: "url",
		Source:    server.URL,
	})
	if err == nil || !strings.Contains(err.Error(), "URL rendering is disabled") {
		t.Fatalf("expected disabled URL error, got %v", err)
	}
}

func TestResolveURLAllowedHost(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("hello"))
	}))
	defer server.Close()
	host, err := security.HostFromTarget(server.URL)
	if err != nil {
		t.Fatalf("host: %v", err)
	}

	result, err := security.WithSecurityPolicyOverride(&object.SecurityPolicy{
		AllowedCapabilities: []string{security.CapabilityNetwork},
		AllowedNetworkHosts: []string{host},
	}, func() (any, error) {
		return Resolve(context.Background(), &object.Environment{RenderConfig: &object.RenderConfig{
			Mode:          "auto",
			MaxBytes:      1024,
			AllowURLs:     true,
			AllowURLHosts: []string{host},
		}}, &object.RenderArtifact{
			Kind:      "text",
			SourceTyp: "url",
			Source:    server.URL,
		})
	})
	if err != nil {
		t.Fatalf("resolve URL: %v", err)
	}
	res := result.(*ResolvedArtifact)
	if res.Content != "hello" || res.MIME != "text/plain" {
		t.Fatalf("unexpected resolved artifact: %#v", res)
	}
}

func TestRenderResolvedForTerminalMetadataFallback(t *testing.T) {
	out := RenderResolvedForTerminal(&ResolvedArtifact{
		Kind: "image",
		Name: "pic.png",
		MIME: "image/png",
		Size: 12,
	}, &object.RenderConfig{TerminalProtocol: "none"}, "auto")
	if !strings.Contains(out, "<image pic.png image/png 12 bytes>") {
		t.Fatalf("unexpected fallback: %q", out)
	}
}
