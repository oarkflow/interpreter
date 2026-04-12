# SPL Template Engine - Quick Reference Card

## All 29 @ Directives

### State Management (3)
- **@signal(name = init)** - Create reactive signal
- **@let(var = expr)** - Compute variable at render time
- **@computed(var = expr)** - Alias for @let

### Binding & Events (4)
- **@bind(signal, [attr])** - Bind signal to element attribute
- **@handler(name) { stmt }** - Register named event handler
- **@click(label, signal, action, [value])** - Create action button
- **@watch(expr) { body }** - Render when expression changes

### Control Flow (4)
- **@if(cond) { ... } @else { ... }** - Conditional rendering
- **@for(item in items) { ... }** - Loop iteration
- **@switch(expr) { @case(...) { ... } }** - Multi-way switch
- **@match(expr) { @case(pattern) { ... } }** - Pattern matching

### Reactive (2)
- **@effect(deps) { ... }** - Reactive render block (signal watcher)
- **@reactive(deps) { ... }** - Alias for @effect

### Components & Slots (4)
- **@component(name, props) { ... }** - Define component
- **@render(name, props) { ... }** - Invoke component
- **@slot([name])** - Content placeholder
- **@fill(name) { ... }** - Fill named slot

### Layout & Includes (5)
- **@extends(path)** - Set parent layout
- **@define(name) { ... }** - Define layout block
- **@block(name) { ... }** - Layout placeholder
- **@include(path, [data])** - Include another template
- **@import(path)** - Load component definitions

### Streaming & Lazy (4)
- **@stream { ... }** - Mark for streaming
- **@defer { ... } @fallback { ... }** - Deferred loading
- **@lazy(cond) { ... } @fallback { ... }** - Conditional rendering
- **@raw(path)** - Include raw HTML file

### Utility (1)
- **@// comment** - Single-line comment

---

## All data-spl-* HTML Attributes

### Events
- `data-spl-on-{event}="{expr}"` - Event binding
- `data-spl-on-{event}-mods="prevent,stop,capture,once,passive"` - Event modifiers

### Binding
- `data-spl-bind-{property}="{expr}"` - One-way binding
- `data-spl-model="{signal}"` - Two-way binding (including nested: `form.email`)

### Conditional
- `data-spl-if="{signal}"` - Show if signal truthy
- `data-spl-else="{signal}"` - Show if signal falsy

### API Calls
- `data-spl-api-url="{url}"` - Fetch endpoint (required)
- `data-spl-api-method="GET|POST|PUT|DELETE"` - HTTP method
- `data-spl-api-event="click|load|submit"` - Trigger event
- `data-spl-api-target="{signal}"` - Store response in signal
- `data-spl-api-body="{json}"` - Request body template
- `data-spl-api-parse="json|text|html|auto"` - Response parsing
- `data-spl-api-form="closest"` - Serialize form
- `data-spl-api-reset="{signal1},{signal2}"` - Clear signals on success
- `data-spl-api-content-type="application/json"` - Content-Type header

### Hydration (generated)
- `data-spl-bind="{signal}"` - Two-way binding wrapper
- `data-spl-attr="{attr}"` - Attribute being bound
- `data-spl-effect="{n}"` - Effect block marker
- `data-spl-view="{n}"` - Reactive view marker

---

## Event Syntax in HTML

### With on: Attribute
```html
<button on:click="counter += 1">Add</button>
<form on:submit.prevent="handleSubmit">...</form>
<input on:input.capture="handleInput" />
<input on:focus.once="logFocus" />
<img on:load.passive="onImageLoad" />
```

### With data-spl-on-
```html
<button data-spl-on-click="counter += 1">Add</button>
<form data-spl-on-submit="handleSubmit" data-spl-on-submit-mods="prevent">...</form>
```

### Bind: Attribute
```html
<input bind:value="userName" />
<div bind:textContent="message" />
<img bind:src="imageUrl" />
<input type="checkbox" bind:checked="isActive" />
```

---

## Expression Syntax ${}

```javascript
${variableName}                    // Variable
${obj.prop}                        // Property access
${array[0]}                        // Array index
${func(arg1, arg2)}                // Function call
${expr1 + expr2}                   // Operations
${signal('name')}                  // Read signal (in expressions)
${condition ? trueVal : falseVal}  // Ternary

${raw htmlContent}                 // No escaping
${text | uppercase}                // With filter
${text | lowercase | capitalize}   // Multiple filters
```

---

## JavaScript Runtime API (window.SPL)

### Reading/Writing Signals
```javascript
SPL.read(name)                     // Get value
SPL.write(name, value)             // Set value
SPL.subscribe(name, callback)      // Watch for changes
SPL.signalRef(name)                // Get reference object
SPL.signalName(ref)                // Get name from ref
```

### In Event Handlers (scope helpers)
```javascript
SPL.signal(name)                   // Read (shorthand)
SPL.setSignal(name, value)         // Write (shorthand)
SPL.toggle(nameOrRef)              // Toggle boolean

// Available scope: event, element, Math, Date, JSON, etc.
```

### Evaluation
```javascript
SPL.evalExpression(expr, event, element)  // Evaluate expression
SPL.executeEvent(expr, event, element)    // Execute event code
SPL.interpolate(source)                   // Replace __SPL_SIGNAL__name__ markers
```

### DOM Patching
```javascript
SPL.patch(root)                    // Apply hydration to DOM
SPL.captureFocus(root)             // Save focus state
SPL.restoreFocus(root, snapshot)   // Restore focus state
```

### Nested Paths
```javascript
SPL.readPath("signal.key.nested")  // Read nested value
SPL.writePath("signal.key.nested", value)  // Write nested
```

### Debug
```javascript
SPL.debug.enabled = true
SPL.getRenderStats()               // {totalRenders, views, effects, signals}
```

---

## Handler Example Scopes

### Available in @handler and on: expressions:

```javascript
// Scope objects & functions:
event                      // DOM Event
element                    // Target element
signal(name)               // Read signal value
setSignal(name, value)     // Write signal
toggle(nameOrRef)          // Toggle boolean

// Built-in globals:
Math, Number, String, Boolean, Date, JSON, console, document, window

// Can also reference other handlers:
@handler(foo) { ... }
@handler(bar) {
  foo(event, element)      // Call another handler
}
```

---

## Two-Way Binding Examples

### With @bind
```html
<input value="@bind(userName)" />
<div>@bind(message, "textContent")</div>
```

### With data-spl-model
```html
<input type="text" data-spl-model="firstName" />
<input type="number" data-spl-model="age" />
<input type="checkbox" data-spl-model="agreed" />
<textarea data-spl-model="bio"></textarea>
<select data-spl-model="country">...</select>
```

### Nested Paths
```html
<input data-spl-model="user.firstName" />
<input data-spl-model="form.personal.email" />
<input data-spl-model="settings.theme.darkMode" />
```

---

## API Call Examples

### Simple GET
```html
<button data-spl-api-url="/api/data" data-spl-api-target="result">
  Load Data
</button>
```

### Load on Page Load
```html
<div data-spl-api-url="/api/config"
     data-spl-api-event="load"
     data-spl-api-target="config"></div>
```

### POST with Form
```html
<form data-spl-api-url="/api/submit"
      data-spl-api-method="POST"
      data-spl-api-form="closest"
      data-spl-api-target="result">
  <input name="email" />
  <input name="message" />
  <button type="submit">Send</button>
</form>
```

### POST with Custom Body
```html
<button data-spl-api-url="/api/notify"
        data-spl-api-method="POST"
        data-spl-api-body='{"user": "${userId}", "msg": "${message}"}'
        data-spl-api-target="notification">
  Send Notification
</button>
```

### Multiple Response Types
```html
<!-- Parse as JSON -->
<button data-spl-api-url="/api/data"
        data-spl-api-parse="json"
        data-spl-api-target="data"></button>

<!-- Parse as HTML -->
<button data-spl-api-url="/api/fragment"
        data-spl-api-parse="html"
        data-spl-api-target="content"></button>

<!-- Auto-detect -->
<button data-spl-api-url="/api/smart"
        data-spl-api-parse="auto"
        data-spl-api-target="result"></button>
```

---

## Component Examples

### Simple Component with Slot
```html
@component("Card", title = "Default") {
  <div class="card">
    <h2>${title}</h2>
    @slot()
  </div>
}

@render("Card", {title: "My Card"}) {
  <p>Card content</p>
}
```

### Component with Named Slots
```html
@component("Layout") {
  <div class="layout">
    @slot("header")
    <main>@slot()</main>
    @slot("footer")
  </div>
}

@render("Layout") {
  <h1>Page Title</h1>
  
  @fill("header") {
    <nav>Navigation</nav>
  }
  
  @fill("footer") {
    <p>© 2024</p>
  }
}
```

---

## Loop Variable Reference

```html
@for(item in items) {
  ${$loop.index}      <!-- 0, 1, 2, ... -->
  ${$loop.index1}     <!-- 1, 2, 3, ... -->
  ${$loop.first}      <!-- true, false, false, ... -->
  ${$loop.last}       <!-- false, false, true -->
  ${$loop.length}     <!-- Total number of items -->
}
```

---

## React on Server-Side Only

```html
@let(total = items.length)
@computed(average = sum / count)
@watch(status) {
  <p>Status: ${status}</p>
}
@lazy(isAdmin) {
  <div>Admin tools</div>
} @fallback {
  <p>Not authorized</p>
}
```

---

## React on Client-Side (with Hydration)

```html
@signal(count = 0)

@effect(count) {
  <p>Count is: ${count}</p>
}

@reactive(count, filter) {
  @for(item in items) {
    @if(matchesFilter) { ... }
  }
}

<button on:click="count += 1">Add</button>
<input data-spl-model="name" />
```

---

## Notes

- **Server-side**: `@let`, `@computed`, `@watch`, `@lazy` run at render time
- **Client-side**: `@effect`, `@reactive`, signals, handlers, `data-spl-*` attributes require hydration
- **Both**: `@signal`, `@handler` work both server and client (serialized for hydration)
- **Hydration**: Call `RenderSSR()` instead of `Render()` to enable client-side reactivity
- **Streaming**: `@stream` and `@defer` require special rendering mode
- **Components**: Can be reused, support props/defaults, slots, and fills
- **Patterns**: Use `@match` for complex pattern matching, `@switch` for simple value checks

