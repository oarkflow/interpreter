// Package emailvalidator wraps github.com/oarkflow/ev, exposing email
// syntax/DNS/SMTP validation, disposable-domain detection, role-account
// detection, and free-provider detection as SPL builtins.
//
// DNS and SMTP checks touch the network, so email_validate() is gated
// behind the "network" capability whenever either check is enabled (DNS is
// on by default; SMTP probing is opt-in via opts.check_smtp). Syntax-only
// validation and the disposable/role/free-provider lookups never touch the
// network (the disposable list and role/free-provider tables are embedded
// in the ev package) and are always available.
package emailvalidator

import (
	"context"
	"maps"
	"strings"
	"sync"
	"time"

	"github.com/oarkflow/ev"

	"github.com/oarkflow/interpreter/pkg/eval"
	"github.com/oarkflow/interpreter/pkg/object"
	"github.com/oarkflow/interpreter/pkg/security"
)

func init() {
	eval.RegisterPluginBuiltins(map[string]*object.Builtin{
		// email_validate(email[, opts]) -> (result, err)
		// Runs the full layered validation: syntax, disposable/role/free-
		// provider detection, DNS (enabled by default), and optionally SMTP
		// mailbox probing. `opts` HASH (all optional):
		//   check_dns: BOOLEAN, default true
		//   check_smtp: BOOLEAN, default false - active SMTP RCPT probing;
		//     slower and can be blocked/rate-limited by mail servers.
		//   timeout_ms: INTEGER, default 5000 - overall context timeout.
		// Returns a HASH shaped like ev.Result (see resultToHash) as the
		// first tuple element; err is non-null only for cancellation/
		// timeout or a disabled capability, never for an invalid address
		// (that's reflected in result.verdict/result.reasons instead).
		"email_validate": {Fn: builtinEmailValidate},

		// email_validate_syntax(email) -> HASH
		// Syntax/normalization only - no network access. Shaped like
		// ev.SyntaxResult (status, normalized, local, domain, smtp_utf8,
		// domain_literal, error).
		"email_validate_syntax": {Fn: builtinEmailValidateSyntax},

		// email_is_disposable(email_or_domain) -> BOOLEAN
		"email_is_disposable": {Fn: builtinEmailIsDisposable},

		// email_is_role_account(email_or_local) -> BOOLEAN
		// True for shared/non-personal mailboxes like admin@, support@,
		// noreply@ (checked against the local part before any "+tag").
		"email_is_role_account": {Fn: builtinEmailIsRoleAccount},

		// email_is_free_provider(email_or_domain) -> BOOLEAN
		// True for common consumer webmail domains (gmail.com, etc).
		"email_is_free_provider": {Fn: builtinEmailIsFreeProvider},

		// email_validate_bulk(records[, field][, opts]) -> HASH report
		// Validates an email field across many records in one call, e.g.
		// rows loaded via read_json/read_csv/table_rows, db_query, or
		// xql_run. `records` accepts an ARRAY (of HASH rows, or plain
		// STRING elements - e.g. a bare array/slice of addresses) or a
		// TABLE_VALUE (as returned by read_csv). A bad or undeliverable
		// value in one record never aborts the batch - it is reported
		// per-record instead. `field` is the HASH/row key holding the email
		// STRING; omit it (or pass null) when records are plain strings,
		// e.g. email_validate_bulk(["a@example.com", "b@example.com"]).
		// Optional `opts` HASH:
		//   check_dns: BOOLEAN, default false - unlike email_validate(),
		//     bulk defaults DNS off so large batches stay fast and don't
		//     require the network capability unless explicitly requested.
		//   check_smtp: BOOLEAN, default false - active SMTP RCPT probing;
		//     slower and can be blocked/rate-limited by mail servers.
		//   timeout_ms: INTEGER, default 5000 - per-record context timeout.
		//   workers: INTEGER, default 1 (serial) when neither check_dns nor
		//     check_smtp is set, else 8 - concurrency is only useful once
		//     records do real network I/O; capped at 64.
		// Returns {total, valid_count, invalid_count, results}. `valid`
		// reflects syntax validity only (always computed regardless of
		// opts, matching phone_parse_bulk/ip_lookup_bulk's "valid" meaning
		// "well-formed", not "deliverable"); each results[i] is a *flat*
		// row: the original record's fields (if it was a HASH) plus
		// input/valid/error, and, on successful parse, the same fields as
		// email_validate's result - ready to hand straight to
		// write_json/write_csv/db_exec, e.g.
		// write_csv("verified.csv", report.results).
		"email_validate_bulk": {Fn: builtinEmailValidateBulk},
	})
}

var disposableDetector = ev.NewDisposableDetector()

// domainOrLocalPart returns the domain (wantDomain=true) or local part
// (wantDomain=false) of s, splitting on the last "@" if present; if s has
// no "@" it's assumed to already be the requested part.
func domainOrLocalPart(s string, wantDomain bool) string {
	if i := strings.LastIndexByte(s, '@'); i >= 0 {
		if wantDomain {
			return s[i+1:]
		}
		return s[:i]
	}
	return s
}

func builtinEmailIsDisposable(args ...object.Object) object.Object {
	if len(args) != 1 {
		return object.NewError("email_is_disposable() takes 1 argument (email_or_domain), got %d", len(args))
	}
	s, errObj := asString(args[0], "email_or_domain")
	if errObj != nil {
		return errObj
	}
	return object.NativeBoolToBooleanObject(disposableDetector.IsDisposable(domainOrLocalPart(s, true)))
}

func builtinEmailIsRoleAccount(args ...object.Object) object.Object {
	if len(args) != 1 {
		return object.NewError("email_is_role_account() takes 1 argument (email_or_local), got %d", len(args))
	}
	s, errObj := asString(args[0], "email_or_local")
	if errObj != nil {
		return errObj
	}
	return object.NativeBoolToBooleanObject(ev.IsRoleAccountLocal(domainOrLocalPart(s, false)))
}

func builtinEmailIsFreeProvider(args ...object.Object) object.Object {
	if len(args) != 1 {
		return object.NewError("email_is_free_provider() takes 1 argument (email_or_domain), got %d", len(args))
	}
	s, errObj := asString(args[0], "email_or_domain")
	if errObj != nil {
		return errObj
	}
	return object.NativeBoolToBooleanObject(ev.IsFreeProviderDomain(domainOrLocalPart(s, true)))
}

func builtinEmailValidateSyntax(args ...object.Object) object.Object {
	if len(args) != 1 {
		return object.NewError("email_validate_syntax() takes 1 argument (email), got %d", len(args))
	}
	s, errObj := asString(args[0], "email")
	if errObj != nil {
		return errObj
	}
	return syntaxResultToHash(ev.ValidateSyntax(s, ev.AddressOptions{}))
}

// checkOpts holds the fields shared by email_validate's and
// email_validate_bulk's `opts` HASH.
type checkOpts struct {
	checkDNS  bool
	checkSMTP bool
	timeoutMS int
	workers   int
}

// parseCheckOpts reads check_dns/check_smtp/timeout_ms/workers out of an
// opts HASH. defaultCheckDNS lets callers differ on whether DNS is on by
// default (email_validate: yes; email_validate_bulk: no, so large batches
// stay fast and network-capability-free unless explicitly requested).
func parseCheckOpts(optsHash *object.Hash, defaultCheckDNS bool) (checkOpts, object.Object) {
	opts := checkOpts{checkDNS: defaultCheckDNS, timeoutMS: 5000}
	if optsHash == nil {
		return opts, nil
	}
	if v, ok := hashGet(optsHash, "check_dns"); ok {
		b, ok := v.(*object.Boolean)
		if !ok {
			return opts, object.NewError("argument `opts.check_dns` must be BOOLEAN, got %s", v.Type())
		}
		opts.checkDNS = b.Value
	}
	if v, ok := hashGet(optsHash, "check_smtp"); ok {
		b, ok := v.(*object.Boolean)
		if !ok {
			return opts, object.NewError("argument `opts.check_smtp` must be BOOLEAN, got %s", v.Type())
		}
		opts.checkSMTP = b.Value
	}
	if v, ok := hashGet(optsHash, "timeout_ms"); ok {
		i, ok := v.(*object.Integer)
		if !ok {
			return opts, object.NewError("argument `opts.timeout_ms` must be INTEGER, got %s", v.Type())
		}
		opts.timeoutMS = int(i.Value)
	}
	if v, ok := hashGet(optsHash, "workers"); ok {
		i, ok := v.(*object.Integer)
		if !ok {
			return opts, object.NewError("argument `opts.workers` must be INTEGER, got %s", v.Type())
		}
		opts.workers = int(i.Value)
	}
	return opts, nil
}

// newValidatorFor builds an ev.Validator configured for the given opts.
func newValidatorFor(opts checkOpts) *ev.Validator {
	cfg := ev.DefaultConfig()
	cfg.CheckDNS = opts.checkDNS
	if !opts.checkSMTP {
		cfg.SMTP = nil
	}
	return ev.New(cfg)
}

// validateOne runs a single validation against v with opts' timeout applied.
func validateOne(ctx context.Context, v *ev.Validator, email string, timeoutMS int) (ev.Result, error) {
	if timeoutMS > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(timeoutMS)*time.Millisecond)
		defer cancel()
	}
	return v.Validate(ctx, email)
}

func builtinEmailValidate(args ...object.Object) object.Object {
	if len(args) < 1 || len(args) > 2 {
		return object.NewError("email_validate() takes 1 or 2 arguments (email[, opts]), got %d", len(args))
	}
	email, errObj := asString(args[0], "email")
	if errObj != nil {
		return errObj
	}

	var optsHash *object.Hash
	if len(args) == 2 {
		h, ok := args[1].(*object.Hash)
		if !ok {
			return object.NewError("argument `opts` must be HASH, got %s", args[1].Type())
		}
		optsHash = h
	}
	opts, errObj := parseCheckOpts(optsHash, true)
	if errObj != nil {
		return errObj
	}

	if opts.checkDNS || opts.checkSMTP {
		if err := security.CheckCapabilityAllowed(security.CapabilityNetwork); err != nil {
			return tuple(object.NULL, object.NewError("%s", err))
		}
	}

	v := newValidatorFor(opts)
	defer v.Close()

	result, err := validateOne(context.Background(), v, email, opts.timeoutMS)
	if err != nil {
		return tuple(object.NULL, object.NewError("email_validate: %s", err))
	}
	return tuple(resultToHash(result), object.NULL)
}

func builtinEmailValidateBulk(args ...object.Object) object.Object {
	if len(args) < 1 || len(args) > 3 {
		return object.NewError("email_validate_bulk() takes 1 to 3 arguments (records[, field][, opts]), got %d", len(args))
	}
	records, errObj := asRecords(args[0], "records")
	if errObj != nil {
		return errObj
	}
	field, optsHash, errObj := parseBulkFieldOpts(args)
	if errObj != nil {
		return errObj
	}
	opts, errObj := parseCheckOpts(optsHash, false)
	if errObj != nil {
		return errObj
	}
	if opts.workers <= 0 {
		if opts.checkDNS || opts.checkSMTP {
			opts.workers = 8
		} else {
			opts.workers = 1
		}
	}
	if opts.workers > 64 {
		opts.workers = 64
	}
	if opts.workers > len(records) {
		opts.workers = max(1, len(records))
	}

	if opts.checkDNS || opts.checkSMTP {
		if err := security.CheckCapabilityAllowed(security.CapabilityNetwork); err != nil {
			return object.NewError("%s", err)
		}
	}

	v := newValidatorFor(opts)
	defer v.Close()

	results := make([]object.Object, len(records))
	validCount := 0
	var validMu sync.Mutex

	jobs := make(chan int)
	var wg sync.WaitGroup
	for w := 0; w < opts.workers; w++ {
		wg.Go(func() {
			for i := range jobs {
				rec := records[i]
				value, ok := recordFieldString(rec, field)
				if !ok {
					results[i] = emailBulkResult(rec, "", nil, "missing or non-string email value")
					continue
				}
				syntax := ev.ValidateSyntax(value, ev.AddressOptions{})
				if syntax.Error != "" {
					results[i] = emailBulkResult(rec, value, nil, syntax.Error)
					continue
				}
				validMu.Lock()
				validCount++
				validMu.Unlock()

				result, err := validateOne(context.Background(), v, value, opts.timeoutMS)
				if err != nil {
					results[i] = emailBulkResult(rec, value, map[string]object.Object{
						"error": &object.String{Value: err.Error()},
					}, "")
					continue
				}
				pairs := resultHashPairs(result)
				delete(pairs, "input")
				results[i] = emailBulkResult(rec, value, pairs, "")
			}
		})
	}
	for i := range records {
		jobs <- i
	}
	close(jobs)
	wg.Wait()

	return hashFromPairs(map[string]object.Object{
		"total":         &object.Integer{Value: int64(len(records))},
		"valid_count":   &object.Integer{Value: int64(validCount)},
		"invalid_count": &object.Integer{Value: int64(len(records) - validCount)},
		"results":       &object.Array{Elements: results},
	})
}

func mxRecordsToArray(mx []ev.MXRecord) *object.Array {
	elems := make([]object.Object, len(mx))
	for i, m := range mx {
		elems[i] = hashFromPairs(map[string]object.Object{
			"host":       &object.String{Value: m.Host},
			"preference": &object.Integer{Value: int64(m.Pref)},
		})
	}
	return &object.Array{Elements: elems}
}

func reasonsToArray(reasons []ev.Reason) *object.Array {
	elems := make([]object.Object, len(reasons))
	for i, r := range reasons {
		elems[i] = hashFromPairs(map[string]object.Object{
			"code":    &object.String{Value: r.Code},
			"message": &object.String{Value: r.Message},
			"weight":  &object.Integer{Value: int64(r.Weight)},
		})
	}
	return &object.Array{Elements: elems}
}

func syntaxResultToHash(s ev.SyntaxResult) *object.Hash {
	return hashFromPairs(map[string]object.Object{
		"status":         &object.String{Value: string(s.Status)},
		"normalized":     &object.String{Value: s.Normalized},
		"local":          &object.String{Value: s.Local},
		"domain":         &object.String{Value: s.Domain},
		"smtp_utf8":      object.NativeBoolToBooleanObject(s.SMTPUTF8),
		"domain_literal": object.NativeBoolToBooleanObject(s.DomainLiteral),
		"error":          &object.String{Value: s.Error},
	})
}

func dnsResultToHash(d ev.DNSResult) *object.Hash {
	return hashFromPairs(map[string]object.Object{
		"status":      &object.String{Value: string(d.Status)},
		"mx":          mxRecordsToArray(d.MX),
		"implicit_mx": object.NativeBoolToBooleanObject(d.ImplicitMX),
		"null_mx":     object.NativeBoolToBooleanObject(d.NullMX),
		"has_a":       object.NativeBoolToBooleanObject(d.HasA),
		"has_aaaa":    object.NativeBoolToBooleanObject(d.HasAAAA),
		"error":       &object.String{Value: d.Error},
		"duration_ms": &object.Integer{Value: d.Duration.Milliseconds()},
	})
}

func smtpResultToHash(s ev.SMTPResult) *object.Hash {
	return hashFromPairs(map[string]object.Object{
		"status":        &object.String{Value: string(s.Status)},
		"mailbox":       &object.String{Value: string(s.Mailbox)},
		"catch_all":     &object.String{Value: string(s.CatchAll)},
		"host":          &object.String{Value: s.Host},
		"code":          &object.Integer{Value: int64(s.Code)},
		"enhanced_code": &object.String{Value: s.EnhancedCode},
		"message":       &object.String{Value: s.Message},
		"tls":           object.NativeBoolToBooleanObject(s.TLS),
		"cached":        object.NativeBoolToBooleanObject(s.Cached),
		"attempts":      &object.Integer{Value: int64(s.Attempts)},
		"duration_ms":   &object.Integer{Value: s.Duration.Milliseconds()},
		"error":         &object.String{Value: s.Error},
	})
}

func reputationStatsToHash(r ev.ReputationStats) *object.Hash {
	pairs := map[string]object.Object{
		"delivered":    &object.Integer{Value: int64(r.Delivered)},
		"hard_bounces": &object.Integer{Value: int64(r.HardBounces)},
		"soft_bounces": &object.Integer{Value: int64(r.SoftBounces)},
		"complaints":   &object.Integer{Value: int64(r.Complaints)},
		"unsubscribes": &object.Integer{Value: int64(r.Unsubscribes)},
		"deferred":     &object.Integer{Value: int64(r.Deferred)},
		"score":        &object.Integer{Value: int64(r.Score)},
	}
	if !r.LastEventAt.IsZero() {
		pairs["last_event_at"] = &object.String{Value: r.LastEventAt.Format(time.RFC3339)}
	}
	return hashFromPairs(pairs)
}

func resultToHash(r ev.Result) *object.Hash {
	return hashFromPairs(resultHashPairs(r))
}

// resultHashPairs is resultToHash's field set as a plain Go map, so
// email_validate_bulk can overlay it onto a per-record row (see
// emailBulkResult) without re-wrapping into a HASH and back out.
func resultHashPairs(r ev.Result) map[string]object.Object {
	return map[string]object.Object{
		"input":         &object.String{Value: r.Input},
		"normalized":    &object.String{Value: r.Normalized},
		"verdict":       &object.String{Value: string(r.Verdict)},
		"risk_score":    &object.Integer{Value: int64(r.RiskScore)},
		"syntax":        syntaxResultToHash(r.Syntax),
		"dns":           dnsResultToHash(r.DNS),
		"smtp":          smtpResultToHash(r.SMTP),
		"disposable":    object.NativeBoolToBooleanObject(r.Disposable),
		"role_account":  object.NativeBoolToBooleanObject(r.RoleAccount),
		"free_provider": object.NativeBoolToBooleanObject(r.FreeProvider),
		"reputation":    reputationStatsToHash(r.Reputation),
		"reasons":       reasonsToArray(r.Reasons),
		"checked_at":    &object.String{Value: r.CheckedAt.Format(time.RFC3339)},
		"duration_ms":   &object.Integer{Value: r.Duration.Milliseconds()},
	}
}

// asRecords normalizes a bulk builtin's `records` argument (ARRAY or
// TABLE_VALUE, as produced by read_json/read_csv/db_query/xql_run/
// table_rows) into a flat slice of per-record objects.
func asRecords(arg object.Object, name string) ([]object.Object, object.Object) {
	switch v := arg.(type) {
	case *object.Array:
		return v.Elements, nil
	case *object.TableValue:
		rows := make([]object.Object, len(v.Rows))
		for i, row := range v.Rows {
			rows[i] = hashFromObjectMap(row)
		}
		return rows, nil
	default:
		return nil, object.NewError("argument `%s` must be ARRAY or TABLE_VALUE, got %s", name, arg.Type())
	}
}

// parseBulkFieldOpts resolves the optional trailing `field` and `opts`
// arguments of a bulk builtin (args[1:]). `field` may be omitted entirely
// (records are plain values), passed as STRING or null, or - when the
// caller has no field to name at all - skipped straight to a trailing opts
// HASH, e.g. email_validate_bulk(records) or email_validate_bulk(records, opts).
func parseBulkFieldOpts(args []object.Object) (field string, opts *object.Hash, errObj object.Object) {
	switch len(args) {
	case 1:
		return "", nil, nil
	case 2:
		if h, ok := args[1].(*object.Hash); ok {
			return "", h, nil
		}
		if s, ok := args[1].(*object.String); ok {
			return s.Value, nil, nil
		}
		if args[1] == object.NULL {
			return "", nil, nil
		}
		return "", nil, object.NewError("argument 2 must be STRING, null, or HASH opts, got %s", args[1].Type())
	case 3:
		if s, ok := args[1].(*object.String); ok {
			field = s.Value
		} else if args[1] != object.NULL {
			return "", nil, object.NewError("argument `field` must be STRING or null, got %s", args[1].Type())
		}
		h, ok := args[2].(*object.Hash)
		if !ok {
			return "", nil, object.NewError("argument `opts` must be HASH, got %s", args[2].Type())
		}
		return field, h, nil
	default:
		return "", nil, object.NewError("takes 1 to 3 arguments (records[, field][, opts]), got %d", len(args))
	}
}

// recordFieldString reads a STRING value out of one record. When field is
// "" the record itself must be a STRING (records passed as plain values
// rather than HASH rows).
func recordFieldString(rec object.Object, field string) (string, bool) {
	switch v := rec.(type) {
	case *object.Hash:
		if field == "" {
			return "", false
		}
		val, ok := hashGet(v, field)
		if !ok {
			return "", false
		}
		s, ok := val.(*object.String)
		if !ok {
			return "", false
		}
		return s.Value, true
	case *object.String:
		if field != "" {
			return "", false
		}
		return v.Value, true
	default:
		return "", false
	}
}

// emailBulkResult builds one email_validate_bulk results[i] entry. When rec
// is a HASH row, its fields are flattened into the result first (so id/
// name/etc. from the source record ride along), then overlaid with
// input/valid/error and, on success, email_validate's result fields - the
// whole thing is a flat row ready to hand straight to
// write_json/write_csv/db_exec. resultPairs is nil on a syntax failure;
// errMsg is "" on success (a non-syntax error, e.g. DNS timeout, is instead
// carried as resultPairs["error"], see builtinEmailValidateBulk).
func emailBulkResult(rec object.Object, input string, resultPairs map[string]object.Object, errMsg string) *object.Hash {
	pairs := hashObjectPairs(rec)
	pairs["input"] = &object.String{Value: input}
	pairs["valid"] = object.NativeBoolToBooleanObject(errMsg == "")
	if errMsg != "" {
		pairs["error"] = &object.String{Value: errMsg}
	} else if _, ok := resultPairs["error"]; !ok {
		pairs["error"] = object.NULL
	}
	maps.Copy(pairs, resultPairs)
	return hashFromPairs(pairs)
}

// hashObjectPairs copies a HASH's pairs into a plain Go map keyed by string,
// or returns an empty map for anything else (e.g. plain-STRING records).
func hashObjectPairs(rec object.Object) map[string]object.Object {
	h, ok := rec.(*object.Hash)
	if !ok {
		return map[string]object.Object{}
	}
	pairs := make(map[string]object.Object, len(h.Pairs))
	for _, pair := range h.Pairs {
		key := pair.Key.Inspect()
		if s, ok := pair.Key.(*object.String); ok {
			key = s.Value
		}
		pairs[key] = pair.Value
	}
	return pairs
}

func hashFromObjectMap(m map[string]object.Object) *object.Hash {
	out := make(map[object.HashKey]object.HashPair, len(m))
	for k, v := range m {
		key := &object.String{Value: k}
		out[key.HashKey()] = object.HashPair{Key: key, Value: v}
	}
	return &object.Hash{Pairs: out}
}

func hashFromPairs(pairs map[string]object.Object) *object.Hash {
	out := make(map[object.HashKey]object.HashPair, len(pairs))
	for k, v := range pairs {
		key := &object.String{Value: k}
		out[key.HashKey()] = object.HashPair{Key: key, Value: v}
	}
	return &object.Hash{Pairs: out}
}

func hashGet(h *object.Hash, key string) (object.Object, bool) {
	k := &object.String{Value: key}
	pair, ok := h.Pairs[k.HashKey()]
	if !ok {
		return nil, false
	}
	return pair.Value, true
}

func asString(arg object.Object, name string) (string, object.Object) {
	if s, ok := arg.(*object.Secret); ok {
		return s.Value, nil
	}
	if arg == nil {
		return "", object.NewError("argument `%s` must be STRING, got <nil>", name)
	}
	if arg.Type() != object.STRING_OBJ {
		return "", object.NewError("argument `%s` must be STRING, got %s", name, arg.Type())
	}
	return arg.(*object.String).Value, nil
}

func tuple(values ...object.Object) *object.Array {
	return &object.Array{Elements: values}
}
