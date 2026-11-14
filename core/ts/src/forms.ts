/** Construction of existing ground descriptor syntax; checking remains in wf/diagnose. */
import { atom, assertValue, tuple, type Value } from "@bitspark/ontos-core";
import { encodeText } from "@bitspark/ontos-data";

export type RecordMode = "open" | "closed";

export interface RecordField {
  readonly name: string;
  readonly presence: "required" | "optional";
  readonly form: Value;
}

const bytes = new TextEncoder();
const word = (name: string): Value => atom(bytes.encode(name));

/** Constructs horos/list-of without closing or checking the supplied child form. */
export function listForm(itemForm: Value): Value {
  assertValue(itemForm);
  return tuple([word("horos/list-of"), itemForm]);
}

/** Constructs horos/map-of, preserving the supplied key and value forms. */
export function mapForm(keyForm: Value, valueForm: Value): Value {
  assertValue(keyForm);
  assertValue(valueForm);
  return tuple([word("horos/map-of"), keyForm, valueForm]);
}

/**
 * Constructs the existing horos/record descriptor, preserving field order and
 * child forms. Names are Unicode scalar strings, without normalization, and must
 * be distinct. Mode and presence are explicit; an absent optional field is not
 * a field containing null.
 *
 * This checks the record's local construction inputs, not whole-descriptor
 * well-formedness. In particular, a child may contain a ref that an enclosing
 * rec will bind. Use wf or diagnoseWf on the completed descriptor.
 *
 * Throws TypeError for invalid native inputs or duplicate names; encodeText's
 * DataError rejects a name containing an unpaired surrogate. No coercion occurs.
 */
export function recordForm(mode: RecordMode, fields: readonly RecordField[]): Value {
  if (mode !== "open" && mode !== "closed") {
    throw new TypeError("record mode must be open or closed");
  }
  if (!Array.isArray(fields)) throw new TypeError("record fields must be an array");
  const names = new Set<string>();
  const children: Value[] = [word("horos/record"), word(mode)];
  for (const field of fields) {
    if (field === null || typeof field !== "object") {
      throw new TypeError("record field must provide name, presence and form");
    }
    const { name, presence, form } = field;
    if (typeof name !== "string") throw new TypeError("record field name must be text");
    if (presence !== "required" && presence !== "optional") {
      throw new TypeError("record field presence must be required or optional");
    }
    const encodedName = encodeText(name);
    if (names.has(name)) throw new TypeError("record field names must be distinct");
    names.add(name);
    assertValue(form);
    children.push(
      tuple([
        word("horos/field"),
        encodedName,
        word(presence === "required" ? "req" : "opt"),
        form,
      ]),
    );
  }
  return tuple(children);
}
