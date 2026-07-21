# 23 — Math Builtins

Source: `pkg/builtins/math.go` (constants + trig/log), `pkg/builtins/enhancements.go`
(extra numeric helpers). See doc 20 for `abs`/`pow`/`sqrt`/`min`/`max` and
doc 21 for `clamp`/`round`/`floor`/`ceil`.

## Constants (zero-arg functions)

```spl
print PI();   // 3.141592653589793
print E();    // 2.718281828459045
print INF();  // +Inf
print NAN();  // NaN
```

## Trigonometry & logarithms

```spl
print sin(0); print cos(0); print tan(0);
print asin(1); print acos(1); print atan(1); print atan2(1, 1);
print sinh(0); print cosh(0); print tanh(0);
print log(E());   // 1  (natural log)
print log2(8);     // 3
print log10(100);  // 2
print exp(1);        // 2.718281828459045
print hypot(3, 4);   // 5
print to_radians(180); // 3.141592653589793
print to_degrees(3.14159265); // ~179.9999...
```

## Special-value checks

```spl
print is_nan(NAN());   // true
print is_inf(INF());   // true
print is_finite(1.0);  // true
print is_integer(5.0); // true
```

## Extra numeric helpers (`enhancements.go`)

```spl
print cbrt(27);            // 3
print mod(10, 3);          // 1
print sign(-5);            // -1
print trunc(3.9);          // 3
print round_to(3.14159, 2);// 3.14
print lerp(0, 10, 0.5);    // 5   — linear interpolation
print normalize(5, 0, 10); // 0.5 — value's fraction within [min,max]
print map_range(5, 0, 10, 0, 100); // 50 — rescale between two ranges
print percent(25, 200);    // 12.5
print factorial(5);        // 120
print gcd(12, 18);         // 6
print lcm(4, 6);           // 12
print is_prime(17);        // true
```

## Randomness

```spl
print random_float();          // [0,1)
print random_choice([1,2,3]);  // one random element
print shuffle([1,2,3,4,5]);    // a shuffled copy
print sample([1,2,3,4,5], 2);  // 2 random elements, no repeats
```

`random()`/`random_range(min, max)` (doc 20/21) and `seed_random(n)` (doc 20)
round out the core PRNG surface; `random_bytes`/`random_string`/`uuid` (doc
25) cover cryptographic/identifier-oriented randomness.

## Statistics

`mean`, `median`, `mode`, `variance`, `stddev`, `percentile` are documented
in doc 21 (Collection Builtins) since they operate over arrays.
