import { test } from "node:test";
import assert from "node:assert/strict";
import { atom, tuple, type Value } from "@bitspark/ontos-core";
import {
  DataError,
  encodeBool,
  encodeInt,
  encodeMap,
  encodeNull,
  encodeText,
} from "@bitspark/ontos-data";
import { conforms, recordForm, wf, type RecordField } from "../src/index.js";
import { diagnose } from "../src/diagnose.js";

const word = (s: string): Value => atom(new TextEncoder().encode(s));
const form = (name: string, ...args: Value[]): Value =>
  tuple([word(`horos/${name}`), ...args]);
const text = form("scalar", word("utf8-text"));
const int = form("scalar", word("int"));
const map = (...entries: [string, Value][]): Value =>
  encodeMap(entries.map(([key, value]) => [encodeText(key), value]));

test("recordForm constructs the specified descriptor bytes without reordering fields", () => {
  const got = recordForm("closed", [
    { name: "z", presence: "required", form: text },
    { name: "a", presence: "optional", form: int },
  ]);
  const expected = tuple([
    word("horos/record"),
    word("closed"),
    tuple([word("horos/field"), encodeText("z"), word("req"), text]),
    tuple([word("horos/field"), encodeText("a"), word("opt"), int]),
  ]);
  assert.ok(got.equals(expected));
  assert.deepEqual(diagnose(map(), got), {
    verdict: "mismatch",
    location: { path: [], at: [], reason: "missing-field", field: "z" },
  });
});

test("required, optional, absent and null preserve existing record judgments", () => {
  const contact = recordForm("closed", [
    { name: "name", presence: "required", form: text },
    { name: "age", presence: "optional", form: int },
  ]);
  assert.equal(wf(contact), true);
  assert.equal(conforms(map(["name", encodeText("Ada")]), contact), true);
  assert.equal(
    conforms(map(["name", encodeText("Ada")], ["age", encodeInt(0n)]), contact),
    true,
  );
  assert.equal(conforms(map(), contact), false);
  assert.equal(
    conforms(map(["name", encodeText("Ada")], ["age", encodeNull()]), contact),
    false,
  );
  assert.equal(conforms(map(["name", encodeNull()]), contact), false);
});

test("open and closed modes retain canonical-map and text-key requirements", () => {
  const open = recordForm("open", []);
  const closed = recordForm("closed", []);
  assert.equal(conforms(map(), open), true);
  assert.equal(conforms(map(), closed), true);
  assert.equal(conforms(map(["extra", tuple([atom([0, 255])])]), open), true);
  assert.equal(conforms(map(["extra", encodeBool(false)]), closed), false);
  assert.equal(conforms(encodeMap([[encodeInt(1n), encodeText("x")]]), open), false);
  const unsorted = tuple([
    word("map"),
    tuple([encodeText("z"), encodeInt(1n)]),
    tuple([encodeText("a"), encodeInt(2n)]),
  ]);
  assert.equal(conforms(unsorted, open), false);
});

test("independent nested record and exact opaque payload forms compose", () => {
  const payload = tuple([atom([0xff]), tuple([])]);
  const sample = recordForm("closed", [
    {
      name: "position",
      presence: "required",
      form: recordForm("closed", [
        { name: "x", presence: "required", form: int },
        { name: "y", presence: "required", form: int },
      ]),
    },
    { name: "payload", presence: "required", form: form("exact", payload) },
  ]);
  assert.equal(wf(sample), true);
  assert.equal(
    conforms(
      map(
        ["position", map(["x", encodeInt(-4n)], ["y", encodeInt(2n)])],
        ["payload", payload],
      ),
      sample,
    ),
    true,
  );
  assert.equal(
    conforms(map(["position", map(["x", encodeInt(-4n)])], ["payload", payload]), sample),
    false,
  );
  assert.equal(
    conforms(
      map(
        ["position", map(["x", encodeInt(-4n)], ["y", encodeInt(2n)])],
        ["payload", encodeText("opaque")],
      ),
      sample,
    ),
    false,
  );
});

test("a child ref is left to its enclosing lexical recursion binder", () => {
  const record = recordForm("closed", [
    { name: "next", presence: "optional", form: form("ref", word("Node")) },
  ]);
  assert.equal(wf(record), false);
  const recursive = form("rec", word("Node"), record);
  assert.equal(wf(recursive), true);
  assert.equal(conforms(map(["next", map(["next", map()])]), recursive), true);
  assert.equal(conforms(map(["next", encodeNull()]), recursive), false);
  const malformed = recordForm("closed", [
    { name: "x", presence: "required", form: atom([]) },
  ]);
  assert.equal(wf(malformed), false);
});

test("names use exact Unicode scalar strings and have no special object-property names", () => {
  const names = ["", "__proto__", "constructor", "é", "e\u0301", "\ufeff", "😀"];
  const record = recordForm(
    "closed",
    names.map((name) => ({ name, presence: "required", form: text })),
  );
  assert.equal(wf(record), true);
  assert.equal(
    conforms(
      map(...names.map((name) => [name, encodeText(name)] as [string, Value])),
      record,
    ),
    true,
  );
  assert.throws(
    () => recordForm("closed", [{ name: "\ud800", presence: "required", form: text }]),
    DataError,
  );
});

test("native construction refuses duplicate and invalid inputs without coercion", () => {
  assert.throws(
    () =>
      recordForm("closed", [
        { name: "x", presence: "required", form: text },
        { name: "x", presence: "optional", form: int },
      ]),
    TypeError,
  );
  assert.throws(() => recordForm("strict" as "closed", []), TypeError);
  assert.throws(() => recordForm("closed", {} as readonly RecordField[]), TypeError);
  for (const field of [
    null,
    { name: 1, presence: "required", form: text },
    { name: "x", presence: true, form: text },
    { name: "x", presence: "required", form: "text" },
  ]) {
    assert.throws(
      () => recordForm("closed", [field as unknown as RecordField]),
      TypeError,
    );
  }
});

test("the descriptor owns its field sequence after construction", () => {
  const fields = [{ name: "x", presence: "required" as const, form: int }];
  const record = recordForm("closed", fields);
  fields[0]!.name = "changed";
  fields[0]!.form = text;
  fields.length = 0;
  assert.equal(conforms(map(["x", encodeInt(1n)]), record), true);
  assert.equal(conforms(map(), record), false);
});
