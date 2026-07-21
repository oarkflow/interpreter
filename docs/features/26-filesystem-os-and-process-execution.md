# 26 — Filesystem, OS & Process Execution

Source: `pkg/builtins/core.go` (file/OS/exec basics),
`pkg/builtins/filesystem.go` (directory/path helpers). All path-based
builtins are sandboxed through path sanitization and
`security.CheckFileReadAllowed`/`CheckFileWriteAllowed` — reads/writes are
rooted to the sandbox/module directory by default (doc 45).

## Reading & writing files

```spl
let ok, err = write_file("test_io.txt", "Hello File System!");
print ok;  // true
print err; // null

let content, rerr = read_file("test_io.txt");
print content; // "Hello File System!"

print file_exists("test_io.txt"); // true

let ok2, err2 = remove_file("test_io.txt");
print ok2; // true
```

All three follow the `[value, error]` tuple convention.

## Environment variables

```spl
print os_env("HOME");        // read
os_env("MY_VAR", "hello");   // write (returns ok/err depending on policy)
print os_env("MY_VAR");      // "hello"
```

Writing is gated by `SPL_ALLOW_ENV_WRITE` / the active security policy (doc
45); `SPL_`-prefixed variables and dynamic-linker variables
(`PATH`, `LD_PRELOAD`, ...) can never be mutated regardless of policy.

## Directory & path helpers (`filesystem.go`)

```spl
let names, derr = readdir(".");           // [names[], err]
let matches, gerr = glob("*.txt");         // [paths[], err]
mkdir("subdir"[, perm]);
rmdir("subdir");
let info, serr = stat("test_io.txt");
// [{name, size, mode, mod_time, is_dir}, err]
chmod("path", 0o644);

print basename("/a/b/c.txt");  // "c.txt"
print dirname("/a/b/c.txt");   // "/a/b"
print path_join("a", "b", "c.txt"); // "a/b/c.txt"
```

## Process execution: `exec`

```spl
let output = exec("echo", "hello-exec", 1000);
print output; // "hello-exec"
```

Unlike most integration builtins, `exec` returns the command's **captured
stdout directly as a string** (not a `[value, error]` tuple) on success.
`exec(cmd, ...args, timeoutMs)`:

- is **command-whitelisted** — only commands explicitly allowed by policy
  (`--allow-exec` / `SPL_EXEC_ALLOW_CMDS`) or, in a fully trusted/default
  context, any command on `PATH`, may run;
- can be globally disabled with `SPL_DISABLE_EXEC=1`;
- bounds execution with a timeout (in milliseconds, the trailing numeric
  argument);
- requires the `exec` capability under a restrictive security policy.

```spl
permissions({"strict": true});
let denied = try {
    exec("date", 1000);
} catch (e) {
    e; // policy-denial message
};
permissions({"strict": false});
```

## Process exit

```spl
exit();     // exit code 0
exit(1);    // exit code 1
```

`exit` requires the `process_exit` capability and is disabled entirely
under `SPL_PROTECT_HOST=1` (doc 45).

## See also

- doc 39 (Daily Tools) for higher-level, preview-first file/archive/media
  operations (`bulk_rename`, `file_organize`, `archive_compress`, ...).
- doc 45 (Security Policy & Sandboxed Execution) for the full capability and
  allow/deny-list model governing all of the above.
