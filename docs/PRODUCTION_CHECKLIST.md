# Production Checklist

- [ ] CI green (`go test`, `go test -race`, fuzz smoke, bench smoke, `govulncheck`)
- [ ] Runtime limits configured (`SPL_MAX_RECURSION`, `SPL_MAX_STEPS`, `SPL_EVAL_TIMEOUT_MS`, `SPL_MAX_HEAP_MB`)
- [ ] Security policy configured for target environment (`SPL_SECURITY_MODE`, allow/deny envs)
- [ ] `exec` policy explicitly configured (`SPL_DISABLE_EXEC` and/or `SPL_EXEC_ALLOW_CMDS`)
- [ ] Network/db/file policies reviewed for least privilege
- [ ] Rollback version identified and tested
- [ ] Release notes completed with compatibility impact
