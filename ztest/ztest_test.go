package ztest

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestShouldSkip(t *testing.T) {
	assert.Equal(t, "script test on in-process run", (&ZTest{Script: "x"}).ShouldSkip(""))
	assert.Equal(t, "reason", (&ZTest{Skip: "reason"}).ShouldSkip(""))
	assert.Equal(t, `tag "x" does not match ZTEST_TAG=""`, (&ZTest{Tag: "x"}).ShouldSkip(""))
}
