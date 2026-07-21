// Package phone wraps github.com/oarkflow/phone (a libphonenumber-equivalent
// parser), exposing phone number parsing, validation, formatting, and
// country lookup as SPL builtins for daily-ops tasks like form validation
// and data cleanup.
package phone

import (
	"maps"
	"strings"

	"github.com/oarkflow/phone"

	"github.com/oarkflow/interpreter/pkg/eval"
	"github.com/oarkflow/interpreter/pkg/object"
)

func phoneTypeName(t phone.PhoneNumberType) string {
	switch t {
	case phone.FIXED_LINE:
		return "fixed_line"
	case phone.MOBILE:
		return "mobile"
	case phone.FIXED_LINE_OR_MOBILE:
		return "fixed_line_or_mobile"
	case phone.TOLL_FREE:
		return "toll_free"
	case phone.PREMIUM_RATE:
		return "premium_rate"
	case phone.SHARED_COST:
		return "shared_cost"
	case phone.VOIP:
		return "voip"
	case phone.PERSONAL_NUMBER:
		return "personal_number"
	case phone.PAGER:
		return "pager"
	case phone.UAN:
		return "uan"
	case phone.VOICEMAIL:
		return "voicemail"
	default:
		return "unknown"
	}
}

func phoneResultPairs(parsed *phone.PhoneNumber) map[string]object.Object {
	// GetCarrierForNumber only returns a name for mobile-capable numbers
	// (mobile/fixed_line_or_mobile/pager); it's "" with no error otherwise,
	// and is itself only a best-effort guess (per the library's own doc
	// comment) since numbers can be ported between carriers.
	carrier, _ := phone.GetCarrierForNumber(parsed, "en")
	region := phone.GetRegionCodeForNumber(parsed)
	pairs := map[string]object.Object{
		"valid":           object.NativeBoolToBooleanObject(phone.IsValidNumber(parsed)),
		"possible":        object.NativeBoolToBooleanObject(phone.IsPossibleNumber(parsed)),
		"e164":            &object.String{Value: phone.Format(parsed, phone.E164)},
		"international":   &object.String{Value: phone.Format(parsed, phone.INTERNATIONAL)},
		"national":        &object.String{Value: phone.Format(parsed, phone.NATIONAL)},
		"country_code":    &object.Integer{Value: int64(parsed.GetCountryCode())},
		"national_number": &object.String{Value: uint64ToString(parsed.GetNationalNumber())},
		"region":          &object.String{Value: region},
		"type":            &object.String{Value: phoneTypeName(phone.GetNumberType(parsed))},
		"carrier":         &object.String{Value: carrier},
		"network":         object.NULL,
	}
	if net, ok := lookupNetworkForCarrier(region, carrier); ok {
		pairs["network"] = networkToHash(net)
	}
	return pairs
}

// lookupNetworkForCarrier cross-references a carrier name (from
// GetCarrierForNumber) against the per-country PLMN/MCC-MNC table (from
// phone_networks' underlying dataset) by fuzzy name match, since the
// library has no direct number-to-MCC/MNC lookup - a carrier name doesn't
// uniquely identify one network record either (multiple historical/retired
// entries can share a brand), so this is a best-effort match: prefer an
// "Operational" entry, otherwise return the first name match found.
func lookupNetworkForCarrier(region, carrier string) (phone.Network, bool) {
	if carrier == "" || region == "" {
		return phone.Network{}, false
	}
	if err := phone.LoadNetworks(); err != nil {
		return phone.Network{}, false
	}
	var best phone.Network
	found := false
	for _, n := range phone.CountryNetwork[strings.ToUpper(region)] {
		name := n.Operator
		if name == "" {
			name = n.Brand
		}
		if !networkNameMatches(name, carrier) {
			continue
		}
		if !found {
			best = n
			found = true
		}
		if strings.EqualFold(n.Status, "Operational") {
			return n, true
		}
	}
	return best, found
}

func networkNameMatches(operatorName, carrier string) bool {
	a := strings.ToLower(strings.TrimSpace(operatorName))
	b := strings.ToLower(strings.TrimSpace(carrier))
	if a == "" || b == "" {
		return false
	}
	return a == b || strings.Contains(a, b) || strings.Contains(b, a)
}

func networkToHash(n phone.Network) *object.Hash {
	return hashFromPairs(map[string]object.Object{
		"mcc":          &object.String{Value: n.Mcc},
		"mnc":          &object.String{Value: n.Mnc},
		"plmn":         &object.String{Value: n.Plmn},
		"country_code": &object.String{Value: n.CountryCode},
		"country_name": &object.String{Value: n.CountryName},
		"region":       &object.String{Value: n.Region},
		"type":         &object.String{Value: n.Type},
		"brand":        &object.String{Value: n.Brand},
		"operator":     &object.String{Value: n.Operator},
		"status":       &object.String{Value: n.Status},
		"bands":        &object.String{Value: n.Bands},
		"latitude":     &object.String{Value: n.Lat},
		"longitude":    &object.String{Value: n.Long},
	})
}

func init() {
	eval.RegisterPluginBuiltins(map[string]*object.Builtin{
		// phone_parse(number[, default_region]) -> (result, err)
		// Parses and validates a phone number, e.g.
		// phone_parse("(650) 253-0000", "US"). `default_region` is a 2-letter
		// ISO country code used to interpret numbers without a leading "+";
		// it's ignored (but still required by the underlying library, so
		// pass "" or a best guess) when the number already includes a country
		// calling code, e.g. "+16502530000".
		// Includes `carrier` (best-effort guessed name, "" for non-mobile
		// numbers) and `network` (that carrier's MCC/MNC/PLMN/status entry
		// cross-referenced from the phone_networks table, or null when no
		// match is found) - both are approximations, since numbers can be
		// ported between carriers and a carrier name doesn't map to a single
		// unique network record. Use phone_networks(region) directly for the
		// full, unfiltered per-country operator table.
		"phone_parse": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) < 1 || len(args) > 2 {
					return object.NewError("phone_parse() takes 1 or 2 arguments (number[, default_region]), got %d", len(args))
				}
				number, errObj := asString(args[0], "number")
				if errObj != nil {
					return errObj
				}
				region := "ZZ"
				if len(args) == 2 {
					r, errObj := asString(args[1], "default_region")
					if errObj != nil {
						return errObj
					}
					if r != "" {
						region = r
					}
				}
				parsed, err := phone.Parse(number, region)
				if err != nil {
					return tuple(object.NULL, &object.String{Value: err.Error()})
				}
				return tuple(hashFromPairs(phoneResultPairs(parsed)), object.NULL)
			},
		},

		// phone_valid(number[, default_region]) -> BOOLEAN
		// Convenience check: true only if the number parses and is a valid,
		// dialable number. Never throws - unparseable input is simply false.
		"phone_valid": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) < 1 || len(args) > 2 {
					return object.NewError("phone_valid() takes 1 or 2 arguments (number[, default_region]), got %d", len(args))
				}
				number, errObj := asString(args[0], "number")
				if errObj != nil {
					return errObj
				}
				region := "ZZ"
				if len(args) == 2 {
					r, errObj := asString(args[1], "default_region")
					if errObj != nil {
						return errObj
					}
					if r != "" {
						region = r
					}
				}
				parsed, err := phone.Parse(number, region)
				if err != nil {
					return object.FALSE
				}
				return object.NativeBoolToBooleanObject(phone.IsValidNumber(parsed))
			},
		},

		// phone_country(country_code) -> (result, err)
		// Looks up a 2-letter ISO country code, e.g. phone_country("AU"),
		// returning its name, dialing prefix, and ISO currency.
		"phone_country": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 1 {
					return object.NewError("phone_country() takes 1 argument (country_code), got %d", len(args))
				}
				code, errObj := asString(args[0], "country_code")
				if errObj != nil {
					return errObj
				}
				country, err := phone.GetCountry(code)
				if err != nil {
					return tuple(object.NULL, &object.String{Value: err.Error()})
				}
				pairs := map[string]object.Object{
					"code":            &object.String{Value: country.Code},
					"name":            &object.String{Value: country.Name},
					"phone":           &object.String{Value: country.Phone},
					"currency":        &object.String{Value: country.Currency},
					"currency_symbol": &object.String{Value: country.CurrencySymbol},
				}
				return tuple(hashFromPairs(pairs), object.NULL)
			},
		},

		// phone_networks(country_code[, opts]) -> (result, err)
		// Lists known mobile network operators for a 2-letter ISO country
		// code, e.g. phone_networks("US"), each with its MCC (mobile country
		// code), MNC (mobile network code), PLMN, operator/brand name, and
		// status. This is a per-country reference table, not a per-number
		// lookup: a phone number's carrier (see phone_parse's `carrier`
		// field) doesn't on its own identify a unique MCC/MNC, since numbers
		// can be ported between operators. Optional `opts` HASH:
		//   status: STRING, keep only entries whose status matches exactly
		//     (case-insensitive), e.g. {"status": "Operational"} to drop
		//     retired/reserved/unknown entries (the raw dataset includes
		//     historical and non-operational entries too).
		"phone_networks": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) < 1 || len(args) > 2 {
					return object.NewError("phone_networks() takes 1 or 2 arguments (country_code[, opts]), got %d", len(args))
				}
				code, errObj := asString(args[0], "country_code")
				if errObj != nil {
					return errObj
				}
				statusFilter := ""
				if len(args) == 2 {
					optsHash, ok := args[1].(*object.Hash)
					if !ok {
						return object.NewError("argument `opts` must be HASH, got %s", args[1].Type())
					}
					if v, ok := hashGet(optsHash, "status"); ok {
						s, errObj := asString(v, "opts.status")
						if errObj != nil {
							return errObj
						}
						statusFilter = s
					}
				}
				if err := phone.LoadNetworks(); err != nil {
					return tuple(object.NULL, &object.String{Value: err.Error()})
				}
				networks := phone.CountryNetwork[strings.ToUpper(code)]
				elements := make([]object.Object, 0, len(networks))
				for _, n := range networks {
					if statusFilter != "" && !strings.EqualFold(n.Status, statusFilter) {
						continue
					}
					elements = append(elements, networkToHash(n))
				}
				return tuple(&object.Array{Elements: elements}, object.NULL)
			},
		},

		// phone_parse_bulk(records[, field][, opts]) -> HASH report
		// Parses and validates a phone number field across many records in
		// one call, e.g. rows loaded via read_json/read_csv/table_rows,
		// db_query, or xql_run. `records` accepts an ARRAY (of HASH rows, or
		// plain STRING elements - e.g. a bare array/slice of phone numbers)
		// or a TABLE_VALUE (as returned by read_csv). A bad or unparseable
		// value in one record never aborts the batch - it is reported
		// per-record instead. `field` is the HASH/row key holding the phone
		// number STRING; omit it (or pass null) when records are plain
		// strings, e.g. phone_parse_bulk(["+1650...", "+61..."]).
		// Optional `opts` HASH:
		//   default_region: STRING, fallback 2-letter region for every row.
		//   region_field:   STRING, HASH/row key holding a per-record region
		//     that overrides default_region when present.
		// Returns {total, valid_count, invalid_count, results}. Each
		// results[i] is a *flat* row: the original record's fields (if it
		// was a HASH) plus `input`, `valid`, `error` (STRING or null), and,
		// on success, the same fields as phone_parse's result - ready to
		// hand straight to write_json/write_csv/db_exec to save the
		// verified/enriched data back out, e.g.
		// write_csv("verified.csv", report.results).
		"phone_parse_bulk": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) < 1 || len(args) > 3 {
					return object.NewError("phone_parse_bulk() takes 1 to 3 arguments (records[, field][, opts]), got %d", len(args))
				}
				records, errObj := asRecords(args[0], "records")
				if errObj != nil {
					return errObj
				}
				field, optsHash, errObj := parseBulkFieldOpts(args)
				if errObj != nil {
					return errObj
				}

				defaultRegion := "ZZ"
				regionField := ""
				if optsHash != nil {
					if v, ok := hashGet(optsHash, "default_region"); ok {
						s, errObj := asString(v, "opts.default_region")
						if errObj != nil {
							return errObj
						}
						if s != "" {
							defaultRegion = s
						}
					}
					if v, ok := hashGet(optsHash, "region_field"); ok {
						s, errObj := asString(v, "opts.region_field")
						if errObj != nil {
							return errObj
						}
						regionField = s
					}
				}

				results := make([]object.Object, len(records))
				validCount := 0
				for i, rec := range records {
					value, ok := recordFieldString(rec, field)
					if !ok {
						results[i] = bulkRecordResult(rec, "", nil, "missing or non-string phone value")
						continue
					}
					region := defaultRegion
					if regionField != "" {
						if regionVal, ok := recordFieldString(rec, regionField); ok && regionVal != "" {
							region = regionVal
						}
					}
					parsed, err := phone.Parse(value, region)
					if err != nil {
						results[i] = bulkRecordResult(rec, value, nil, err.Error())
						continue
					}
					if phone.IsValidNumber(parsed) {
						validCount++
					}
					results[i] = bulkRecordResult(rec, value, phoneResultPairs(parsed), "")
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
// HASH, e.g. phone_parse_bulk(records) or phone_parse_bulk(records, opts).
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

// bulkRecordResult builds one phone_parse_bulk results[i] entry. When rec is
// a HASH row, its fields are flattened into the result first (so id/name/
// etc. from the source record ride along), then overlaid with input/valid/
// error and, on success, the phone_parse fields - the whole thing is a flat
// row ready to hand straight to write_json/write_csv/db_exec. resultPairs
// is nil on a parse failure; errMsg is "" on success.
func bulkRecordResult(rec object.Object, input string, resultPairs map[string]object.Object, errMsg string) *object.Hash {
	pairs := hashObjectPairs(rec)
	pairs["input"] = &object.String{Value: input}
	pairs["valid"] = object.FALSE
	if errMsg != "" {
		pairs["error"] = &object.String{Value: errMsg}
	} else {
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

func uint64ToString(v uint64) string {
	if v == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	return string(buf[i:])
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
