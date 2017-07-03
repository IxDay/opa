// Copyright 2017 The OPA Authors.  All rights reserved.
// Use of this source code is governed by an Apache2
// license that can be found in the LICENSE file.

package dependencies

import (
	"fmt"
	"sort"
	"testing"

	"github.com/open-policy-agent/opa/ast"
)

type testData struct {
	ast  string
	min  []string
	full []string
}

func TestDependencies(t *testing.T) {
	tests := []testData{
		{
			ast: `package a.b.c
			 import data.a.x
			 import data.a.y

			 d {
				a = x
				b = data.a.y
				a = b
			 }`,
			min: []string{"a.x", "a.y"},
		},
		{
			ast: `package a.b.c
			 import data.a.x

			 d {
				a = x
				b = a.y
				a = "4"
			 }`,

			min: []string{"a.x"},
		},
		{
			ast: `package a.b.c
			 import data.a.x

			 d {
				true = a.y
				a = x
			 }`,

			min: []string{"a.x.y"},
		},
		{
			ast: `package a.b.c
			 import data.a.x

			 d = f {
				 a = x.y
				 e = "foo"
				 f = [b | a[i].b = e
					a[i].c = b]
			 }`,

			min: []string{"a.x.y[i].b", "a.x.y[i].c"},
		},
		{
			ast: `package a.b.c
			 import data.a.x

			 d[i] = f {
				 x[i]
				 count([1 | x[i].foo = "foo"], f)
			 }`,

			min: []string{"a.x[i].foo"},
		},
		{
			ast: `package a.b.c
			 import data.a.x

			 d[i] = f {
				 count([1 | x[i].foo = "foo"], f)
				 x[i]
			 }`,

			min: []string{"a.x[i].foo"},
		},
		{
			ast: `package a.b.c
			 import data.a.x

			 d[i] = f {
				 b = x.y
				 b[i]
				 count([1 | x.y[_].foo = "foo"], f)
			 }`,

			min: []string{"a.x.y[i]", "a.x.y[_].foo"},
		},
		{
			ast: `package a.b.c
			 import data.a.x

			 d[i] = f {
				 b = x.y
				 b[i]
				 count([1 | x.y[0].foo = "foo"], f)
			 }`,

			min: []string{"a.x.y[i]", "a.x.y[0].foo"},
		},
		{
			ast: `package a.b.c
			 import data.a.x

			 d[i] {
				x[i]
			 }`,

			min: []string{"a.x[i]"},
		},
		{
			ast: `package a.b.c
			 import data.a.x

			 d[i] {
				b = x.y
				b[i]
			 }`,

			min: []string{"a.x.y[i]"},
		},
		{
			ast: `package a.b.c
			 import data.a.x

			 d[a] = b {
				a = x.y
				b = x.z
				x.y.z = "foo"
				x.z.a.b = "fizz"
				x.a.b = "bar"
			 }`,

			min:  []string{"a.x.y", "a.x.z", "a.x.a.b"},
			full: []string{"a.x.y.z", "a.x.z.a.b"},
		},
		{
			ast: `package a.b.c
			 import data.a.x

			 f {
				 a = x
				 b = a.y
				 c = b.z
				 d = c.a
				 e = d.b
				 e.c = "foo"
			 }`,

			min: []string{"a.x.y.z.a.b.c"},
		},
		{
			ast: `package a.b.c
			 import data.a.x
			 import data.a.y
			 import data.j

			 f {
				 a = x
				 b = a.y
				 c = b.z
				 d = c.a
				 e = d.b
				 d = j
				 e.c = "foo"
				 a = y
				 e["foo"] = "bar"
			 }`,

			min: []string{"a.x", "a.y", "j"},
			full: []string{
				"a.x.y",
				"a.x.y.z",
				"a.x.y.z.a",
				"a.x.y.z.a.b",
				"a.x.y.z.a.b.c",
				"a.x.y.z.a.b.foo",
				"a.y.y",
				"a.y.y.z",
				"a.y.y.z.a",
				"a.y.y.z.a.b",
				"a.y.y.z.a.b.c",
				"a.y.y.z.a.b.foo",
				"j.b",
				"j.b.c",
				"j.b.foo",
			},
		},
		{
			ast: `package a.b.c
			 import data.a.x
			 import data.a.y
			 import data.j

			 f {
				 a = x
				 b = a.y
				 c = b.z
				 d = c.a
				 d.b = "foo"
			 }

			 g {
				 a = x.z.b
				 e = x
				 h = e.y
				 d = h.z
				 i = d.j
				 a = 9
				 i = "bar"
			 }`,

			min: []string{
				"a.x.y.z.a.b",
				"a.x.y.z.j",
				"a.x.z.b",
			},
		},
		{
			ast: `package a.b.c
			 import data.a.x
			 import data.a.y
			 import data.a.z
			 import data.a.f
			 import data.a.g

			 a[i] = [j, k] {
				b = x.y
				y[b.z[0]] = "foo"
				z[b.a.c][i]
				f[j][b.a.b] = "foo"
				g["foo"][k]
			 }`,

			min: []string{
				"a.x.y.z[0]",
				"a.x.y.a.c",
				"a.x.y.a.b",
				"a.y[b.z[0]]",
				"a.z[b.a.c][i]",
				"a.f[j][b.a.b]",
				"a.g.foo[k]",
			},
		},
		{
			ast: `package a.b.c
			 import data.a.x

			 f {
				 a = x
				 b = a.y
				 b = a.z
				 c = b.i
				 c = b.j
				 d = c.m
				 d = c.n
				 d.f = "foo"
			 }`,

			min: []string{"a.x.y", "a.x.z"},
			full: []string{
				"a.x.y.i",
				"a.x.y.i.m",
				"a.x.y.i.m.f",
				"a.x.y.i.n",
				"a.x.y.i.n.f",
				"a.x.y.j",
				"a.x.y.j.m",
				"a.x.y.j.m.f",
				"a.x.y.j.n",
				"a.x.y.j.n.f",
				"a.x.z.i",
				"a.x.z.i.m",
				"a.x.z.i.m.f",
				"a.x.z.i.n",
				"a.x.z.i.n.f",
				"a.x.z.j",
				"a.x.z.j.m",
				"a.x.z.j.m.f",
				"a.x.z.j.n",
				"a.x.z.j.n.f",
			},
		},
		{
			ast: `package a.b.c
			 import data.a.x

			 f = [z, g] {
				 a = x
				 b = a.y.z
				 indexof(a.b, b, z)
				 c = b.e
				 indexof(c, a.b[a.c].c, g)
			 }`,

			min:  []string{"a.x.y.z", "a.x.b", "a.x.c"},
			full: []string{"a.x.b[a.c].c", "a.x.y.z.e"},
		},
		{
			ast: `package a.b.c
			 import data.a.x

			 f = [g] {
				 a = x
				 b = a.y.z
				 c = b.e
				 d = a.u
				 indexof(b, a.c[d].c, g)
				 c = "foo"
			 }`,

			min:  []string{"a.x.y.z", "a.x.u", "a.x.c[d].c"},
			full: []string{"a.x.y.z.e"},
		},
		{
			ast: `package a.b.c
			 import data.a.x

			 f {
				 x
			 }`,

			min: []string{"a.x"},
		},
	}

	for n, test := range tests {
		t.Run(fmt.Sprint(n), func(t *testing.T) {
			module := ast.MustParseModule(test.ast)
			compiler := ast.NewCompiler()
			if compiler.Compile(map[string]*ast.Module{"test": module}); compiler.Failed() {
				t.Fatalf("Failed to compile policy: %v", compiler.Errors)
			}

			var exp []ast.Ref
			for _, e := range test.min {
				r := ast.MustParseRef("data." + e)
				exp = append(exp, r)
			}
			sort.Slice(exp, func(i, j int) bool {
				return exp[i].Compare(exp[j]) < 0
			})

			mod := compiler.Modules["test"]
			min, full := runDeps(t, mod, test)

			// Test that we get the same result by analyzing all the
			// rules separately.
			var minRules, fullRules []ast.Ref
			for _, rule := range mod.Rules {
				m, f := runDeps(t, rule, test)
				minRules = append(minRules, m...)
				fullRules = append(fullRules, f...)
			}

			assertRefSliceEq(t, exp, min)
			assertRefSliceEq(t, exp, minRules)

			for _, full := range test.full {
				r := ast.MustParseRef("data." + full)
				exp = append(exp, r)
			}
			sort.Slice(exp, func(i, j int) bool {
				return exp[i].Compare(exp[j]) < 0
			})

			assertRefSliceEq(t, exp, full)
			assertRefSliceEq(t, exp, fullRules)
		})
	}
}

func runDeps(t *testing.T, x interface{}, test testData) (min, full []ast.Ref) {
	min, err := Minimal(x)
	if err != nil {
		t.Fatalf("Unexpected dependency error: %v", err)
	}

	full, err = All(x)
	if err != nil {
		t.Fatalf("Unexpected dependency error: %v", err)
	}

	return min, full
}

// For some reason, reflect.DeepEqual doesn't work on Ref slices.
func assertRefSliceEq(t *testing.T, exp, result []ast.Ref) {
	if len(result) != len(exp) {
		t.Fatalf("Expected refs %v, got %v", exp, result)
	}

	for i, e := range exp {
		r := result[i]
		if e.Compare(r) != 0 {
			t.Fatalf("Expected refs %v, got %v", exp, result)
			break
		}
	}
}
