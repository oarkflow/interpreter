# 27 — Data Values & Image Processing

Source: `pkg/builtins/structured_data.go`, `pkg/builtins/file_values.go`,
`pkg/builtins/render.go` (always available, part of the root module), plus
the optional `plugins/images` package (only linked into `cmd/interpreter`,
including its `--playground` mode; not part of the root
`github.com/oarkflow/interpreter` module).

## Reading structured files

```spl
let data = read_json("data.json");
print data; // {name: "test", value: 42}

let table = read_csv("data.csv");
print table; // <table columns=2 rows=2>
```

`read_json`/`read_csv` return the parsed value **directly** (not a
`[value, error]` tuple); errors surface as a thrown/runtime error instead.
`read_csv` returns a `TableValue`.

## Writing structured files

```spl
let ok = write_json("out.json", {"x": 1});
let ok2 = write_csv("out.csv", table);
```

## Working with `TableValue`

```spl
print table_rows(table);    // [{name:"Alice",age:"30"}, {name:"Bob",age:"25"}]
print table_columns(table); // ["name", "age"]
print table_select(table, ["name"]); // <table columns=1 rows=2>
print table_filter(table, function(row) { return row.age > "25"; });
print table_map(table, function(row) { return {"name": row.name}; });
```

`table_filter`/`table_map` take an environment-aware callback
(`FnWithEnv`); `table_map`'s callback must return a `Hash` per row.

## CSV text (no file I/O)

```spl
let decoded = csv_decode("a,b\n1,2\n3,4");
print decoded;             // <table columns=2 rows=2>
print csv_encode(decoded); // "a,b\n1,2\n3,4\n"
```

## File values

```spl
let f = file_load("data.json");
print file_name(f); // "data.json"
print file_mime(f);  // "application/json"
print file_size(f);  // 27
print file_text(f);  // raw file contents as a string
print file_bytes(f); // base64-encoded bytes

file_save(f, "copy.json");
file_copy("data.json", "backup.json");
file_move("backup.json", "archive/backup.json");
file_rename("archive/backup.json", "renamed.json");
```

`file_load` accepts a path, a URL, an already-loaded `FileValue`, or a
render artifact.

## Render artifacts: `file()`, `image()`, `render()`

```spl
let art = file("data.json");   // wraps a path/URL/bytes as a RenderArtifact
let img = image("photo.png");  // image-flavored artifact
let r = render({"a": 1});      // generic value → artifact, for playground/UI display
```

`opts` (a trailing hash on any of the three) can set
`kind`/`source_type`/`mime`/`name`/`alt`/`mode`/`width`/`height`/`max_bytes`.
These artifacts are what the playground's `/api/execute` response returns
in its `artifacts` array (doc 46) and what the REPL/embedding render
pipeline resolves into inline data URLs, metadata-only summaries, or is
dropped entirely, depending on render mode.

## Image processing (`plugins/images`, optional — `cmd/interpreter` only)

```spl
import "images";

let img = image_load("logo.png");
print image_info(img);
// {format: "png", height: 1, mime: "image/png", name: "logo.png", size: 73, width: 1}

let resized = image_resize(img, 50, 50[, {"filter": "linear"}]);
let cropped = image_crop(img, x, y, width, height);
let rotated = image_rotate(img, 90);
let converted = image_convert(img, "jpeg"); // png|jpeg|jpg|gif
let artifact = image_render(converted);      // wrap for display
```

`image_resize`'s optional `opts.filter` selects `nearest|linear|box|mitchell`
(default Lanczos resampling); `opts.format` can set the output format at
resize time too.

Under a custom embedding host built against only the root
`github.com/oarkflow/interpreter` module (which doesn't blank-import
`plugins/images`), calling `image_load` fails with an actionable error:

```text
ERROR: optional builtin "image_load" from module "images" is not linked
into this interpreter; use cmd/interpreter (built with the full plugin
set), import the optional Go package in your embedding host, or build a
custom preset
```

For batch/file-to-file image operations (`image_resize_file`,
`image_convert_batch`, `image_thumbnail`, ...) that don't require holding
decoded images in memory, see doc 39 (Daily Tools) — those are a separate,
preview-first builtin family (`tools/images`) available even without the
in-memory `plugins/images` codec package.
