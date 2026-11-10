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

## Where a value fails

For the record above (`name` at item 2, `age` at item 3), `diagnose` on
`map { text("age"): text("x"), text("name"): text("Ada") }` reports:

```json
{"verdict":"mismatch","location":{"path":[{"kind":"field","name":"age"}],"at":[3,3],"reason":"refused"}}
```

The age field's value was refused by the descriptor at item 3 of the field form
at item 3 of the record. Without the name, the record itself reports
`missing-field` with `"field":"name"`, before any present field is checked.

For the binary tree, take a branch whose left child is a branch with a text left
leaf. At the root and the inner branch, the compound alternative gets further
into the value than `scalar int`, so each union passes on its location. At the
leaf neither alternative descends, so that union reports itself: the path is two
`item` steps of index 1 and `at` is `[2]`, `no-alternative`. Each `ref`
continues at its `rec` body, so every level reports the same position.
