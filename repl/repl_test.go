// Copyright 2016 The OPA Authors.  All rights reserved.
// Use of this source code is governed by an Apache2
// license that can be found in the LICENSE file.

package repl

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/open-policy-agent/opa/ast"
	"github.com/open-policy-agent/opa/storage"
	"github.com/open-policy-agent/opa/util"
)

func TestComplete(t *testing.T) {
	store := newTestStore()

	mod1 := ast.MustParseModule(`package a.b.c
	p = 1
	q = 2`)

	mod2 := ast.MustParseModule(`package a.b.d
	r = 3`)

	if err := storage.InsertPolicy(store, "mod1", mod1, nil, false); err != nil {
		panic(err)
	}

	if err := storage.InsertPolicy(store, "mod2", mod2, nil, false); err != nil {
		panic(err)
	}

	var buf bytes.Buffer
	repl := newRepl(store, &buf)
	repl.OneShot("s = 4")
	buf.Reset()

	result := repl.complete("")
	expected := []string{
		"data.a.b.c.p",
		"data.a.b.c.q",
		"data.a.b.d.r",
		"data.repl.s",
		"data.repl.version",
	}

	sort.Strings(result)
	sort.Strings(expected)

	if !reflect.DeepEqual(result, expected) {
		t.Fatalf("Expected %v but got: %v", expected, result)
	}

	result = repl.complete("data.a.b")
	expected = []string{
		"data.a.b.c.p",
		"data.a.b.c.q",
		"data.a.b.d.r",
	}

	sort.Strings(result)
	sort.Strings(expected)

	if !reflect.DeepEqual(result, expected) {
		t.Fatalf("Expected %v but got: %v", expected, result)
	}

	result = repl.complete("data.a.b.c.p[x]")
	expected = nil

	if !reflect.DeepEqual(result, expected) {
		t.Fatalf("Expected %v but got: %v", expected, result)
	}

	repl.OneShot("import data.a.b.c.p as xyz")
	repl.OneShot("import data.a.b.d")

	result = repl.complete("x")
	expected = []string{
		"xyz",
	}

	if !reflect.DeepEqual(result, expected) {
		t.Fatalf("Expected %v but got: %v", expected, result)
	}
}

func TestDump(t *testing.T) {
	input := `{"a": [1,2,3,4]}`
	var data map[string]interface{}
	err := util.UnmarshalJSON([]byte(input), &data)
	if err != nil {
		panic(err)
	}
	store := storage.New(storage.InMemoryWithJSONConfig(data))
	var buffer bytes.Buffer
	repl := newRepl(store, &buffer)
	repl.OneShot("dump")
	expectOutput(t, buffer.String(), "{\"a\":[1,2,3,4]}\n")
}

func TestDumpPath(t *testing.T) {
	input := `{"a": [1,2,3,4]}`
	var data map[string]interface{}
	err := util.UnmarshalJSON([]byte(input), &data)
	if err != nil {
		panic(err)
	}
	store := storage.New(storage.InMemoryWithJSONConfig(data))
	var buffer bytes.Buffer
	repl := newRepl(store, &buffer)

	dir, err := ioutil.TempDir("", "dump-path-test")
	if err != nil {
		t.Fatal(err)
	}

	defer os.RemoveAll(dir)
	file := filepath.Join(dir, "tmpfile")
	repl.OneShot(fmt.Sprintf("dump %s", file))

	if buffer.String() != "" {
		t.Errorf("Expected no output but got: %v", buffer.String())
	}

	bs, err := ioutil.ReadFile(file)
	if err != nil {
		t.Fatalf("Expected file read to succeed but got: %v", err)
	}

	var result map[string]interface{}
	if err := util.UnmarshalJSON(bs, &result); err != nil {
		t.Fatalf("Expected json unmarhsal to suceed but got: %v", err)
	}

	if !reflect.DeepEqual(data, result) {
		t.Fatalf("Expected dumped json to equal %v but got: %v", data, result)
	}
}

func TestShow(t *testing.T) {
	store := storage.New(storage.InMemoryConfig())
	var buffer bytes.Buffer
	repl := newRepl(store, &buffer)

	repl.OneShot("package repl_test")
	repl.OneShot("show")
	assertREPLText(t, buffer, "package repl_test\n")
	buffer.Reset()

	repl.OneShot("import xyz")
	repl.OneShot("show")

	expected := `package repl_test

import xyz` + "\n"
	assertREPLText(t, buffer, expected)
	buffer.Reset()

	repl.OneShot("import data.foo as bar")
	repl.OneShot("show")

	expected = `package repl_test

import xyz
import data.foo as bar` + "\n"
	assertREPLText(t, buffer, expected)
	buffer.Reset()

	repl.OneShot("p[1] :- true")
	repl.OneShot("p[2] :- true")
	repl.OneShot("show")

	expected = `package repl_test

import xyz
import data.foo as bar

p[1] :- true
p[2] :- true` + "\n"
	assertREPLText(t, buffer, expected)
	buffer.Reset()

	repl.OneShot("package abc")
	repl.OneShot("show")

	assertREPLText(t, buffer, "package abc\n")
	buffer.Reset()

	repl.OneShot("package repl_test")
	repl.OneShot("show")

	assertREPLText(t, buffer, expected)
	buffer.Reset()
}

func TestUnset(t *testing.T) {
	store := storage.New(storage.InMemoryConfig())
	var buffer bytes.Buffer
	repl := newRepl(store, &buffer)

	repl.OneShot("magic = 23")
	repl.OneShot("p = 3.14")
	repl.OneShot("unset p")

	err := repl.OneShot("p")
	if _, ok := err.(ast.Errors); !ok {
		t.Fatalf("Expected AST error but got: %v", err)
	}

	buffer.Reset()
	repl.OneShot("p = 3.14")
	repl.OneShot("p = 3 :- false")
	repl.OneShot("unset p")

	err = repl.OneShot("p")
	if _, ok := err.(ast.Errors); !ok {
		t.Fatalf("Expected AST error but got err: %v, output: %v", err, buffer.String())
	}

	if err := repl.OneShot("unset "); err == nil {
		t.Fatalf("Expected unset error for bad syntax but got: %v", buffer.String())
	}

	if err := repl.OneShot("unset 1=1"); err == nil {
		t.Fatalf("Expected unset error for bad syntax but got: %v", buffer.String())
	}

	if err := repl.OneShot(`unset "p"`); err == nil {
		t.Fatalf("Expected unset error for bad syntax but got: %v", buffer.String())
	}

	buffer.Reset()
	repl.OneShot(`unset q`)
	if buffer.String() != "warning: no matching rules in current module\n" {
		t.Fatalf("Expected unset error for missing rule but got: %v", buffer.String())
	}

	buffer.Reset()
	repl.OneShot(`magic`)
	if buffer.String() != "23\n" {
		t.Fatalf("Expected magic to be defined but got: %v", buffer.String())
	}

	buffer.Reset()
	repl.OneShot(`package data.other`)
	repl.OneShot(`unset magic`)
	if buffer.String() != "warning: no matching rules in current module\n" {
		t.Fatalf("Expected unset error for bad syntax but got: %v", buffer.String())
	}
}

func TestOneShotEmptyBufferOneExpr(t *testing.T) {
	store := newTestStore()
	var buffer bytes.Buffer
	repl := newRepl(store, &buffer)
	repl.OneShot("data.a[i].b.c[j] = 2")
	expectOutput(t, buffer.String(), "+---+---+\n| i | j |\n+---+---+\n| 0 | 1 |\n+---+---+\n")
	buffer.Reset()
	repl.OneShot("data.a[i].b.c[j] = \"deadbeef\"")
	expectOutput(t, buffer.String(), "false\n")
}

func TestOneShotEmptyBufferOneRule(t *testing.T) {
	store := newTestStore()
	var buffer bytes.Buffer
	repl := newRepl(store, &buffer)
	repl.OneShot("p[x] :- data.a[i] = x")
	expectOutput(t, buffer.String(), "")
}

func TestOneShotBufferedExpr(t *testing.T) {
	store := newTestStore()
	var buffer bytes.Buffer
	repl := newRepl(store, &buffer)
	repl.OneShot("data.a[i].b.c[j] = ")
	expectOutput(t, buffer.String(), "")
	repl.OneShot("2")
	expectOutput(t, buffer.String(), "")
	repl.OneShot("")
	expectOutput(t, buffer.String(), "+---+---+\n| i | j |\n+---+---+\n| 0 | 1 |\n+---+---+\n")
}

func TestOneShotBufferedRule(t *testing.T) {
	store := newTestStore()
	var buffer bytes.Buffer
	repl := newRepl(store, &buffer)
	repl.OneShot("p[x] :- ")
	expectOutput(t, buffer.String(), "")
	repl.OneShot("data.a[i]")
	expectOutput(t, buffer.String(), "")
	repl.OneShot(" = ")
	expectOutput(t, buffer.String(), "")
	repl.OneShot("x")
	expectOutput(t, buffer.String(), "")
	repl.OneShot("")
	expectOutput(t, buffer.String(), "")
}

func TestOneShotJSON(t *testing.T) {
	store := newTestStore()
	var buffer bytes.Buffer
	repl := newRepl(store, &buffer)
	repl.outputFormat = "json"
	repl.OneShot("data.a[i] = x")
	var expected interface{}
	input := `
	[
		{
			"i": 0,
			"x": {
			"b": {
				"c": [
				true,
				2,
				false
				]
			}
			}
		},
		{
			"i": 1,
			"x": {
			"b": {
				"c": [
				false,
				true,
				1
				]
			}
			}
		}
	]
	`
	if err := util.UnmarshalJSON([]byte(input), &expected); err != nil {
		panic(err)
	}

	var result interface{}

	if err := util.UnmarshalJSON(buffer.Bytes(), &result); err != nil {
		t.Errorf("Unexpected output format: %v", err)
		return
	}
	if !reflect.DeepEqual(expected, result) {
		t.Errorf("Expected %v but got: %v", expected, result)
	}
}

func TestEvalData(t *testing.T) {
	store := newTestStore()
	var buffer bytes.Buffer
	repl := newRepl(store, &buffer)
	testmod := ast.MustParseModule(`package ex
	p = [1,2,3]`)
	if err := storage.InsertPolicy(store, "test", testmod, nil, false); err != nil {
		panic(err)
	}
	repl.OneShot("data")
	expected := parseJSON(`
	{
		"a": [
			{
			"b": {
				"c": [
				true,
				2,
				false
				]
			}
			},
			{
			"b": {
				"c": [
				false,
				true,
				1
				]
			}
			}
		],
		"ex": {
			"p": [
			1,
			2,
			3
			]
		}
	}`)
	result := parseJSON(buffer.String())

	// Strip REPL documents out as these change depending on build settings.
	data := result.(map[string]interface{})
	delete(data, "repl")

	if !reflect.DeepEqual(result, expected) {
		t.Fatalf("Expected:\n%v\n\nGot:\n%v", expected, result)
	}
}

func TestEvalFalse(t *testing.T) {
	store := newTestStore()
	var buffer bytes.Buffer
	repl := newRepl(store, &buffer)
	repl.OneShot("false")
	result := buffer.String()
	if result != "false\n" {
		t.Errorf("Expected result to be false but got: %v", result)
	}
}

func TestEvalConstantRule(t *testing.T) {
	store := newTestStore()
	var buffer bytes.Buffer
	repl := newRepl(store, &buffer)
	repl.OneShot("pi = 3.14")
	result := buffer.String()
	if result != "" {
		t.Errorf("Expected rule to be defined but got: %v", result)
		return
	}
	buffer.Reset()
	repl.OneShot("pi")
	result = buffer.String()
	expected := "3.14\n"
	if result != expected {
		t.Errorf("Expected pi to evaluate to 3.14 but got: %v", result)
		return
	}
	buffer.Reset()
	repl.OneShot("pi.deadbeef")
	result = buffer.String()
	if result != "undefined\n" {
		t.Errorf("Expected pi.deadbeef to be undefined but got: %v", result)
		return
	}
	buffer.Reset()
	repl.OneShot("pi > 3")
	result = buffer.String()
	if result != "true\n" {
		t.Errorf("Expected pi > 3 to be true but got: %v", result)
		return
	}
}

func TestEvalSingleTermMultiValue(t *testing.T) {
	store := newTestStore()
	var buffer bytes.Buffer
	repl := newRepl(store, &buffer)
	repl.outputFormat = "json"

	input := `
	[
		{
			"data.a[i].b.c[_]": true,
			"i": 0
		},
		{
			"data.a[i].b.c[_]": 2,
			"i": 0
		},
		{
			"data.a[i].b.c[_]": false,
			"i": 0
		},
		{
			"data.a[i].b.c[_]": false,
			"i": 1
		},
		{
			"data.a[i].b.c[_]": true,
			"i": 1
		},
		{
			"data.a[i].b.c[_]": 1,
			"i": 1
		}
	]`

	var expected interface{}
	if err := util.UnmarshalJSON([]byte(input), &expected); err != nil {
		panic(err)
	}

	repl.OneShot("data.a[i].b.c[_]")
	var result interface{}
	if err := util.UnmarshalJSON(buffer.Bytes(), &result); err != nil {
		t.Errorf("Expected valid JSON document: %v: %v", err, buffer.String())
		return
	}

	if !reflect.DeepEqual(expected, result) {
		t.Errorf("Expected %v but got: %v", expected, result)
		return
	}

	buffer.Reset()

	repl.OneShot("data.deadbeef[x]")
	s := buffer.String()
	if s != "undefined\n" {
		t.Errorf("Expected undefined from reference but got: %v", s)
		return
	}

	buffer.Reset()

	repl.OneShot("p[x] :- a = [1,2,3,4], a[_] = x")
	buffer.Reset()
	repl.OneShot("p[x]")

	input = `
	[
		{
			"x": 1
		},
		{
			"x": 2
		},
		{
			"x": 3
		},
		{
			"x": 4
		}
	]
	`

	if err := util.UnmarshalJSON([]byte(input), &expected); err != nil {
		panic(err)
	}

	if err := util.UnmarshalJSON(buffer.Bytes(), &result); err != nil {
		t.Errorf("Expected valid JSON document: %v: %v", err, buffer.String())
		return
	}

	if !reflect.DeepEqual(expected, result) {
		t.Errorf("Exepcted %v but got: %v", expected, result)
	}
}

func TestEvalSingleTermMultiValueSetRef(t *testing.T) {
	store := newTestStore()
	var buffer bytes.Buffer
	repl := newRepl(store, &buffer)
	repl.outputFormat = "json"
	repl.OneShot("p[1] :- true")
	repl.OneShot("p[2] :- true")
	repl.OneShot("q = {3,4} :- true")
	repl.OneShot("r = [x, y] :- x = {5,6}, y = [7,8]")

	repl.OneShot("p[x]")
	expected := parseJSON(`[{"x": 1}, {"x": 2}]`)
	result := parseJSON(buffer.String())
	if !reflect.DeepEqual(result, expected) {
		t.Fatalf("Expected %v but got: %v", expected, result)
	}

	buffer.Reset()
	repl.OneShot("q[x]")
	expected = parseJSON(`[{"x": 3}, {"x": 4}]`)
	result = parseJSON(buffer.String())
	if !reflect.DeepEqual(result, expected) {
		t.Fatalf("Expected %v but got: %v", expected, result)
	}

	// Example below shows behavior for ref that iterates an embedded set. The
	// tricky part here is that r[_] may refer to multiple collection types. If
	// we eventually have a way of distinguishing between the bindings added for
	// refs to sets, then those bindings could be filtered out. For now this is
	// acceptable, as it should be an edge case.
	buffer.Reset()
	repl.OneShot("r[_][x]")
	expected = parseJSON(`[{"x": 5, "r[_][x]": true}, {"x": 6, "r[_][x]": true}, {"x": 0, "r[_][x]": 7}, {"x": 1, "r[_][x]": 8}]`)
	result = parseJSON(buffer.String())
	if !reflect.DeepEqual(result, expected) {
		t.Fatalf("Expected %v but got: %v", expected, result)
	}
}

func TestEvalRuleCompileError(t *testing.T) {
	store := newTestStore()
	var buffer bytes.Buffer
	repl := newRepl(store, &buffer)
	repl.OneShot("p[x] :- true")
	result := buffer.String()
	expected := "error: 1 error occurred: 1:1: p: x is unsafe (variable x must appear in at least one expression within the body of p)\n"
	if result != expected {
		t.Errorf("Expected error message in output but got: %v", result)
		return
	}
	buffer.Reset()
	repl.OneShot("p = true :- true")
	result = buffer.String()
	if result != "" {
		t.Errorf("Expected valid rule to compile (because state should be unaffected) but got: %v", result)
	}
}

func TestEvalBodyCompileError(t *testing.T) {
	store := newTestStore()
	var buffer bytes.Buffer
	repl := newRepl(store, &buffer)
	repl.outputFormat = "json"
	err := repl.OneShot("x = 1, y > x")
	if _, ok := err.(ast.Errors); !ok {
		t.Fatalf("Expected error message in output but got`: %v", buffer.String())
	}
	buffer.Reset()
	repl.OneShot("x = 1, y = 2, y > x")
	var result2 []interface{}
	err = util.UnmarshalJSON(buffer.Bytes(), &result2)
	if err != nil {
		t.Errorf("Expected valid JSON output but got: %v", buffer.String())
		return
	}
	expected2 := []interface{}{
		map[string]interface{}{
			"x": json.Number("1"),
			"y": json.Number("2"),
		},
	}
	if !reflect.DeepEqual(expected2, result2) {
		t.Errorf(`Expected [{"x": 1, "y": 2}] but got: %v"`, result2)
		return
	}
}

func TestEvalBodyContainingWildCards(t *testing.T) {
	store := newTestStore()
	var buffer bytes.Buffer
	repl := newRepl(store, &buffer)
	repl.OneShot("data.a[_].b.c[_] = x")
	expected := strings.TrimSpace(`
+-------+
|   x   |
+-------+
| true  |
| 2     |
| false |
| false |
| true  |
| 1     |
+-------+`)
	result := strings.TrimSpace(buffer.String())
	if result != expected {
		t.Errorf("Expected only a single column of output but got:\n%v", result)
	}

}

func TestEvalBodyGlobals(t *testing.T) {
	store := newTestStore()
	var buffer bytes.Buffer
	repl := newRepl(store, &buffer)

	repl.OneShot("package repl")
	repl.OneShot(`globals["foo.bar"] = "hello" :- true`)
	repl.OneShot(`globals["baz"] = data.a[0].b.c[2] :- true`)
	repl.OneShot("package test")
	repl.OneShot("import foo.bar")
	repl.OneShot("import baz")
	repl.OneShot(`p :- bar = "hello", baz = false`)

	repl.OneShot("p")

	result := buffer.String()
	if result != "true\n" {
		t.Fatalf("expected true but got: %v", result)
	}
}

func TestEvalImport(t *testing.T) {
	store := newTestStore()
	var buffer bytes.Buffer
	repl := newRepl(store, &buffer)
	repl.OneShot("import data.a")
	if len(buffer.Bytes()) != 0 {
		t.Errorf("Expected no output but got: %v", buffer.String())
		return
	}
	buffer.Reset()
	repl.OneShot("a[0].b.c[0] = true")
	result := buffer.String()
	expected := "true\n"
	if result != expected {
		t.Errorf("Expected expression to evaluate successfully but got: %v", result)
		return
	}

	// https://github.com/open-policy-agent/opa/issues/158 - re-run query to
	// make sure import is not lost
	buffer.Reset()
	repl.OneShot("a[0].b.c[0] = true")
	result = buffer.String()
	expected = "true\n"
	if result != expected {
		t.Fatalf("Expected expression to evaluate successfully but got: %v", result)
	}
}

func TestEvalPackage(t *testing.T) {
	store := newTestStore()
	var buffer bytes.Buffer
	repl := newRepl(store, &buffer)
	repl.OneShot("package foo.bar")
	repl.OneShot("p = true :- true")
	repl.OneShot("package baz.qux")
	buffer.Reset()
	err := repl.OneShot("p")
	if err.Error() != "1 error occurred: 1:1: p is unsafe (variable p must appear in the output position of at least one non-negated expression)" {
		t.Fatalf("Expected unsafe variable error but got: %v", err)
	}
	repl.OneShot("import data.foo.bar.p")
	buffer.Reset()
	repl.OneShot("p")
	if buffer.String() != "true\n" {
		t.Errorf("Expected expression to eval successfully but got: %v", buffer.String())
		return
	}
}

func TestEvalTrace(t *testing.T) {
	store := newTestStore()
	var buffer bytes.Buffer
	repl := newRepl(store, &buffer)
	repl.OneShot("trace")
	repl.OneShot("data.a[i].b.c[j] = x, data.a[k].b.c[x] = 1")
	expected := strings.TrimSpace(`
Enter eq(data.a[i].b.c[j], x), eq(data.a[k].b.c[x], 1)
| Eval eq(data.a[i].b.c[j], x)
| Eval eq(data.a[k].b.c[true], 1)
| Fail eq(data.a[k].b.c[true], 1)
| Redo eq(data.a[0].b.c[0], x)
| Eval eq(data.a[k].b.c[2], 1)
| Fail eq(data.a[0].b.c[2], 1)
| Redo eq(data.a[0].b.c[2], 1)
| Exit eq(data.a[i].b.c[j], x), eq(data.a[k].b.c[x], 1)
Redo eq(data.a[i].b.c[j], x), eq(data.a[k].b.c[x], 1)
| Redo eq(data.a[0].b.c[1], x)
| Eval eq(data.a[k].b.c[false], 1)
| Fail eq(data.a[k].b.c[false], 1)
| Redo eq(data.a[0].b.c[2], x)
| Eval eq(data.a[k].b.c[false], 1)
| Fail eq(data.a[k].b.c[false], 1)
| Redo eq(data.a[1].b.c[0], x)
| Eval eq(data.a[k].b.c[true], 1)
| Fail eq(data.a[k].b.c[true], 1)
| Redo eq(data.a[1].b.c[1], x)
| Eval eq(data.a[k].b.c[1], 1)
| Fail eq(data.a[0].b.c[1], 1)
| Redo eq(data.a[0].b.c[1], 1)
| Fail eq(data.a[1].b.c[1], 1)
+---+---+---+---+
| i | j | k | x |
+---+---+---+---+
| 0 | 1 | 1 | 2 |
+---+---+---+---+`)
	expected += "\n"

	if expected != buffer.String() {
		t.Fatalf("Expected output to be exactly:\n%v\n\nGot:\n\n%v\n", expected, buffer.String())
	}
}

func TestEvalTruth(t *testing.T) {
	store := newTestStore()
	var buffer bytes.Buffer
	repl := newRepl(store, &buffer)
	repl.OneShot("truth")
	repl.OneShot("data.a[i].b.c[j] = x, data.a[k].b.c[x] = 1")
	expected := strings.TrimSpace(`
Enter eq(data.a[i].b.c[j], x), eq(data.a[k].b.c[x], 1)
| Redo eq(data.a[0].b.c[0], x)
| Redo eq(data.a[0].b.c[2], 1)
| Exit eq(data.a[i].b.c[j], x), eq(data.a[k].b.c[x], 1)
+---+---+---+---+
| i | j | k | x |
+---+---+---+---+
| 0 | 1 | 1 | 2 |
+---+---+---+---+`)
	expected += "\n"

	if expected != buffer.String() {
		t.Fatalf("Expected output to be exactly:\n%v\n\nGot:\n\n%v\n", expected, buffer.String())
	}
}

func TestBuildHeader(t *testing.T) {
	expr := ast.MustParseStatement(`[{"a": x, "b": data.a.b[y]}] = [{"a": 1, "b": 2}]`).(ast.Body)[0]
	terms := expr.Terms.([]*ast.Term)
	result := map[string]struct{}{}
	buildHeader(result, terms[1])
	expected := map[string]struct{}{
		"x": struct{}{}, "y": struct{}{},
	}
	if !reflect.DeepEqual(result, expected) {
		t.Errorf("Build header expected %v but got %v", expected, result)
	}
}

func assertREPLText(t *testing.T, buf bytes.Buffer, expected string) {
	result := buf.String()
	if result != expected {
		t.Fatalf("Expected:\n%v\n\nGot:\n%v", expected, result)
	}
}

func expectOutput(t *testing.T, output string, expected string) {
	if output != expected {
		t.Errorf("Repl output: expected %#v but got %#v", expected, output)
	}
}

func newRepl(store *storage.Storage, buffer *bytes.Buffer) *REPL {
	repl := New(store, "", buffer, "", "")
	return repl
}

func newTestStore() *storage.Storage {
	input := `
    {
        "a": [
            {
                "b": {
                    "c": [true,2,false]
                }
            },
            {
                "b": {
                    "c": [false,true,1]
                }
            }
        ]
    }
    `
	var data map[string]interface{}
	err := util.UnmarshalJSON([]byte(input), &data)
	if err != nil {
		panic(err)
	}
	return storage.New(storage.InMemoryWithJSONConfig(data))
}

func parseJSON(s string) interface{} {
	var v interface{}
	if err := util.UnmarshalJSON([]byte(s), &v); err != nil {
		panic(err)
	}
	return v
}
