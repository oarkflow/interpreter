#!/usr/bin/env bash
# CLI-level feature tour: password-protected archives, WebP encoding, and
# env-var security hardening under the default trusted profile.
#
# These aren't SPL-scriptable (they're spltool/interpreter CLI features, or
# ambient process env vars), so unlike examples/all_in_one.spl this is a
# runnable shell script rather than a .spl file.
#
# Run from the repository root:
#   bash examples/cli_tools_demo.sh
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT

echo "Building spltool and interpreter into $WORK ..."
go build -o "$WORK/spltool" "$ROOT/cmd/spltool"
(cd "$ROOT/cmd/interpreter" && go build -o "$WORK/interpreter-full" .)

echo
echo "=== 1) Password-protected archives (github.com/yeka/zip, AES-256) ==="
mkdir -p "$WORK/to_zip"
echo "top secret contents" > "$WORK/to_zip/secret.txt"

"$WORK/spltool" archive compress "$WORK/to_zip" "$WORK/secret.zip" \
    --format zip --password 'correct horse battery staple' --apply
echo "compressed: $WORK/secret.zip"

"$WORK/spltool" archive extract "$WORK/secret.zip" "$WORK/extracted" \
    --password 'correct horse battery staple' --apply
echo "extracted contents:"
# Compressing a directory keeps that directory's own name as an entry
# prefix (to_zip/secret.txt, not secret.txt) - same behavior as an
# unencrypted zip made the same way.
cat "$WORK/extracted/to_zip/secret.txt"

echo
echo "--- extracting with the wrong password should fail ---"
if "$WORK/spltool" archive extract "$WORK/secret.zip" "$WORK/extracted_wrong" \
    --password 'not the password' --apply 2>/dev/null; then
    echo "unexpected: wrong password succeeded"
else
    echo "confirmed: wrong password was rejected"
fi

echo
echo "=== 2) WebP encoding (github.com/HugoSmits86/nativewebp, lossless) ==="
mkdir -p "$WORK/to_convert"
cat > "$WORK/make_png.go" <<'GOSRC'
package main

import (
	"image"
	"image/color"
	"image/png"
	"os"
)

func main() {
	img := image.NewNRGBA(image.Rect(0, 0, 8, 8))
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			img.Set(x, y, color.NRGBA{R: 220, G: 20, B: 60, A: 255})
		}
	}
	f, err := os.Create(os.Args[1])
	if err != nil {
		panic(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		panic(err)
	}
}
GOSRC
go run "$WORK/make_png.go" "$WORK/to_convert/source.png"

"$WORK/spltool" image convert "$WORK/to_convert" "$WORK/converted" --to webp --apply
echo "converted: $(ls "$WORK/converted"/*.webp)"

echo
echo "=== 3) Env-var security hardening under the default trusted profile ==="
cat > "$WORK/try_exec.spl" <<'SPL'
print exec("echo", "should this run?");
SPL

echo "--- without SPL_PROTECT_HOST: exec(...) is allowed ---"
"$WORK/interpreter-full" "$WORK/try_exec.spl" || echo "(unexpectedly denied)"

echo
echo "--- with SPL_PROTECT_HOST=1: exec(...) is now denied ---"
# Capture output and exit code separately rather than piping into grep: the
# interpreter is expected to exit non-zero here, and under `set -o
# pipefail` a pipeline reports that non-zero exit regardless of whether
# grep itself found a match, which would make this check always "fail".
PROTECTED_OUT="$(SPL_PROTECT_HOST=1 "$WORK/interpreter-full" "$WORK/try_exec.spl" 2>&1)" || true
echo "$PROTECTED_OUT"
if echo "$PROTECTED_OUT" | grep -qi "denied"; then
    echo "confirmed: SPL_PROTECT_HOST=1 restricted exec() under the default trusted profile"
else
    echo "unexpected: SPL_PROTECT_HOST=1 did not restrict exec()"
fi

echo
echo "=== Tour complete ==="
