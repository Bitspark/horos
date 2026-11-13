// Command bench times Conforms and Diagnose on ordinary input: a list of n closed records
// {name text, age opt int, tags list-of text}, with age's type an int ("plain") or a
// three-way union of leaves ("union"). It measures the checker, not seeded conformance,
// and no check runs it; scripts/bench.py runs it beside the other cores (ADR 0013).
//
//	go run ./internal/bench 200000 plain
package main

import (
	"fmt"
	"math/big"
	"os"
	"runtime"
	"strconv"
	"time"

	horos "github.com/Bitspark/horos/core/go"
	core "github.com/bitspark/ontos/core/go"
	data "github.com/bitspark/ontos/data/go"
)

func a(s string) core.Value { return core.NewAtom([]byte(s)) }

func d(ctor string, args ...core.Value) core.Value {
	return core.NewTuple(append([]core.Value{a("horos/" + ctor)}, args...)...)
}

func text(s string) core.Value { return data.MustEncodeText(s) }

func field(name, presence string, ty core.Value) core.Value {
	return d("field", text(name), a(presence), ty)
}

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintln(os.Stderr, "usage: bench <n> plain|union")
		os.Exit(2)
	}
	n, err := strconv.Atoi(os.Args[1])
	if err != nil || n < 0 {
		fmt.Fprintln(os.Stderr, "bench: n must be a non-negative integer")
		os.Exit(2)
	}
	age := d("scalar", a("int"))
	if os.Args[2] == "union" {
		age = d("union", d("exact", data.EncodeNull()), d("scalar", a("int")), d("scalar", a("utf8-text")))
	}
	desc := d("list-of", d("record", a("closed"),
		field("name", "req", d("scalar", a("utf8-text"))),
		field("age", "opt", age),
		field("tags", "req", d("list-of", d("scalar", a("utf8-text"))))))
	items := make([]core.Value, n)
	for i := range items {
		items[i], err = data.EncodeMap([][2]core.Value{
			{text("age"), data.EncodeInt(big.NewInt(int64(i)))},
			{text("name"), text("n" + strconv.Itoa(i))},
			{text("tags"), data.EncodeList([]core.Value{text("a"), text("b"), text("c")})},
		})
		if err != nil {
			panic(err)
		}
	}
	v := data.EncodeList(items)

	measure := func(run func() string) (string, time.Duration, float64) {
		runtime.GC()
		var m0, m1 runtime.MemStats
		runtime.ReadMemStats(&m0)
		start := time.Now()
		out := run()
		elapsed := time.Since(start)
		runtime.ReadMemStats(&m1)
		return out, elapsed, float64(m1.TotalAlloc-m0.TotalAlloc) / 1e6
	}
	ok, ct, ca := measure(func() string { return strconv.FormatBool(horos.Conforms(v, desc)) })
	dg, dt, da := measure(func() string { return string(horos.Diagnose(v, desc).Verdict) })
	fmt.Printf(`{"core":"go","n":%d,"mode":%q,"conforms":%s,"conforms_ms":%d,"conforms_mb":%.1f,"diagnose":%q,"diagnose_ms":%d,"diagnose_mb":%.1f}`+"\n",
		n, os.Args[2], ok, ct.Milliseconds(), ca, dg, dt.Milliseconds(), da)
}
