# 37 — Template Engine & Directives

Source: `pkg/template` (built-in `{{var}}` renderer, part of the root
`github.com/oarkflow/interpreter` module), `plugins/template` (optional
package wrapping `github.com/oarkflow/spl`, a full `@directive` HTML
templating engine; linked only into `cmd/interpreter`, including its
`--playground` mode). `docs/SPL_DIRECTIVES_COMPLETE_GUIDE.md`,
`docs/SPL_QUICK_REFERENCE.md`, `docs/README_SPL_DIRECTIVES.md` are the
in-repo deep-dive references this document summarizes and verifies against.

**Important**: this is a distinct system from the SPL *scripting language*
covered in docs 01–19 — it's an HTML/SSR template layer consumed via
`res.render(...)`/`res.render_ssr(...)` (doc 29).

## Two renderer implementations, selected by what's linked

| Binary | Renderer used by `res.render(...)` | Template syntax |
|---|---|---|
| A custom embedding host built against only the root module (no `plugins/template`) | `pkg/template`'s simple built-in renderer | `{{var}}` / `{{#items}}...{{/items}}` (Mustache-ish) |
| `cmd/interpreter` (links `plugins/template`) | the full directive engine (`github.com/oarkflow/spl`) | `${expr}` + `@directive` syntax |

`plugins/template`'s `init()` self-registers via
`RegisterTemplateRuntimeFactory`, **replacing** the simple renderer
process-wide — under `cmd/interpreter` (which links every `plugins/*`
package unconditionally), `{{var}}`-style templates are no longer
interpolated; the file is instead parsed for `${...}`/`@...` syntax. Only a
custom host that omits `plugins/template` (while still using `res.render`
from the root module) sees the simple renderer.

### Verified: simple renderer (root-module-only embedding host)

`views/simple.html`:
```text
Hello {{name}}! Count: {{count}}
```
```spl
let app = web_app("./views");
route(app, "GET", "/", function(req, res) {
    res.render("simple.html", {"name": "World", "count": 3});
});
```
Response body: `Hello World! Count: 3`

### Verified: directive renderer (`cmd/interpreter`)

`views/directive.html`:
```html
<div>Hello ${name}!</div>
@if(count > 0) {
  <p>You have ${count} items</p>
} @else {
  <p>No items</p>
}
@for(item in items) {
  <li>${item}</li>
}
```
```spl
let app = web_app("./views");
route(app, "GET", "/", function(req, res) {
    res.render("directive.html", {"name": "World", "count": 3, "items": ["a","b","c"]});
});
```
Response body:
```html
<div>Hello World!</div>

  <p>You have 3 items</p>

  <li>a</li>

  <li>b</li>

  <li>c</li>
```

## Expression syntax: `${...}`

```text
${var}
${obj.prop}
${arr[0]}
${fn(a, b)}
${a + b}
${cond ? a : b}
${raw_html}                 // no escaping
${text | uppercase}          // filter
${text | lowercase | capitalize}  // chainable filters
```

Filters: `uppercase`, `lowercase`, `capitalize`, `length`.

## Full directive catalog (29 directives)

**State**: `@signal(name = init)`, `@let(var = expr)` (server-only), `@computed(var = expr)` (alias of `@let`)

**Binding / events**: `@bind(signal[, attr])` (default attr `textContent`),
`@handler(name) { stmt }`, `@click(label, signal, action[, value])`
(actions: `toggle|inc|set`), `@watch(expr) { body }` (server-only)

**Control flow**: `@if/@elseif/@else`, `@for(item in items)` /
`@for(key, value in hash)` (with `$loop.index/index1/first/last/length`),
`@switch/@case/@default`, `@match/@case(pattern if guard)/@default`
(destructuring supported)

**Reactive** (client-side, requires `RenderSSR()` hydration):
`@effect(dep1, dep2, ...) { body }` (re-renders on signal change,
preserves focus/scroll), `@reactive(...)` (alias of `@effect`)

**Components / slots**: `@component("Name"[, props]) {...}`,
`@render("Name"[, props]) {...}`, `@slot([name])`, `@fill("name") {...}`

**Layout / includes**: `@extends("path")` (must be first directive),
`@define("name") {...}`, `@block("name") { default }`,
`@include("path"[, data])`, `@import("path")`

**Streaming / lazy**: `@stream {...}` (flush immediately),
`@defer {...} @fallback {...}`, `@lazy(cond) {...} @fallback {...}`
(server-only), `@raw("path")` (unescaped include)

**Utility**: `@//` comment

Server-only directives (never sent to the client): `@let`, `@computed`,
`@watch`, `@lazy`. Both server + client: `@signal`, `@handler`. Client-side
only (require SSR hydration): `@effect`, `@reactive`, and the `data-spl-*`
attribute bindings below.

## HTML attribute bindings (hydration-time, `data-spl-*`)

```html
<form on:submit.prevent="handleSubmit">      <!-- data-spl-on-submit + -mods="prevent" -->
<input bind:value="form.email">              <!-- data-spl-bind-value -->
<input data-spl-model="form.email">          <!-- two-way binding -->
<div data-spl-if="showPanel"> ... </div>
```

Event modifiers: `.prevent .stop .capture .once .passive`. API-binding
attributes (`data-spl-api-url`, `-method`, `-event`, `-target`, `-parse`,
`-body`, `-content-type`, `-form`, `-reset`) wire a DOM element directly to
a fetch call without hand-written JS.

## Component example

```html
@component("Card", title = "Default Title") {
  <div class="card"><h3>${title}</h3>@slot()</div>
}
@render("Card", {title: "My Card"}) {
  <p>Card content here</p>
  @fill("footer") { <button>Close</button> }
}
```

## Client runtime (`window.SPL`, after hydration)

`SPL.read/write/subscribe(name...)`, `SPL.signal/setSignal/toggle`,
`SPL.registerHandler`, `SPL.executeEvent`, `SPL.evalExpression`,
`SPL.interpolate`, `SPL.patch(root)`, `SPL.captureFocus/restoreFocus`,
`SPL.getRenderStats()`. Unused feature groups (bindings/models/events/
API/conditionals/focus) are tree-shaken out of the generated client bundle.

## Full-featured example app

See `examples/app` for a complete scaffolded application using
`res.render_ssr(...)` with signals/effects/components, and
`docs/SPL_DIRECTIVES_COMPLETE_GUIDE.md` for the exhaustive reference this
document summarizes.
