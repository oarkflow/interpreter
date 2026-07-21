# 09 — Classes & Interfaces

Source: `pkg/parser/parser.go` (`parseClassStatement`, `parseInterfaceStatement`),
`pkg/object/object.go` (`ClassObject`, `ClassInstance`, `InterfaceLiteral`).

SPL classes are a lightweight, single-inheritance OOP layer on top of the
same closure/hash machinery as everything else. A `ClassInstance`'s runtime
`Type()` reports as `HASH_OBJ`, so instances interoperate naturally with
hash-oriented builtins.

## Basic class

```spl
class User {
    init(name) {
        this.name = name;
    }
    greet() {
        return "Hello " + this.name;
    }
}

let u = User("SPL");   // classes are directly callable, no `new` required
print u.greet();       // Hello SPL
```

`init(...)` is the constructor, invoked automatically when the class is
called. `this` refers to the current instance inside methods.

## `new` is optional sugar

```spl
let u2 = new User("SPL"); // identical to User("SPL")
```

`new Ctor(...)` simply evaluates `Ctor(...)` as a call — SPL classes don't
require `new` at all.

## Inheritance (`extends`) and `super`

```spl
abstract class Shape {
    abstract area();
    describe() { return "area=" + this.area(); }
}
class Circle extends Shape {
    init(radius) { this.radius = radius; }
    area() { return 3.14159 * this.radius * this.radius; }
}
print Circle(2).describe(); // area=12.56636
```

```spl
class Person {
    init(name) { this.name = name; }
    greet() { return "Hi, " + this.name; }
}
class Employee extends Person {
    init(name, role) {
        super(name);       // calls the parent constructor
        this.role = role;
    }
}
let e = Employee("Sam", "Engineer");
print e.greet(); // Hi, Sam
print e.role;    // Engineer
```

`super(...)` calls the parent's `init`; `super.method()` calls a parent
method explicitly (e.g. to extend rather than replace behavior).

## `abstract` classes and methods

```spl
abstract class Shape {
    abstract area();     // no body — subclasses must implement it
    describe() { return "area=" + this.area(); }
}
```

An `abstract` class cannot be instantiated directly; an `abstract` method
declares a required override without providing an implementation.

## Interfaces (runtime metadata)

```spl
interface Greetable {
    greet();
}
class Person implements Greetable {
    init(name) { this.name = name; }
    greet() { return "Hi, " + this.name; }
}
```

Interfaces are **not enforced** by the runtime — `implements` records
metadata (useful for tooling/documentation and reflection) but a class that
omits a required method is not statically or dynamically rejected.

## `private` fields

```spl
class BankAccount {
    private balance = 0;
    init(initial) { this.balance = initial; }
    deposit(amount) {
        this.balance += amount;
        return this.balance;
    }
}
let account = BankAccount(100);
print account.deposit(50); // 150
```

## `static` members

```spl
class InstanceCounter {
    static total = 0;
    init() { InstanceCounter.total += 1; }
    static getTotal() { return InstanceCounter.total; }
}
InstanceCounter();
InstanceCounter();
print InstanceCounter.getTotal(); // 2
```

Static fields/methods are attached to the class itself and shared across all
instances, accessed via `ClassName.member`.

## Instances as hashes

Because `ClassInstance` reports type `HASH_OBJ`, instance fields are
readable/writable via ordinary dot access (`this.name`, `account.balance`
if not private), and instances can be passed anywhere a hash-like value is
expected.
