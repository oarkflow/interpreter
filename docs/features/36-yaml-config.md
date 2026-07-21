# 36 — YAML Config

Source: `plugins/yaml` (optional package, wraps `gopkg.in/yaml.v3`; linked
only into `cmd/interpreter`, including its `--playground` mode). Registers a
YAML format parser with `pkg/config` (enabling `config_load`/`config_parse`
to handle `.yaml`/`.yml`) plus two dedicated builtins, and virtual modules
`"yaml"` and `"config/yaml"` (the latter a legacy alias name from when this
package lived at the top-level `config/yaml` module path).

## `yaml_encode` / `yaml_decode`

```spl
let doc = yaml_encode({"name": "svc", "replicas": 3, "tags": ["a", "b"]}, {"indent": 2});
print doc;
// name: svc
// replicas: 3
// tags:
//   - a
//   - b

let parsed = yaml_decode(doc);
print parsed.replicas; // 3
```

`opts.indent` (int) sets the indent width. `yaml_decode` returns `NULL` for
empty/blank input and an `object.Error` (not a plain string) on parse
failure — different from `config_load`/`config_parse`'s plain-string error
convention (doc 46).

> **Note**: `yaml_encode` serializes the **actual unwrapped value**, even
> for fields that would print masked as a `SECRET` elsewhere — it does not
> re-mask secrets on the way out. Don't `yaml_encode` a hash containing
> live secret values into a log or response body.

## `config_load(path, "yaml")`

```spl
let cfg = config_load("config.yaml", "yaml");
print cfg.name;      // svc
print cfg.replicas;  // 3
```

Without importing `yaml`/`config/yaml` (or an equivalent parser), calling
`config_load(path, "yaml")` under any binary that doesn't link this package
(e.g. a custom embedding host built against only the root module) fails
with `"unsupported config format \"yaml\"; import an optional parser
package"`.

## Secret masking on nested keys

Given:

```yaml
name: svc
replicas: 3
auth:
  username: admin
  password: supersecret
```

```spl
let cfg = config_load("config.yaml", "yaml");
print cfg.auth.username; // ***
print cfg.auth.password; // ***
print secret_reveal(cfg.auth.password); // supersecret
```

**Both `username` and `password` print masked**, even though `username`
alone isn't normally treated as a sensitive key name. Once any key inside a
nested hash (here, `password` under `auth`) matches the sensitive-key list,
the wrapping logic treats the surrounding subtree as sensitive context, so
sibling string values in the same nested hash are wrapped as `SECRET`
too. In practice: once a config section contains *any* credential-shaped
key, treat every string in that section as potentially masked, and use
`secret_reveal(...)` on whichever fields you actually need in plain form.
See doc 46 for the full secret-masking mechanism (`IsSensitiveKey`,
`ApplySecretWrapping`).

## Module aliases

```spl
import "yaml" as yaml;
yaml.config_load("config.yaml");
yaml.yaml_encode({...});

import "config/yaml" as cfg;
cfg.yaml_decode(text);
```

Both `"yaml"` and `"config/yaml"` expose the same four names:
`config_load`, `config_parse`, `yaml_encode`, `yaml_decode`.
