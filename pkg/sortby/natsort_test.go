package sortby

// Copyright 2018 Marc-Antoine Ruel. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

import (
	"testing"
)

func TestLessLess(t *testing.T) {
	data := [][2]string{
		{"", "a"},
		{"a", "b"},
		{"a", "aa"},
		{"a0", "a1"},
		{"a0", "a00"},
		{"a00", "a01"},
		{"a01", "a2"},
		{"a01x", "a2x"},
		// Only the last number matters.
		{"a0b00", "a00b1"},
		{"a0b00", "a00b01"},
		{"a00b0", "a0b00"},
		{"a00b00", "a0b01"},
		{"a00b00", "a0b1"},
	}
	for _, l := range data {
		if !Less(l[0], l[1]) {
			t.Fatalf("Less(%q, %q) returned false", l[0], l[1])
		}
	}
}

func TestLessNot(t *testing.T) {
	data := [][2]string{
		{"a", ""},
		{"a", "a"},
		{"aa", "a"},
		{"b", "a"},
		{"a01", "a00"},
		{"a01", "a01"},
		{"a1", "a1"},
		{"a2", "a01"},
		{"a2x", "a01x"},
		{"a00b00", "a0b0"},
		{"a00b01", "a0b00"},
		{"a00b00", "a0b00"},
		{"10", "2"},
	}
	for _, l := range data {
		if Less(l[0], l[1]) {
			t.Fatalf("Less(%q, %q) returned true", l[0], l[1])
		}
	}
}

// BenchmarkLessDigitsTwoGroupsNative benchmarks calling a function that just
// does a < b without taking into account the digits, using the same string as
// BenchmarkLessDigitsTwoGroups.
func BenchmarkLessDigitsTwoGroupsNative(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		if less("a01a2", "a01a01") {
			b.Fatal("unexpected result")
		}
	}
}

//go:noinline
func less(a, b string) bool {
	return a < b
}

// BenchmarkLessDigitsTwoGroups compares "a01a2" and "a01a01".
func BenchmarkLessDigitsTwoGroups(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		if Less("a01a2", "a01a01") {
			b.Fatal("unexpected result")
		}
	}
}

// BenchmarkLessStringOnly doesn't contain digits.
func BenchmarkLessStringOnly(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		if Less("abcd", "abc") {
			b.Fatal("unexpected result")
		}
	}
}

// BenchmarkLessDigitsOnly contains only digits.
func BenchmarkLessDigitsOnly(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		if Less("10", "2") {
			b.Fatal("unexpected result")
		}
	}
}

// BenchmarkLess10Blocks benchmark a mix of 10 strings and digits.
func BenchmarkLess10Blocks(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		if Less("a01a01a01a01a01a01a01a01a01a2", "a01a01a01a01a01a01a01a01a01a01") {
			b.Fatal("unexpected result")
		}
	}
}
