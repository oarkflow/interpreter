package tcpguard

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/oarkflow/interpreter/pkg/eval"
	"github.com/oarkflow/interpreter/pkg/object"
	"github.com/oarkflow/interpreter/plugins/server"
)

// policyBlockAdmin mirrors the README's minimal "protect-admin" example:
// block requests under /admin/* with a missing or sqlmap-flavored
// User-Agent, allow everything else. Passed as an inline BCL block (not a
// file) to exercise tcpguard_load's block-string mode.
const policyBlockAdmin = `
pack "example-security-pack" {
  version "1.0.0"
  mode enforce
}

guard "tcpguard-main" {
  mode enforce
  version "test"
}

rule "protect-admin" {
  scope {
    methods ["GET", "POST"]
    paths ["/admin/*"]
  }

  trigger {
    on request.received
  }

  when {
    any {
      request.user_agent equals ""
      request.user_agent contains "sqlmap"
    }
  }

  risk {
    base 90
  }

  actions {
    critical {
      run block
    }
  }
}
`

func mustBuiltin(t *testing.T, name string) *object.Builtin {
	t.Helper()
	b, ok := eval.BuiltinByName(name)
	if !ok {
		t.Fatalf("builtin %q not registered", name)
	}
	return b
}

func TestGuardMiddlewareBlocksAndAllows(t *testing.T) {
	loadResult := builtinTCPGuardLoad(&object.String{Value: policyBlockAdmin})
	arr, ok := loadResult.(*object.Array)
	if !ok || len(arr.Elements) != 2 || arr.Elements[1] != object.NULL {
		t.Fatalf("tcpguard_load failed: %s", loadResult.Inspect())
	}
	bundle := arr.Elements[0].(*TCPGuardBundle)

	newResult := builtinTCPGuardNew(bundle)
	arr, ok = newResult.(*object.Array)
	if !ok || len(arr.Elements) != 2 || arr.Elements[1] != object.NULL {
		t.Fatalf("tcpguard_new failed: %s", newResult.Inspect())
	}
	guard := arr.Elements[0].(*SPLGuard)

	srvObj := mustBuiltin(t, "server").Fn(&object.Integer{Value: 0})
	srv, ok := srvObj.(*server.SPLServer)
	if !ok {
		t.Fatalf("server() did not return *server.SPLServer: %s", srvObj.Inspect())
	}

	okHandler := &object.Builtin{Fn: func(args ...object.Object) object.Object {
		res := args[1].(*server.SPLResponse)
		res.Ctx.Type("text/plain")
		_ = res.Ctx.SendString("ok")
		return object.NULL
	}}
	mustBuiltin(t, "route").Fn(srv, &object.String{Value: "GET"}, &object.String{Value: "/hello"}, okHandler)
	mustBuiltin(t, "route").Fn(srv, &object.String{Value: "GET"}, &object.String{Value: "/admin/secret"}, okHandler)

	if errObj := builtinGuardMiddleware(srv, guard); object.IsError(errObj) {
		t.Fatalf("guard_middleware failed: %s", errObj.Inspect())
	}

	port := freePort(t)
	env := object.NewEnvironment()
	listenResult := mustBuiltin(t, "listen_async").FnWithEnv(env, srv, &object.Integer{Value: int64(port)})
	if object.IsError(listenResult) {
		t.Fatalf("listen_async failed: %s", listenResult.Inspect())
	}
	time.Sleep(50 * time.Millisecond)
	base := fmt.Sprintf("http://127.0.0.1:%d", port)
	defer func() { _ = mustBuiltin(t, "shutdown").Fn(srv) }()

	// /hello with a normal User-Agent: outside the guarded scope, always allowed.
	if status, _ := doGet(t, base+"/hello", "MyTestClient/1.0"); status != http.StatusOK {
		t.Fatalf("/hello status = %d, want 200", status)
	}

	// /admin/secret with a normal User-Agent: in scope but condition doesn't match.
	if status, _ := doGet(t, base+"/admin/secret", "MyTestClient/1.0"); status != http.StatusOK {
		t.Fatalf("/admin/secret (normal UA) status = %d, want 200", status)
	}

	// /admin/secret with a sqlmap User-Agent: blocked by the guard.
	if status, body := doGet(t, base+"/admin/secret", "sqlmap/1.6"); status == http.StatusOK {
		t.Fatalf("/admin/secret (sqlmap UA) status = %d, want a block status, body=%s", status, body)
	}
}

func freePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("find free port: %v", err)
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port
}

func doGet(t *testing.T, url, userAgent string) (int, string) {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	if userAgent != "" {
		req.Header.Set("User-Agent", userAgent)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}
