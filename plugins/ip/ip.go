// Package ip wraps github.com/oarkflow/ip, exposing client-IP extraction
// from proxy headers, private/loopback address detection, and (opt-in,
// capability-gated) IP geolocation as SPL builtins. The geolocation dataset
// is fetched and cached to local disk on first use, so it's gated behind an
// explicit ip_geo_init() call rather than running at import time.
package ip

import (
	"fmt"
	"maps"
	"net"

	oip "github.com/oarkflow/ip"
	"github.com/oarkflow/ip/geoip"

	"github.com/oarkflow/interpreter/pkg/eval"
	"github.com/oarkflow/interpreter/pkg/object"
	"github.com/oarkflow/interpreter/pkg/security"
)

func init() {
	eval.RegisterPluginBuiltins(map[string]*object.Builtin{
		// ip_is_private(ip) -> BOOLEAN
		// True for loopback, RFC1918/link-local IPv4, and unique-local/
		// link-local IPv6 addresses.
		"ip_is_private": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 1 {
					return object.NewError("ip_is_private() takes 1 argument (ip), got %d", len(args))
				}
				s, errObj := asString(args[0], "ip")
				if errObj != nil {
					return errObj
				}
				parsed := net.ParseIP(s)
				if parsed == nil {
					return object.NewError("ip_is_private: %q is not a valid IP address", s)
				}
				private := parsed.IsPrivate() || parsed.IsLoopback() || parsed.IsLinkLocalUnicast()
				return object.NativeBoolToBooleanObject(private)
			},
		},

		// ip_client_from_header(remote_ip, header_value[, opts]) -> STRING
		// Extracts the real client IP from a proxy-chain header value (e.g.
		// the X-Forwarded-For value), preferring the first public address
		// and falling back to remote_ip. Optional `opts` HASH:
		//   trust_proxy: BOOLEAN, default true - when false, always returns
		//     remote_ip untouched (the safe default when the header could be
		//     spoofed by an untrusted client).
		"ip_client_from_header": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) < 2 || len(args) > 3 {
					return object.NewError("ip_client_from_header() takes 2 or 3 arguments (remote_ip, header_value[, opts]), got %d", len(args))
				}
				remote, errObj := asString(args[0], "remote_ip")
				if errObj != nil {
					return errObj
				}
				headerValue, errObj := asString(args[1], "header_value")
				if errObj != nil {
					return errObj
				}
				trustProxy := true
				if len(args) == 3 {
					optsHash, ok := args[2].(*object.Hash)
					if !ok {
						return object.NewError("argument `opts` must be HASH, got %s", args[2].Type())
					}
					if v, ok := hashGet(optsHash, "trust_proxy"); ok {
						b, ok := v.(*object.Boolean)
						if !ok {
							return object.NewError("argument `opts.trust_proxy` must be BOOLEAN, got %s", v.Type())
						}
						trustProxy = b.Value
					}
				}
				if !trustProxy {
					return &object.String{Value: remote}
				}
				result := geoip.FromHeader(remote, func(string) string { return headerValue })
				return &object.String{Value: result}
			},
		},

		// ip_geo_init() -> (ok, err)
		// Downloads (or loads a cached copy of) a local IP-to-geolocation
		// database under ~/.ipdata so ip_country/ip_lookup can resolve real
		// data. This is an explicit opt-in step, gated on the network and
		// filesystem-write capabilities, because it fetches and caches a
		// multi-megabyte third-party dataset rather than doing so silently
		// at import time.
		"ip_geo_init": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 0 {
					return object.NewError("ip_geo_init() takes no arguments, got %d", len(args))
				}
				if err := security.CheckCapabilityAllowed(security.CapabilityNetwork); err != nil {
					return tuple(object.FALSE, &object.String{Value: err.Error()})
				}
				if err := security.CheckCapabilityAllowed(security.CapabilityFilesystemWrite); err != nil {
					return tuple(object.FALSE, &object.String{Value: err.Error()})
				}
				if err := safeGeoInit(); err != nil {
					return tuple(object.FALSE, &object.String{Value: err.Error()})
				}
				return tuple(object.TRUE, object.NULL)
			},
		},

		// ip_country(ip) -> STRING
		// Looks up the 2-letter country code for an IP address. Returns ""
		// until ip_geo_init() has loaded the geolocation database.
		"ip_country": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 1 {
					return object.NewError("ip_country() takes 1 argument (ip), got %d", len(args))
				}
				s, errObj := asString(args[0], "ip")
				if errObj != nil {
					return errObj
				}
				return &object.String{Value: oip.Country(s)}
			},
		},

		// ip_lookup(ip) -> HASH
		// Full geolocation record (country, region, city, lat/long) for an
		// IP address. `found` is false until ip_geo_init() has loaded the
		// database, or if the address has no match (e.g. a private IP).
		"ip_lookup": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 1 {
					return object.NewError("ip_lookup() takes 1 argument (ip), got %d", len(args))
				}
				s, errObj := asString(args[0], "ip")
				if errObj != nil {
					return errObj
				}
				return hashFromPairs(ipLookupPairs(oip.Lookup(s)))
			},
		},

		// ip_lookup_bulk(records[, field]) -> HASH report
		// Validates and geolocates an IP address field across many records
		// in one call, e.g. rows loaded via read_json/read_csv/table_rows,
		// db_query, or xql_run. `records` accepts an ARRAY (of HASH rows, or
		// plain STRING elements - e.g. a bare array/slice of IP addresses)
		// or a TABLE_VALUE (as returned by read_csv). A bad value in one
		// record never aborts the batch - it is reported per-record instead.
		// `field` is the HASH/row key holding the IP STRING; omit it (or
		// pass null) when records are plain strings, e.g.
		// ip_lookup_bulk(["8.8.8.8", "1.1.1.1"]).
		// Returns {total, valid_count, invalid_count, results}, where
		// `valid_count` counts syntactically valid IP addresses (not
		// geolocation matches - call ip_geo_init() first for that). Each
		// results[i] is a *flat* row: the original record's fields (if it
		// was a HASH) plus `input`, `valid`, `error` (STRING or null), and,
		// for valid addresses, ip_is_private/ip_lookup's fields - ready to
		// hand straight to write_json/write_csv/db_exec to save the
		// enriched data back out, e.g. write_csv("checked.csv", report.results).
		"ip_lookup_bulk": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) < 1 || len(args) > 2 {
					return object.NewError("ip_lookup_bulk() takes 1 or 2 arguments (records[, field]), got %d", len(args))
				}
				records, errObj := asRecords(args[0], "records")
				if errObj != nil {
					return errObj
				}
				field := ""
				if len(args) == 2 {
					if s, ok := args[1].(*object.String); ok {
						field = s.Value
					} else if args[1] != object.NULL {
						return object.NewError("argument `field` must be STRING or null, got %s", args[1].Type())
					}
				}

				results := make([]object.Object, len(records))
				validCount := 0
				for i, rec := range records {
					value, ok := recordFieldString(rec, field)
					if !ok {
						results[i] = ipBulkResult(rec, "", nil, "missing or non-string ip value")
						continue
					}
					parsed := net.ParseIP(value)
					if parsed == nil {
						results[i] = ipBulkResult(rec, value, nil, fmt.Sprintf("%q is not a valid IP address", value))
						continue
					}
					validCount++
					pairs := ipLookupPairs(oip.Lookup(value))
					pairs["is_private"] = object.NativeBoolToBooleanObject(parsed.IsPrivate() || parsed.IsLoopback() || parsed.IsLinkLocalUnicast())
					results[i] = ipBulkResult(rec, value, pairs, "")
				}

				return hashFromPairs(map[string]object.Object{
					"total":         &object.Integer{Value: int64(len(records))},
					"valid_count":   &object.Integer{Value: int64(validCount)},
					"invalid_count": &object.Integer{Value: int64(len(records) - validCount)},
					"results":       &object.Array{Elements: results},
				})
			},
		},
	})
}

func ipLookupPairs(rec geoip.GeoRecord) map[string]object.Object {
	return map[string]object.Object{
		"found":        object.NativeBoolToBooleanObject(rec.Found),
		"country_code": &object.String{Value: rec.CountryCode},
		"country":      &object.String{Value: rec.Country},
		"region":       &object.String{Value: rec.Region},
		"city":         &object.String{Value: rec.City},
		"latitude":     &object.Float{Value: rec.Latitude},
		"longitude":    &object.Float{Value: rec.Longitude},
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

// ipBulkResult builds one ip_lookup_bulk results[i] entry. When rec is a
// HASH row, its fields are flattened into the result first (so id/name/etc.
// from the source record ride along), then overlaid with input/valid/error
// and, on success, the ip_lookup fields - the whole thing is a flat row
// ready to hand straight to write_json/write_csv/db_exec. lookupPairs is nil
// when the value wasn't even a valid IP address; errMsg is "" on success.
func ipBulkResult(rec object.Object, input string, lookupPairs map[string]object.Object, errMsg string) *object.Hash {
	pairs := hashObjectPairs(rec)
	pairs["input"] = &object.String{Value: input}
	pairs["valid"] = object.NativeBoolToBooleanObject(errMsg == "")
	if errMsg != "" {
		pairs["error"] = &object.String{Value: errMsg}
	} else {
		pairs["error"] = object.NULL
	}
	maps.Copy(pairs, lookupPairs)
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

// safeGeoInit calls geoip.Init(), which can panic if it cannot create its
// local cache directory under any candidate path. Builtins must never crash
// the interpreter process, so a panic here is converted into a returned
// error instead.
func safeGeoInit() (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("ip_geo_init: %v", r)
		}
	}()
	geoip.Init()
	return nil
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
