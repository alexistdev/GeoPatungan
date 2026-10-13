/*
 * Copyright (c) 2026.
 * Github : https://github.com/alexistdev
 * Email   : AlexistDev@gmail.com
 */

package app

import (
	"encoding/json"
	"reflect"
	"testing"
)

func shapeJSON(t *testing.T, doc string) any {
	t.Helper()
	var v any
	if err := json.Unmarshal([]byte(doc), &v); err != nil {
		t.Fatal(err)
	}
	return shapeOf(v)
}

func want(t *testing.T, doc string) any {
	t.Helper()
	var v any
	if err := json.Unmarshal([]byte(doc), &v); err != nil {
		t.Fatal(err)
	}
	return v
}

func TestShapeOfScalarsObjectsArrays(t *testing.T) {
	got := shapeJSON(t, `{"a":1,"b":"x","c":true,"d":null,"e":[],"f":[{"g":1},{"g":2}],"h":{"i":"x"}}`)
	exp := want(t, `{"a":"number","b":"string","c":"boolean","d":"null","e":["unknown"],"f":[{"g":"number"}],"h":{"i":"string"}}`)
	if !reflect.DeepEqual(got, exp) {
		t.Errorf("got %v, want %v", got, exp)
	}
}

func TestMergeScalarUnion(t *testing.T) {
	got := mergeShape(shapeJSON(t, `{"a":"x"}`), shapeJSON(t, `{"a":null}`))
	if !reflect.DeepEqual(got, want(t, `{"a":"null|string"}`)) {
		t.Errorf("got %v", got)
	}
	// urutan tidak berpengaruh
	got2 := mergeShape(shapeJSON(t, `{"a":null}`), shapeJSON(t, `{"a":"x"}`))
	if !reflect.DeepEqual(got, got2) {
		t.Errorf("tidak komutatif: %v vs %v", got, got2)
	}
}

func TestMergeAbsentFields(t *testing.T) {
	got := mergeShape(shapeJSON(t, `{"a":1,"b":"x"}`), shapeJSON(t, `{"a":2}`))
	if !reflect.DeepEqual(got, want(t, `{"a":"number","b":"absent|string"}`)) {
		t.Errorf("got %v", got)
	}
}

func TestMergeNullWithObjectKeepsNestedShape(t *testing.T) {
	// regresi: null lalu objek sempat menghilangkan bentuk objeknya
	obj := shapeJSON(t, `{"r":{"id":"x","n":1}}`)
	nul := shapeJSON(t, `{"r":null}`)
	exp := want(t, `{"r":{"$optional":{"id":"string","n":"number"},"$when":"null"}}`)
	if got := mergeShape(nul, obj); !reflect.DeepEqual(got, exp) {
		t.Errorf("null+objek: %v", got)
	}
	if got := mergeShape(obj, nul); !reflect.DeepEqual(got, exp) {
		t.Errorf("objek+null: %v", got)
	}
	// tiga kali bergabung tetap stabil dan tidak membungkus ganda
	if got := mergeShape(mergeShape(nul, obj), nul); !reflect.DeepEqual(got, exp) {
		t.Errorf("gabungan ulang: %v", got)
	}
	if got := mergeShape(mergeShape(obj, nul), obj); !reflect.DeepEqual(got, exp) {
		t.Errorf("gabungan ulang 2: %v", got)
	}
}

func TestMergeObjectsDeepAndArrays(t *testing.T) {
	a := shapeJSON(t, `{"items":[{"a":1,"b":null}]}`)
	b := shapeJSON(t, `{"items":[{"a":2,"b":"x"}]}`)
	if got := mergeShape(a, b); !reflect.DeepEqual(got, want(t, `{"items":[{"a":"number","b":"null|string"}]}`)) {
		t.Errorf("got %v", got)
	}
	// array kosong tidak menghapus bentuk elemen dari respons lain
	empty := shapeJSON(t, `{"items":[]}`)
	if got := mergeShape(empty, b); !reflect.DeepEqual(got, b) {
		t.Errorf("kosong+isi: %v", got)
	}
	if got := mergeShape(b, empty); !reflect.DeepEqual(got, b) {
		t.Errorf("isi+kosong: %v", got)
	}
}

func TestMergeIdenticalIsIdempotent(t *testing.T) {
	a := shapeJSON(t, `{"a":[{"b":{"c":null}}]}`)
	if got := mergeShape(a, a); !reflect.DeepEqual(got, a) {
		t.Errorf("got %v", got)
	}
}

func TestMergeDistinguishesNullFromAbsentForComposites(t *testing.T) {
	obj := shapeJSON(t, `{"r":{"id":"x"}}`)
	nul := shapeJSON(t, `{"r":null}`)
	gone := shapeJSON(t, `{}`)
	if got := mergeShape(obj, gone); !reflect.DeepEqual(got, want(t, `{"r":{"$optional":{"id":"string"},"$when":"absent"}}`)) {
		t.Errorf("objek+absent: %v", got)
	}
	// null dan absent sekaligus
	both := mergeShape(mergeShape(obj, nul), gone)
	if !reflect.DeepEqual(both, want(t, `{"r":{"$optional":{"id":"string"},"$when":"absent|null"}}`)) {
		t.Errorf("objek+null+absent: %v", both)
	}
	if again := mergeShape(gone, mergeShape(nul, obj)); !reflect.DeepEqual(again, both) {
		t.Errorf("urutan memengaruhi hasil: %v vs %v", again, both)
	}
}
