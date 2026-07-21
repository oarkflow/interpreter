# 39 — Daily Tools (Files, Archive, Images, Media, Office, Secrets, System, Network)

Source: `pkg/builtins/tools` (part of the root `github.com/oarkflow/interpreter`
module — unlike the `plugins/*` packages (server, database, xql, money,
phone, ip, ...), it's linked into **every** binary that imports the root
module at all, with no separate plugin blank-import required). Virtual
modules: `tools/files`, `tools/archive`, `tools/images`, `tools/office`,
`tools/secrets`, `tools/media`, `tools/system`, `tools/network`.

Every mutating operation follows the same **preview-first** convention:
returns an `Operation{status, op, src, dst, reason, error, size_before,
size_after}` describing what *would* happen (`status: "planned"`) unless
`{"apply": true}` is passed, in which case it actually runs
(`status: "applied"`).

## `tools/files`

```spl
import "tools/files";

let plan = bulk_rename("photos", {
    "match": "*.jpg", "template": "{name}_{seq}.{ext}", "apply": false
});
print plan;
// [{status:"planned", op:"rename", src:".../a.jpg", dst:".../a_001.jpg", ...}, ...]

let found = file_search("photos", {"match": "*.jpg"});
// [{path, name, size, mode, mod_time, is_dir, mime}, ...]

let sum = file_checksum("photos/a.jpg");
// {path, sha256, size}

file_move_plan(src, dst[, {"apply": true}]);
file_copy_plan(src, dst[, {"apply": true}]);
file_remove_plan(path[, {"apply": true}]);
file_organize("./downloads", "./downloads/by-type", {"apply": false});
file_dedupe("./photos"[, opts]);
```

### `file_finder` — fluent alternative to `file_search`

```spl
let results = file_finder("photos")
    .files()
    .ext("jpg")
    .size(0, 1000000)
    .sort("size", true)
    .limit(10)
    .exec();
```

Chainable methods: `.files()/.dirs()/.any()`, `.match()/.pattern_type()`,
`.regex()`, `.name()`, `.path_contains()/.path_regex()`,
`.content()/.content_regex()`, `.ext()`, `.size(min, max)`,
`.modified(after, before)`, `.recursive(bool)`, `.max_depth()`, `.limit()`,
`.sort(key[, desc])`, `.exec()`.

## `tools/archive`

```spl
import "tools/archive";

let plan = archive_compress("photos", "photos.zip", {"format": "zip", "apply": true});
print plan; // {status:"applied", op:"compress", size_before:128, size_after:264, ...}

let list = archive_list("photos.zip");
// [{path, name, size, is_dir}, ...] — lists WITHOUT extracting

archive_extract("photos.zip", "restored/"[, {"apply": true}]);
```

Supports zip/tar/gzip, inferred from the destination extension.

## `tools/images` (file-to-file, no in-memory codec package required)

```spl
import "tools/images";

image_convert_batch("./photos", "./web", {"to": "png", "apply": true}); // alias: image_optimize
image_crop_file(src, dst, {"x":0,"y":0,"width":100,"height":100,"apply":true});
image_resize_file(src, dst, {"width":200,"apply":true});
image_thumbnail(src, dst, {"size":256,"apply":true});
let info = image_info_file("photo.jpg"); // metadata straight from a file path
```

This is distinct from the in-memory `plugins/images` package (doc 27), which
requires blank-importing that plugin (`cmd/interpreter` does; a lightweight
embedding host built against only the root module doesn't) — these
file-to-file operations are part of the root module itself and work
regardless.

## `tools/media`

```spl
import "tools/media";
print ffmpeg_status();
// {ffmpeg: true, ffmpeg_path: "/opt/homebrew/bin/ffmpeg", ffprobe: true, ffprobe_path: "...", install_command: [...]}

media_info("clip.mov");                       // uses ffprobe if exec-allowed
media_convert("input.mov", "output.mp4"[, {"install": true, "apply": true}]);
ffmpeg_install({"apply": true});               // detects brew/winget/apt-get/dnf/yum/pacman/apk
```

## `tools/office`

```spl
import "tools/office";
let text = office_text("report.docx"); // supports .txt/.md/.log/.csv/.json/.docx/.xlsx
let doc = office_read("data.csv");
// {path, name, size, ext, rows: [[...],...]}  (.csv/.xlsx add "rows"; .json adds "value"; others add "text")
```

## `tools/secrets`

```spl
import "tools/secrets";
print secret_generate(16);   // masked SECRET, random alphanumeric
print token_generate(8);     // masked SECRET, random URL-safe token bytes
file_encrypt(src, dst, passphrase[, {"apply": true}]); // scrypt-derived key
file_decrypt(src, dst, passphrase[, {"apply": true}]);
```

## `tools/system` / `tools/network`

```spl
import "tools/system";
print system_info();
// {os, arch, cpus, hostname, cwd, go_version}

import "tools/network";
print dns_lookup("localhost");        // ["127.0.0.1", "::1"]
print tcp_check("127.0.0.1:80", 500); // {address, ok, duration_ms, error}
print http_probe("https://example.com"[, timeout_ms]);
```

`system_info` requires the `system` capability; `dns_lookup`/`tcp_check`/
`http_probe` require the `network` capability.

## CLI equivalents

Every one of these has a matching `spltool` subcommand (doc 42):
`spltool files rename/organize/checksum`, `spltool archive compress/extract`,
`spltool image convert/resize`, `spltool office read`,
`spltool media ffmpeg-status/convert/install-ffmpeg`,
`spltool secrets generate/encrypt/decrypt`.
