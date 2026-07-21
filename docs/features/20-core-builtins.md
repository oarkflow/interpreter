# 20 — Core Builtins

Source: `pkg/builtins/core.go`. These are always available in every build
(no optional package required).

## Type introspection

```spl
print len([1,2,3]);      // 3
print len("hello");      // 5
print keys({"a":1,"b":2}); // ["a","b"]
print type(42);          // "INTEGER"
print typeof("x");       // "string"
print is_int(1);         // true
print is_float(1.0);     // true
print is_string("x");    // true
print is_array([1]);     // true
print is_hash({});       // true
print is_null(null);     // true
print is_number(1);      // true
print is_bool(true);     // true
print is_function(function(){}); // true
```

`type(x)` returns the internal `ObjectType` name (e.g. `"INTEGER"`);
`typeof x` (the prefix operator, doc 04) returns a friendlier lowercase
string (`"integer"`). Prefer `typeof` in scripts; `type()` is closer to the
Go-level type tag.

## Conversion & parsing

```spl
print to_int("42");        // 42
print to_float("3.14");    // 3.14
print to_string(42);       // "42"
print parse_string(x);     // like to_string, generic parse entry point
print parse_bool("true");  // true
print parse_int("ff", 16); // 255 (optional base)
print parse_float("3.14"); // 3.14
```

## Output

```spl
puts("raw output, no trailing newline handling like print");
print upper("abc");  // ABC
print lower("ABC");  // abc
```

## Misc

```spl
print split("a,b,c", ","); // ["a","b","c"]
print join(["a","b","c"], "-"); // "a-b-c"
print push([1,2], 3);      // [1,2,3] — pure function, returns a new array
print time();               // current unix timestamp (seconds)
sleep(100);                  // block for 100ms
let name = input("Name: ");  // read a line from stdin
print random();               // random float [0,1) or random(max) for an int
seed_random(42);              // seed the PRNG for reproducible sequences
```

## Math (small core set — see doc 23 for the full trig/stats library)

```spl
print abs(-5);    // 5
print pow(2, 10); // 1024
print sqrt(16);   // 4
print min(3,1,2); // 1
print max(3,1,2); // 3
```

## Collections (small core set — see doc 21 for the full collection library)

```spl
print contains([1,2,3], 2);   // true
print sort([3,1,2]);          // [1,2,3] (homogeneous int/string only)
print uniq([1,1,2,2,3]);      // [1,2,3]
print find([1,2,3], 2);       // 2
print reduce([1,2,3], "sum"); // 6 — string-op reducer ("sum"/"concat"), distinct from Array.reduce(fn)
print range(5);                // [0,1,2,3,4]
print range(1, 10, 2);         // [1,3,5,7,9]
```

## Concurrency-adjacent core builtins

```spl
let f1 = go_async(function() { return 1; });
print await_all([f1]);
print await_race([f1]);
```

(See doc 12 for the full concurrency model.)

## Crypto & secrets (small set — see doc 25 for the full crypto/encoding library)

```spl
print random_bytes(4);          // base64-encoded random bytes
print random_string(8, "abc");  // random string from an alphabet
print uuid();                    // v7 (time-ordered) by default
print uuid(4);                   // v4 (fully random)
print hash("sha256", "hello");
print hmac("sha256", "key", "data");
print password_hash("secret");
print password_verify("secret", password_hash("secret")); // true
print encrypt("aes_gcm", "0123456789abcdef", "hello");
print decrypt("aes_gcm", "0123456789abcdef", cipher);
print constant_time_eq("a", "a"); // true — timing-safe comparison
print base64_encode("hello"); print base64_decode("aGVsbG8=");
print hex_encode("hi");        print hex_decode("6869");
print url_encode("a b");       print url_decode("a%2Bb");
print json_encode({"a": 1});   print json_decode('{"a":1}');
print secret("x");                 // wraps as SECRET, prints as ***
print secret_reveal(secret("x"));  // "x"
print secret_mask("mypassword");   // "********rd" (default 2 visible chars)
print password_generate(16);
print api_key("sk", 24);
```

## Errors & observability

```spl
let e = Error("bad", {"code": "E1"});
print e; // {code: "E1", message: "bad", name: "Error", stack: ""}

metric("requests", 1);
trace("checkpoint", {"stage": "init"});
let frozen = immutable({"a": 1}); // see doc 18
```

## File / OS / process execution

`read_file`, `write_file`, `file_exists`, `remove_file`, `os_env`, `exit`,
`exec` are documented in doc 27 (Filesystem, OS & Process Execution).

## Config loading

`config_load(path[, format])`, `config_parse(raw, format)` are documented in
doc 46 (Config Loading & Secrets Masking).

## Formatting

`sprintf`, `printf`, `interpolate` are documented in doc 26 (Formatting &
Interpolation).
