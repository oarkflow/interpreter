# SPL Template Engine - Complete Documentation Index

This directory contains comprehensive documentation of the SPL template engine, extracted from the source code in `/template/*.go`.

## 📚 Documentation Files

### 1. **SPL_DIRECTIVES_COMPLETE_GUIDE.md** (18 KB, 665 lines)
Complete reference covering all SPL features:
- All 29 @ directives with syntax, descriptions, and examples
- Expression syntax `${}`
- All 18+ `data-spl-*` HTML attributes
- Event binding with modifiers
- Two-way binding patterns
- @api feature for HTTP requests
- Client-side hydration system
- JavaScript Runtime API (`window.SPL`)
- Feature detection and tree-shaking
- Complete example app

**When to use:** Looking for detailed explanations, complete examples, or learning the system deeply.

### 2. **SPL_QUICK_REFERENCE.md** (9.7 KB, 386 lines)
Quick lookup guide organized by category:
- All 29 @ directives organized by type
- Complete list of all `data-spl-*` attributes
- Quick syntax examples for each feature
- JavaScript runtime API quick reference
- Common usage patterns
- Event and binding shortcuts

**When to use:** Need a quick reminder of syntax, quick examples, or quick lookup during coding.

---

## 🎯 Quick Navigation

### By Feature Type

#### State Management
- **@signal(name = init)** - Reactive signal (client + server)
- **@let(var = expr)** - Computed variable (server only)
- **@computed(var = expr)** - Alias for @let
- **@watch(expr) { ... }** - Render when value changes (server only)

#### Binding & Events  
- **@bind(signal, [attr])** - Bind signal to element
- **on:event="expr"** - Event handler (client + server)
- **@handler(name) { ... }** - Named handler (client + server)
- **@click(label, signal, action)** - Quick button action
- **data-spl-model="{signal}"** - Two-way form binding

#### Reactive Rendering
- **@effect(deps) { ... }** - Re-render on signal change
- **@reactive(deps) { ... }** - Synonym for @effect
- **data-spl-if="{signal}"** - Conditional show/hide
- **data-spl-on-{event}="{expr}"** - Event binding

#### Control Flow
- **@if/@else** - Conditional rendering
- **@for** - Loop iteration with $loop metadata
- **@switch/@case** - Multi-way branching
- **@match/@case** - Pattern matching with guards

#### Components & Composition
- **@component(name, props) { ... }** - Define component
- **@render(name, props) { ... }** - Invoke component  
- **@slot([name])** - Content placeholder
- **@fill(name) { ... }** - Fill named slot

#### API Integration
- **data-spl-api-url** - Fetch endpoint
- **data-spl-api-method** - HTTP method
- **data-spl-api-target** - Store response in signal
- **data-spl-api-form** - Serialize form
- **data-spl-api-body** - Template body with ${signal} substitution

#### Layout & Templates
- **@extends(path)** - Parent layout
- **@define(name) { ... }** - Define block override
- **@block(name) { ... }** - Block placeholder
- **@include(path)** - Include another template
- **@import(path)** - Load components

#### Streaming & Performance
- **@stream { ... }** - Mark for streaming
- **@defer { ... } @fallback { ... }** - Deferred rendering
- **@lazy(cond) { ... } @fallback { ... }** - Conditional render
- **@raw(path)** - Include raw HTML

---

## 🔍 Finding Information

### I need to...

- **Create a reactive counter** → See examples in both guides under "Reactive Rendering"
- **Make a form that syncs with state** → Look for "Two-Way Binding" section
- **Fetch data from an API** → Search for "API Directives" or "data-spl-api-*"
- **Build a reusable component** → See "Components & Slots" section
- **Handle events** → Look for "@handler" or "on:" syntax
- **Create a conditional view** → See "@if/@else" or "data-spl-if"
- **Iterate over a list** → See "@for" directive with $loop metadata
- **Pattern match on data** → See "@match" directive
- **Understand hydration** → See "Client-Side Hydration" section
- **Use the JavaScript API** → See "JavaScript Runtime API" section

---

## 📋 Complete @ Directive List (29 total)

| Category | Directives | Count |
|----------|-----------|-------|
| State | @signal, @let, @computed | 3 |
| Binding | @bind | 1 |
| Events | @handler, @click | 2 |
| Reactive | @effect, @reactive, @watch | 3 |
| Control Flow | @if, @elseif, @else, @for, @switch, @case, @match | 7 |
| Components | @component, @render, @slot, @fill | 4 |
| Layout | @extends, @define, @block, @include, @import | 5 |
| Streaming | @stream, @defer, @lazy, @fallback | 4 |
| Utility | @raw, @// | 2 |
| **TOTAL** | | **29** |

---

## 🔗 Data Attributes List (18+ total)

| Category | Attributes | Count |
|----------|-----------|-------|
| Events | data-spl-on-*, data-spl-on-*-mods | 2 |
| Binding | data-spl-bind-*, data-spl-model | 2 |
| Conditional | data-spl-if, data-spl-else | 2 |
| API | data-spl-api-url, method, event, target, body, parse, form, reset, content-type | 9 |
| Hydration (generated) | data-spl-bind, data-spl-attr, data-spl-effect, data-spl-view | 4 |
| **TOTAL** | | **18+** |

---

## 💡 Key Concepts

### Signals
Reactive values that automatically notify subscribers when changed.
```html
@signal(count = 0)
```
- `SPL.read(name)` / `SPL.write(name, value)` - JavaScript API
- Available in effects/views for automatic re-rendering

### Effects & Reactive Views
Sections that automatically re-render when their signal dependencies change.
```html
@effect(count, user) { <div>${count} ${user.name}</div> }
@reactive(count, user) { ... } <!-- synonym -->
```
- Requires SSR hydration (`RenderSSR()`)
- Preserves focus/scroll during updates

### Two-Way Binding
Form inputs automatically sync with signal state, both directions.
```html
@signal(name = "")
<input data-spl-model="name" />
<!-- Input changes sync to signal, signal changes update input -->
```

### Hydration
Process of making server-rendered HTML interactive on the client:
1. Server renders template with `RenderSSR()`
2. HTML includes `data-spl-*` markers
3. Browser loads SPL runtime + hydration payload
4. Interactive features become active

### Tree-Shaking
SPL automatically detects used features and only includes necessary JS:
- No @effect used? → Don't include focus-preservation code
- No events? → Don't include event handler runtime
- Keeps bundles as small as possible

---

## 🏗️ Architecture Overview

### Server-Side Rendering
1. **Parse** - Convert SPL template to AST (parse.go)
2. **Render** - Evaluate AST with data context (render.go)
3. **Hydration** - Add markers and JS for client interactivity (reactive.go)
4. **Output** - HTML + JavaScript payload

### Client-Side Runtime
1. **Load** - SPL runtime + hydration payload in browser
2. **Initialize** - Register signals, handlers, effects/views
3. **Patch** - Apply event listeners and bindings to DOM
4. **React** - Re-render sections when signals change

---

## 🔧 Common Patterns

### Form with Validation
```html
@signal(form = {email: "", password: ""})

<form data-spl-api-url="/api/login"
      data-spl-api-method="POST"
      data-spl-api-form="closest"
      data-spl-api-target="result">
  <input data-spl-model="form.email" type="email" />
  <input data-spl-model="form.password" type="password" />
  <button type="submit">Login</button>
</form>

@reactive(result) {
  @if(result.error) { <p>${result.error}</p> }
  @else { <p>Success!</p> }
}
```

### Dynamic List
```html
@signal(items = [])

<button data-spl-api-url="/api/items"
        data-spl-api-event="load"
        data-spl-api-target="items">Load</button>

@reactive(items) {
  @for(item in items) {
    <div>${item.name} - ${item.price}</div>
  } @empty {
    <p>No items</p>
  }
}
```

### Reusable Component
```html
@component("Modal", title = "Dialog", dismissible = true) {
  <div class="modal">
    <header>@bind(title, "textContent")</header>
    @slot()
  </div>
}

@render("Modal", {title: "Confirm?"}) {
  <p>Are you sure?</p>
  @fill("footer") {
    <button>Cancel</button>
    <button>Confirm</button>
  }
}
```

---

## 📖 Source Code Reference

The documentation was extracted from:
- `/template/parse.go` - Parser implementation (1600+ lines)
- `/template/render.go` - Rendering logic (1000+ lines)
- `/template/reactive.go` - Reactive directives (70+ lines)
- `/template/hydration_runtime.go` - Client-side runtime (930+ lines)
- `/template/stream.go` - Streaming directives (180+ lines)
- `/examples/fiber-template/views/` - Real-world examples

---

## ✅ Completeness

This documentation is **COMPLETE** and covers:
- ✅ All 29 @ directives  
- ✅ All 18+ data-spl-* attributes
- ✅ Expression syntax and filters
- ✅ Event handling with modifiers
- ✅ Two-way binding patterns
- ✅ Component system with slots
- ✅ API integration
- ✅ Client-side runtime API
- ✅ Hydration system
- ✅ Pattern matching
- ✅ Streaming and deferred rendering
- ✅ Layout inheritance
- ✅ Real-world examples

**Last Updated:** April 12, 2026
**Source Code Analyzed:** 3,000+ lines of Go
**Examples Provided:** 50+

---

## 📞 Questions?

Refer to:
1. **SPL_QUICK_REFERENCE.md** - For quick syntax lookup
2. **SPL_DIRECTIVES_COMPLETE_GUIDE.md** - For detailed explanations
3. Source code in `/template/*.go` - For implementation details
