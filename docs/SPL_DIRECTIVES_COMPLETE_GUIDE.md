# SPL Template Engine - Complete Directive & Feature Reference

## Overview
SPL is a reactive template engine with server-side rendering (SSR) and client-side hydration. This guide covers ALL directives, features, and data attributes available.

---

# 1. TEMPLATE DIRECTIVES (@-syntax)

## Core Structural Directives

### @extends
**Syntax:** `@extends("path/to/layout.html")`
**Description:** Specifies the parent layout file for template inheritance. Must be the first directive.
**Example:**
```
@extends("layouts/main.html")
```

### @define
**Syntax:** `@define("blockName") { ... }`
**Description:** Defines a named block of content that will replace `@block` placeholders in the parent layout.
**Example:**
```
@define("content") {
  <h1>Page content here</h1>
}
```

### @block
**Syntax:** `@block("blockName") { defaultContent }`
**Description:** Declares a placeholder in a layout that can be overridden by `@define` blocks.
**Example:**
```
@block("content") {
  <p>Default content if not overridden</p>
}
```

### @include
**Syntax:** `@include("path/to/file.html")` or `@include("path/to/file.html", dataExpr)`
**Description:** Includes another template file. Optional second parameter passes data to the included template.
**Example:**
```
@include("components/header.html", {title: "My Page"})
```

### @import
**Syntax:** `@import("path/to/components.html")`
**Description:** Loads component definitions from another file for use with `@render`.

---

## Reactive State & Binding Directives

### @signal
**Syntax:** `@signal(signalName = initialValue)`
**Description:** Creates a reactive signal that can be read/written by handlers and effects. Initial value is a SPL expression.
**Example:**
```
@signal(count = 0)
@signal(user = {"name": "John", "age": 30})
@signal(items = [1, 2, 3])
```

### @bind
**Syntax:** `@bind(signalName)` or `@bind(signalName, "attribute")`
**Description:** Binds a signal value to an element attribute. Default attribute is `textContent`. Hydrates as two-way binding.
**Attributes:** `textContent`, `value`, `checked`, `innerHTML`, `html`, or any element property
**Example:**
```
<div>@bind(count)</div>
<div class="counter">@bind(count, "textContent")</div>
<span>@bind(user.name, "innerHTML")</span>
```

### @let
**Syntax:** `@let(variableName = expression)`
**Description:** Assigns a computed value to a variable at render time (server-side only).
**Example:**
```
@let(fullName = firstName + " " + lastName)
<p>${fullName}</p>
```

### @computed
**Syntax:** `@computed(variableName = expression)`
**Description:** Same as `@let` - creates a derived value (separate directive for semantic clarity).
**Example:**
```
@computed(total = price * quantity)
<p>Total: ${total}</p>
```

### @watch
**Syntax:** `@watch(expression) { body }`
**Description:** Renders body only when watched expression value changes (server-side only).
**Example:**
```
@watch(count) {
  <p>Count changed to ${count}</p>
}
```

---

## Control Flow Directives

### @if / @elseif / @else
**Syntax:**
```
@if(condition) {
  ...
} @elseif(otherCondition) {
  ...
} @else {
  ...
}
```
**Description:** Conditional rendering. If hydrated and condition is a simple signal name, becomes reactive.
**Example:**
```
@if(count > 0) {
  <p>Count is positive</p>
} @else {
  <p>Count is zero or negative</p>
}
```

### @for
**Syntax:** `@for(item in iterable) { ... }` or `@for(key, value in iterable) { ... }`
**Description:** Iterates over arrays and objects. Loop variable is available in body.
**Loop Metadata:** `$loop.index`, `$loop.index1`, `$loop.first`, `$loop.last`, `$loop.length`
**Example:**
```
@for(item in items) {
  <div>${item.name} (index: ${$loop.index})</div>
}

@for(key, value in hash) {
  <p>${key}: ${value}</p>
}
```

### @switch / @case / @default
**Syntax:**
```
@switch(expression) {
  @case(value1, value2) { ... }
  @case(value3) { ... }
  @default { ... }
}
```
**Description:** Multi-way branching. First matching case executes.
**Example:**
```
@switch(status) {
  @case("pending") { <span>Waiting...</span> }
  @case("complete") { <span>Done!</span> }
  @default { <span>Unknown</span> }
}
```

### @match / @case / @default
**Syntax:**
```
@match(expression) {
  @case(pattern if guard?) { ... }
  @default { ... }
}
```
**Description:** Pattern matching with optional guards. Supports destructuring (arrays, objects, etc).
**Example:**
```
@match(value) {
  @case([a, b]) { <p>Pair: ${a}, ${b}</p> }
  @case(x: integer if x > 0) { <p>Positive int: ${x}</p> }
  @default { <p>Other</p> }
}
```

---

## Component & Slot Directives

### @component
**Syntax:** 
```
@component("ComponentName") { body }
@component("ComponentName", prop1, prop2 as internalName = "default") { body }
```
**Description:** Defines a reusable component with optional props and defaults.
**Available in body:** `props` object, `children` string, slot fills via `@slot`
**Example:**
```
@component("Card", title = "Default Title") {
  <div class="card">
    <h3>${title}</h3>
    @slot()
  </div>
}
```

### @render
**Syntax:** `@render("ComponentName")` or `@render("ComponentName", propsExpr) { ... }`
**Description:** Invokes a component. Props can be inline hash expression. Body becomes `children`.
**Example:**
```
@render("Card", {title: "My Card"}) {
  <p>Card content here</p>
  @fill("footer") {
    <button>Close</button>
  }
}
```

### @slot
**Syntax:** `@slot()` or `@slot("slotName")`
**Description:** Placeholder inside component for injected content. Default slot receives `children`.
**Example:**
```
@slot()
@slot("header")
@slot("footer")
```

### @fill
**Syntax:** `@fill("slotName") { content }`
**Description:** Provides content for a named slot when rendering a component.
**Example:**
```
@render("Card") {
  <p>Default slot content</p>
  @fill("header") { <h1>Header</h1> }
  @fill("footer") { <p>Footer</p> }
}
```

---

## Event & Handler Directives

### @handler
**Syntax:** `@handler(handlerName) { statement }` or `@handler(handlerName) = expression`
**Description:** Registers a named event handler available in `on:*` event attributes. Serialized for client-side hydration.
**Scope:** `event`, `element`, `signal(name)`, `setSignal(name, value)`, `toggle(name)`, etc.
**Example:**
```
@handler(togglePanel) {
  toggle(panelOpen)
}

@handler(incrementCounter) = counter += 1
```

### @click
**Syntax:** `@click("label", signalName, action, value?)`
**Description:** Quick shorthand to create a button with signal action. Actions: `"toggle"`, `"inc"`, `"set"`.
**Example:**
```
@click("Add", count, "inc", "1")
@click("Toggle", open, "toggle")
@click("Set to 5", count, "set", "5")
```

---

## Reactive Rendering Directives

### @effect
**Syntax:** `@effect(dep1, dep2, ...) { body }`
**Description:** Server-side: renders body once. When hydrated to client, re-renders body whenever any dependency signal changes. Preserves focus/scroll position.
**Example:**
```
@effect(count, user) {
  <div>Count: ${count}, User: ${user.name}</div>
}
```

### @reactive
**Syntax:** `@reactive(dep1, dep2, ...) { body }`
**Description:** Synonym for `@effect` - makes a template section reactive on the client. Re-renders when dependencies change.
**Example:**
```
@reactive(searchQuery, filters) {
  @for(result in searchResults) {
    <div>${result}</div>
  }
}
```

### @stream
**Syntax:** `@stream { content }`
**Description:** Marks content for streaming. Flushes to client immediately.
**Example:**
```
@stream {
  <p>This renders immediately</p>
}
```

### @defer
**Syntax:** 
```
@defer { content } @fallback { placeholder }
```
**Description:** Defers rendering of content until after main page loads. Fallback shown initially.
**Example:**
```
@defer {
  <div>Expensive computation...</div>
} @fallback {
  <p>Loading...</p>
}
```

### @lazy
**Syntax:** 
```
@lazy(condition) { content } @fallback { placeholder }
```
**Description:** Conditionally renders content. If condition is falsy, shows fallback. Server-side only.
**Example:**
```
@lazy(isAdmin) {
  <div>Admin panel</div>
} @fallback {
  <p>Access denied</p>
}
```

---

## Utility Directives

### @raw
**Syntax:** `@raw("path/to/file.html")`
**Description:** Includes raw HTML file without parsing or escaping.
**Example:**
```
@raw("static/banner.html")
```

### @comment / @//
**Syntax:** `@// comment text`
**Description:** Single-line comment (consumed during parsing, not rendered).

---

# 2. EXPRESSION SYNTAX ${}

## Basic Interpolation
```
${variableName}
${expression + " string"}
${functionCall()}
${object.property.nested}
${array[0]}
```

## Filters
```
${text | uppercase}
${text | lowercase}
${text | capitalize}
${items | length}
```

## Raw Output (no HTML escaping)
```
${raw htmlContent}
```

---

# 3. HTML ATTRIBUTE BINDINGS (data-spl-*)

## Event Binding
**Syntax:** `on:eventName="expression"` (auto-converted to `data-spl-on-*`)
**Event Modifiers:** `.prevent`, `.stop`, `.capture`, `.once`, `.passive`

```html
<button on:click="counter += 1">Add</button>
<form on:submit.prevent="handleSubmit">...</form>
<input on:input.capture="handleInput" />
```

**Compiled to:**
```html
<button data-spl-on-click="counter += 1">Add</button>
<form data-spl-on-submit="handleSubmit" data-spl-on-submit-mods="prevent">...</form>
```

---

## Binding Attributes
**Syntax:** `bind:property="signalName"` (auto-converted to `data-spl-bind-property`)

```html
<input bind:value="userName" />
<div bind:textContent="message" />
<img bind:src="imageUrl" />
<input type="checkbox" bind:checked="isActive" />
```

---

## Model Binding (two-way)
**Syntax:** `data-spl-model="signalName"` or `data-spl-model="signal.path.nested"`

```html
<input type="text" data-spl-model="firstName" />
<input type="number" data-spl-model="personal.age" />
<textarea data-spl-model="bio"></textarea>
<input type="checkbox" data-spl-model="agreedToTerms" />
<select data-spl-model="country">...</select>
```

---

## Conditional Rendering
**Syntax:** `data-spl-if="signalName"` / `data-spl-else="signalName"`

```html
<div data-spl-if="isVisible">Visible</div>
<div data-spl-else="isVisible" style="display: none;">Hidden</div>
```

---

# 4. API DIRECTIVES (data-spl-api-*)

Enable fetch requests directly from HTML elements.

### Basic API Call
```html
<button data-spl-api-url="/api/todos" data-spl-api-method="GET" data-spl-api-target="todoList">
  Load Todos
</button>
```

### Complete API Attributes

| Attribute | Description | Example |
|-----------|-------------|---------|
| `data-spl-api-url` | API endpoint (required) | `/api/data` |
| `data-spl-api-method` | HTTP method (default: GET) | `GET`, `POST`, `PUT`, `DELETE` |
| `data-spl-api-event` | Trigger event (default: click) | `click`, `load`, `submit` |
| `data-spl-api-target` | Signal to store response | `apiResponse` |
| `data-spl-api-parse` | Response type (default: auto) | `json`, `text`, `html`, `auto` |
| `data-spl-api-body` | Template for request body | `{"name": "${name}"}` |
| `data-spl-api-content-type` | Content-Type header | `application/json` |
| `data-spl-api-form` | Serialize form data | `closest` |
| `data-spl-api-reset` | Signals to reset on success | `loading, error` |

### Examples

**Load on page load:**
```html
<div data-spl-api-url="/api/config" data-spl-api-event="load" data-spl-api-target="config"></div>
```

**POST with form data:**
```html
<form data-spl-api-url="/api/submit" data-spl-api-method="POST" data-spl-api-form="closest" data-spl-api-target="result">
  <input name="email" />
  <button type="submit">Submit</button>
</form>
```

**POST with custom body template:**
```html
<button 
  data-spl-api-url="/api/notify"
  data-spl-api-method="POST"
  data-spl-api-body='{"message": "${userMessage}", "priority": "high"}'
  data-spl-api-target="notification">
  Send
</button>
```

---

# 5. CLIENT-SIDE HYDRATION

When using `RenderSSR()`, generated HTML includes:

1. **Runtime Script** - SPL JavaScript runtime
2. **Hydration Markers** - `data-spl-*` attributes
3. **Payload Script** - Signals, handlers, effects/views

### Generated Markers

```html
<!-- Signals registered for hydration -->
<div data-spl-bind="count">0</div>

<!-- Effects (re-render on signal change) -->
<div data-spl-effect="1">Content</div>

<!-- Views (reactive template sections) -->
<div data-spl-view="1">Content</div>

<!-- Event handlers -->
<button data-spl-on-click="increment">+</button>

<!-- Two-way model binding -->
<input data-spl-model="name" />

<!-- Conditionals -->
<div data-spl-if="condition">Shown if true</div>
<div data-spl-else="condition">Shown if false</div>
```

---

# 6. JAVASCRIPT RUNTIME API (Client-side)

Available on `window.SPL` after hydration:

## Core Functions
```javascript
SPL.read(signalName)                    // Get signal value
SPL.write(signalName, value)            // Set signal value
SPL.subscribe(signalName, callback)     // Listen for changes

SPL.signal(name)                        // Read signal (shorthand in handlers)
SPL.setSignal(nameOrRef, value)         // Write signal (shorthand in handlers)
SPL.toggle(nameOrRef)                   // Toggle boolean signal

SPL.registerHandler(name, function)     // Register event handler
SPL.executeEvent(expr, event, element)  // Execute event expression

SPL.signalRef(name)                     // Get signal reference object
SPL.signalName(ref)                     // Get signal name from reference
```

## Helper Functions
```javascript
SPL.evalExpression(expr, event, element)    // Evaluate expression
SPL.interpolate(source)                    // Interpolate signal placeholders
SPL.readPath(path)                         // Read nested path (signal.key.nested)
SPL.writePath(path, value)                 // Write nested path

SPL.patch(root)                            // Apply hydration patches to DOM
```

## Debug API
```javascript
SPL.debug.enabled = true                    // Enable debug logging
SPL.getRenderStats()                       // Get render statistics
```

---

# 7. FEATURE FLAGS & TREE-SHAKING

SPL automatically detects used features and only includes necessary runtime code:

| Feature | Triggered By | Included Runtime |
|---------|-------------|------------------|
| Bindings | `data-spl-bind-*` | `patchBindings` |
| Models | `data-spl-model` | `patchModels` |
| Events | `data-spl-on-*` | `patchEvents` |
| API | `data-spl-api-*` | `patchAPI` |
| Conditionals | `data-spl-if`/`else` | `patchConditionals` |
| Focus | Effects/Views exist | `captureFocus`, `restoreFocus` |

---

# 8. COMPLETE EXAMPLE

```html
@extends("layouts/app.html")

@define("content") {
  @signal(count = 0)
  @signal(items = [])
  @signal(filter = "all")
  
  @handler(addItem) {
    var list = signal('items');
    list.push({id: Date.now(), text: setSignal('input', ''), done: false});
    setSignal('items', list);
  }
  
  @handler(toggleItem) {
    var itemId = Number(event.currentTarget.getAttribute('data-id'));
    var list = signal('items');
    for(var i = 0; i < list.length; i++) {
      if(list[i].id === itemId) list[i].done = !list[i].done;
    }
    setSignal('items', list);
  }
  
  <div class="app">
    <h1>Todo List</h1>
    
    <!-- Input with two-way binding -->
    <input type="text" data-spl-model="input" placeholder="Add a new item" />
    <button on:click="addItem">Add</button>
    
    <!-- API call example -->
    <button 
      data-spl-api-url="/api/todos"
      data-spl-api-method="GET"
      data-spl-api-target="items"
      data-spl-api-event="load">
      Load from Server
    </button>
    
    <!-- Reactive list -->
    @reactive(items, filter) {
      @for(item in items) {
        <div class="todo" data-id="${item.id}">
          <input type="checkbox" data-spl-model="done" on:click="toggleItem" />
          <span>${item.text}</span>
        </div>
      } @empty {
        <p>No items yet</p>
      }
    }
    
    <!-- Stats section -->
    @effect(items) {
      <p>Total: ${items.length}</p>
    }
  </div>
}
```

---

# SUMMARY OF ALL @ DIRECTIVES

| Directive | Type | Syntax | Purpose |
|-----------|------|--------|---------|
| @signal | State | `@signal(name = init)` | Create reactive signal |
| @bind | Binding | `@bind(signal)` | Bind signal to element |
| @let | Variable | `@let(var = expr)` | Assign computed variable |
| @computed | Variable | `@computed(var = expr)` | Synonym for @let |
| @watch | Reactive | `@watch(expr) {...}` | Re-render on value change |
| @if/@else | Control | `@if(cond) {...} @else {...}` | Conditional rendering |
| @for | Loop | `@for(x in xs) {...}` | Iterate array/object |
| @switch/@case | Branch | `@switch(x) {@case(y) {...}}` | Multi-way switch |
| @match/@case | Pattern | `@match(x) {@case(p) {...}}` | Pattern matching |
| @effect | Reactive | `@effect(deps) {...}` | Reactive rendering block |
| @reactive | Reactive | `@reactive(deps) {...}` | Synonym for @effect |
| @component | Reuse | `@component("Name") {...}` | Define reusable component |
| @render | Reuse | `@render("Name") {...}` | Invoke component |
| @slot | Slot | `@slot()` | Content placeholder |
| @fill | Slot | `@fill("name") {...}` | Fill named slot |
| @handler | Event | `@handler(name) {...}` | Register event handler |
| @click | Action | `@click(label, sig, action)` | Quick action button |
| @include | Template | `@include("path")` | Include another file |
| @import | Template | `@import("path")` | Load components |
| @extends | Layout | `@extends("path")` | Specify parent layout |
| @define | Block | `@define("name") {...}` | Define layout block |
| @block | Block | `@block("name") {...}` | Layout placeholder |
| @raw | Content | `@raw("path")` | Include raw HTML |
| @stream | Stream | `@stream {...}` | Mark for streaming |
| @defer | Lazy | `@defer {...} @fallback {...}` | Deferred rendering |
| @lazy | Lazy | `@lazy(cond) {...} @fallback {...}` | Conditional rendering |
| @// | Comment | `@// text` | Comment (not rendered) |

