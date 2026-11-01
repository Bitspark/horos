# Descriptor examples

These are readable shorthand for the ontos compounds in the
[specification](../spec/horos.md), not a separate text parser.

```text
record closed
  field "name" req (scalar utf8-text)
  field "age"  opt (scalar int)
```

`map { text("name"): text("Ada") }` conforms. A missing name, an integer name,
or an undeclared extra field does not. A name plus integer age conforms.

```text
union (compound "ok" (scalar utf8-text))
      (compound "error" (scalar int))
```

This describes a labelled sum. `Tuple(Atom("ok"), text("ready"))` conforms.
A bare tuple and an ontos/data list are distinct representations.

```text
rec "tree"
  union (scalar int)
        (compound "branch" (ref "tree") (ref "tree"))
```

This describes a finite binary tree with integer leaves. Each recursive reference
is beneath a compound child and therefore guarded. `rec "x" (ref "x")` is not
well formed.

The null singleton is `exact(null())`. An optional nullable text field combines
presence and membership explicitly: `field "next" opt (union (scalar utf8-text)
(exact null()))`. This is not a generic pagination declaration; generic binders
and applications are outside the current descriptor vocabulary.
