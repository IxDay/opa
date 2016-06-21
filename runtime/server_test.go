// Copyright 2016 The OPA Authors.  All rights reserved.
// Use of this source code is governed by an Apache2
// license that can be found in the LICENSE file.

package runtime

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/open-policy-agent/opa/ast"
	"github.com/open-policy-agent/opa/storage"
)

var policyDir string

// TestMain creates a temporary direcotry for the server to
// save policies to. The directory name is stored in policyDir
// and is used by the newFixture function.
func TestMain(m *testing.M) {
	d, err := ioutil.TempDir("", "server_test")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(d)
	policyDir = d
	rc := m.Run()
	os.Exit(rc)
}

type tr struct {
	method string
	path   string
	body   string
	code   int
	resp   string
}

func TestDataV1(t *testing.T) {

	testMod := `package testmod
                p[x] :- q[x], not r[x]
                q[x] :- data.x.y[i] = x
                r[x] :- data.x.z[i] = x

				import req1
				import req2 as reqx
				import req3.attr1
				g :- req1.a[0] = 1, reqx.b[i] = 1
				h :- attr1[i] > 1

				undef :- false
				`
	tests := []struct {
		note string
		reqs []tr
	}{
		{"add root", []tr{
			tr{"PATCH", "/data/x", `[{"op": "add", "path": "/", "value": {"a": 1}}]`, 204, ""},
			tr{"GET", "/data/x/a", "", 200, "1"},
		}},
		{"append array", []tr{
			tr{"PATCH", "/data/x", `[{"op": "add", "path": "/", "value": []}]`, 204, ""},
			tr{"PATCH", "/data/x", `[{"op": "add", "path": "-", "value": {"a": 1}}]`, 204, ""},
			tr{"PATCH", "/data/x", `[{"op": "add", "path": "-", "value": {"a": 2}}]`, 204, ""},
			tr{"GET", "/data/x/0/a", "", 200, "1"},
			tr{"GET", "/data/x/1/a", "", 200, "2"},
		}},
		{"append array one-shot", []tr{
			tr{"PATCH", "/data/x", `[
                {"op": "add", "path": "/", "value": []},
                {"op": "add", "path": "-", "value": {"a": 1}},
                {"op": "add", "path": "-", "value": {"a": 2}}
            ]`, 204, ""},
			tr{"GET", "/data/x/1/a", "", 200, "2"},
		}},
		{"insert array", []tr{
			tr{"PATCH", "/data/x", `[{"op": "add", "path": "/", "value": {
                "y": [
                    {"z": [1,2,3]},
                    {"z": [4,5,6]}
                ]
            }}]`, 204, ""},
			tr{"GET", "/data/x/y/1/z/2", "", 200, "6"},
			tr{"PATCH", "/data/x/y/1", `[{"op": "add", "path": "/z/1", "value": 100}]`, 204, ""},
			tr{"GET", "/data/x/y/1/z", "", 200, "[4, 100, 5, 6]"},
		}},
		{"get virtual", []tr{
			tr{"PUT", "/policies/test", testMod, 200, ""},
			tr{"PATCH", "/data/x", `[{"op": "add", "path": "/", "value": {"y": [1,2,3,4], "z": [3,4,5,6]}}]`, 204, ""},
			tr{"GET", "/data/testmod/p", "", 200, "[1,2]"},
		}},
		{"patch virtual error", []tr{
			tr{"PUT", "/policies/test", testMod, 200, ""},
			tr{"PATCH", "/data/testmod/p", `[{"op": "add", "path": "-", "value": 1}]`, 404, `{
                "Code": 404,
                "Message": "storage error (code: 1): bad path: [testmod p], path refers to non-array document with element p"
            }`},
		}},
		{"get with global", []tr{
			tr{"PUT", "/policies/test", testMod, 200, ""},
			tr{"GET", "/data/testmod/g?global=req1%3A%7B%22a%22%3A%5B1%5D%7D&global=req2%3A%7B%22b%22%3A%5B0%2C1%5D%7D", "", 200, "true"},
		}},
		{"get with global (unbound error)", []tr{
			tr{"PUT", "/policies/test", testMod, 200, ""},
			tr{"GET", "/data/testmod/g?global=req1%3A%7B%22a%22%3A%5B1%5D%7D", "", 400, `{
				"Code": 400,
				"Message": "evaluation error (code: 1): unbound variable req2: req2.b[i]"
			}`},
		}},
		{"get with global (namespaced)", []tr{
			tr{"PUT", "/policies/test", testMod, 200, ""},
			tr{"GET", "/data/testmod/h?global=req3.attr1%3A%5B4%2C3%2C2%2C1%5D", "", 200, `true`},
		}},
		{"get undefined", []tr{
			tr{"PUT", "/policies/test", testMod, 200, ""},
			tr{"GET", "/data/testmod/undef", "", 404, `{"IsUndefined": true}`},
		}},
	}

	for i, tc := range tests {
		executeRequests(t, i+1, tc.note, tc.reqs)
	}
}

func TestIndexGet(t *testing.T) {
	f := newFixture(t)
	get, err := http.NewRequest("GET", `/?q=foo = 1`, strings.NewReader(""))
	if err != nil {
		panic(err)
	}
	f.server.Router.ServeHTTP(f.recorder, get)
	if f.recorder.Code != 200 {
		t.Errorf("Expected success but got: %v", f.recorder)
		return
	}
	page := f.recorder.Body.String()
	if !strings.Contains(page, "Query result") {
		t.Errorf("Expected page to contain 'Query result' but got: %v", page)
		return
	}
}

func TestPoliciesPutV1(t *testing.T) {
	f := newFixture(t)
	req := newReqV1("PUT", "/policies/1", testMod)

	f.server.Router.ServeHTTP(f.recorder, req)

	if f.recorder.Code != 200 {
		t.Errorf("Expected success but got %v", f.recorder)
		return
	}

	policy := f.loadPolicy()
	expected := newPolicy("1", testMod)
	if !expected.Equal(policy) {
		t.Errorf("Expected policies to be equal. Expected:\n\n%v\n\nGot:\n\n%v\n", expected, policy)
	}
}

func TestPoliciesPutV1Empty(t *testing.T) {
	f := newFixture(t)
	req := newReqV1("PUT", "/policies/1", "")

	f.server.Router.ServeHTTP(f.recorder, req)

	if f.recorder.Code != 400 {
		t.Errorf("Expected bad request but got %v", f.recorder)
		return
	}
}

func TestPoliciesPutV1ParseError(t *testing.T) {
	f := newFixture(t)
	req := newReqV1("PUT", "/policies/1", `
    package a.b.c

    p[x] %%^ ;-
    `)

	f.server.Router.ServeHTTP(f.recorder, req)

	if f.recorder.Code != 400 {
		t.Errorf("Expected bad request but got %v", f.recorder)
		return
	}
}

// TODO(tsandall): revisit once safety checks are in place
func testPoliciesPutV1CompileError(t *testing.T) {
	f := newFixture(t)
	req := newReqV1("PUT", "/policies/1", `
    package a.b.c
    p[x] :- q[x]
    q[x] :- p[x]
    `)

	f.server.Router.ServeHTTP(f.recorder, req)

	if f.recorder.Code != 400 {
		t.Errorf("Expected bad request but got %v", f.recorder)
		return
	}
}

func TestPoliciesListV1(t *testing.T) {
	f := newFixture(t)
	put := newReqV1("PUT", "/policies/1", testMod)
	f.server.Router.ServeHTTP(f.recorder, put)
	if f.recorder.Code != 200 {
		t.Errorf("Expected success but got %v", f.recorder)
		return
	}
	f.reset()
	list := newReqV1("GET", "/policies", "")

	f.server.Router.ServeHTTP(f.recorder, list)

	if f.recorder.Code != 200 {
		t.Errorf("Expected success but got %v", f.recorder)
		return
	}

	var policies []*policyV1
	err := json.NewDecoder(f.recorder.Body).Decode(&policies)
	if err != nil {
		t.Errorf("Expected policy list but got error: %v", err)
		return
	}

	expected := []*policyV1{
		newPolicy("1", testMod),
	}
	if len(expected) != len(policies) {
		t.Errorf("Expected %d policies but got: %v", len(expected), policies)
		return
	}
	for i := range expected {
		if !expected[i].Equal(policies[i]) {
			t.Errorf("Expected policies to be equal. Expected:\n\n%v\n\nGot:\n\n%v\n", expected[i], policies[i])
		}
	}
}

func TestPoliciesGetV1(t *testing.T) {
	f := newFixture(t)
	put := newReqV1("PUT", "/policies/1", testMod)
	f.server.Router.ServeHTTP(f.recorder, put)

	if f.recorder.Code != 200 {
		t.Errorf("Expected success but got %v", f.recorder)
		return
	}

	f.reset()
	get := newReqV1("GET", "/policies/1", "")

	f.server.Router.ServeHTTP(f.recorder, get)

	if f.recorder.Code != 200 {
		t.Errorf("Expected success but got %v", f.recorder)
		return
	}

	policy := f.loadPolicy()
	expected := newPolicy("1", testMod)
	if !expected.Equal(policy) {
		t.Errorf("Expected policies to be equal. Expected:\n\n%v\n\nGot:\n\n%v\n", expected, policy)
	}
}

func TestPoliciesGetRawV1(t *testing.T) {
	f := newFixture(t)
	put := newReqV1("PUT", "/policies/1", testMod)
	f.server.Router.ServeHTTP(f.recorder, put)

	if f.recorder.Code != 200 {
		t.Errorf("Expected success but got %v", f.recorder)
		return
	}

	f.reset()
	get := newReqV1("GET", "/policies/1/raw", "")

	f.server.Router.ServeHTTP(f.recorder, get)

	if f.recorder.Code != 200 {
		t.Errorf("Expected success but got %v", f.recorder)
		return
	}

	raw := f.recorder.Body.String()
	if raw != testMod {
		t.Errorf("Expected raw string to equal testMod:\n\nExpected:\n\n%v\n\nGot:\n\n%v\n", testMod, raw)
	}

}

func TestPoliciesDeleteV1(t *testing.T) {
	f := newFixture(t)
	put := newReqV1("PUT", "/policies/1", testMod)
	f.server.Router.ServeHTTP(f.recorder, put)

	if f.recorder.Code != 200 {
		t.Errorf("Expected success but got %v", f.recorder)
		return
	}

	f.reset()
	del := newReqV1("DELETE", "/policies/1", "")

	f.server.Router.ServeHTTP(f.recorder, del)

	if f.recorder.Code != 204 {
		t.Errorf("Expected success but got %v", f.recorder)
		return
	}

	f.reset()
	get := newReqV1("GET", "/policies/1", "")
	f.server.Router.ServeHTTP(f.recorder, get)
	if f.recorder.Code != 404 {
		t.Errorf("Expected not found but got %v", f.recorder)
		return
	}
}

func TestQueryV1(t *testing.T) {
	f := newFixture(t)
	get := newReqV1("GET", `/query?q=a=[1,2,3],a[i]=x`, "")
	f.server.Router.ServeHTTP(f.recorder, get)

	if f.recorder.Code != 200 {
		t.Errorf("Expected success but got %v", f.recorder)
		return
	}

	var expected resultSetV1
	err := json.Unmarshal([]byte(`[{"a":[1,2,3],"i":0,"x":1},{"a":[1,2,3],"i":1,"x":2},{"a":[1,2,3],"i":2,"x":3}]`), &expected)
	if err != nil {
		panic(err)
	}

	var result resultSetV1
	err = json.Unmarshal(f.recorder.Body.Bytes(), &result)
	if err != nil {
		t.Errorf("Unexpected error while unmarshalling result: %v", err)
		return
	}

	if !reflect.DeepEqual(result, expected) {
		t.Errorf("Expected %v but got: %v", expected, result)
	}
}

func TestGlobalParsing(t *testing.T) {

	tests := []struct {
		note     string
		globals  []string
		expected interface{}
	}{
		{"var", []string{`hello:"world"`}, `{hello: "world"}`},
		{"multiple vars", []string{`a:"a"`, `b:"b"`}, `{a: "a", b: "b"}`},
		{"multiple overlapping vars", []string{`a.b.c:"c"`, `a.b.d:"d"`, `x.y:[]`}, `{a: {"b": {"c": "c", "d": "d"}}, x: {"y": []}}`},
		{"conflicting vars", []string{`a.b:"c"`, `a.b.d:"d"`}, globalConflictErr(ast.MustParseRef("a.b.d"))},
		{"conflicting vars-2", []string{`a.b:{"c":[]}`, `a.b.c:["d"]`}, globalConflictErr(ast.MustParseRef("a.b.c"))},
		{"conflicting vars-3", []string{"a:100", `a.b:"c"`}, globalConflictErr(ast.MustParseRef("a.b"))},
		{"conflicting vars-4", []string{`a.b:"c"`, `a:100`}, globalConflictErr(ast.MustParseTerm("a").Value)},
		{"bad path", []string{`"hello":1`}, fmt.Errorf(`invalid global: "hello": path must be a variable or a reference`)},
	}

	for i, tc := range tests {

		bindings, err := parseGlobals(tc.globals)

		switch e := tc.expected.(type) {
		case error:
			if err == nil {
				t.Errorf("%v (#%d): Expected error %v but got: %v", tc.note, i+1, e, bindings)
				continue
			}
			if !reflect.DeepEqual(e, err) {
				t.Errorf("%v (#%d): Expected error %v but got: %v", tc.note, i+1, e, err)
			}
		case string:
			if err != nil {
				t.Errorf("%v (#%d): Unexpected error: %v", tc.note, i+1, err)
				continue
			}
			exp := storage.NewBindings()
			for _, i := range ast.MustParseTerm(e).Value.(ast.Object) {
				exp.Put(i[0].Value, i[1].Value)
			}
			if !exp.Equal(bindings) {
				t.Errorf("%v (#%d): Expected bindings to equal %v but got: %v", tc.note, i+1, exp, bindings)
			}
		}
	}
}

const (
	testMod = `
    package a.b.c
    import data.x.y as z
    import data.p
    q[x] :- p[x], not r[x]
    r[x] :- z[x] = 4`
)

type fixture struct {
	runtime  *Runtime
	server   *Server
	recorder *httptest.ResponseRecorder
	t        *testing.T
}

func newFixture(t *testing.T) *fixture {
	runtime := &Runtime{}
	runtime.Init(&Params{Server: true, PolicyDir: policyDir})
	server := NewServer(runtime, ":8182", false)
	recorder := httptest.NewRecorder()
	return &fixture{
		runtime:  runtime,
		server:   server,
		recorder: recorder,
		t:        t,
	}
}

func (f *fixture) loadPolicy() *policyV1 {
	policy := &policyV1{}
	err := json.NewDecoder(f.recorder.Body).Decode(policy)
	if err != nil {
		panic(err)
	}
	return policy
}

func (f *fixture) loadResponse() interface{} {
	var v interface{}
	err := json.NewDecoder(f.recorder.Body).Decode(&v)
	if err != nil {
		panic(err)
	}
	return v
}

func (f *fixture) v1(method string, path string, body string, code int, resp string) error {
	req := newReqV1(method, path, body)
	f.reset()
	f.server.Router.ServeHTTP(f.recorder, req)
	if f.recorder.Code != code {
		return fmt.Errorf("Expected code %v from %v %v but got: %v", method, code, path, f.recorder)
	}
	if resp != "" {
		var result interface{}
		if err := json.Unmarshal([]byte(f.recorder.Body.String()), &result); err != nil {
			return fmt.Errorf("Expected JSON response from %v %v but got: %v", method, path, f.recorder)
		}
		var expected interface{}
		if err := json.Unmarshal([]byte(resp), &expected); err != nil {
			panic(err)
		}
		if !reflect.DeepEqual(result, expected) {
			return fmt.Errorf("Expected JSON response from %v %v to equal %v but got: %v", method, path, expected, result)
		}
	}
	return nil
}

func (f *fixture) reset() {
	f.recorder = httptest.NewRecorder()
}

func executeRequests(t *testing.T, tc int, note string, reqs []tr) {
	f := newFixture(t)
	for i, req := range reqs {
		if err := f.v1(req.method, req.path, req.body, req.code, req.resp); err != nil {
			t.Errorf("Unexpected response on request %d of test case %d (%v): %v", i+1, tc, note, err)
		}
	}
}

func newPolicy(id, s string) *policyV1 {
	compiler := ast.NewCompiler()
	parsed := ast.MustParseModule(s)
	if compiler.Compile(map[string]*ast.Module{"": parsed}); compiler.Failed() {
		panic(compiler.FlattenErrors())
	}
	mod := compiler.Modules[""]
	return &policyV1{ID: id, Module: mod}
}

func newReqV1(method string, path string, body string) *http.Request {
	req, err := http.NewRequest(method, "/v1"+path, strings.NewReader(body))
	if err != nil {
		panic(err)
	}
	return req
}
