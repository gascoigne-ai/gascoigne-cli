package catoptimizer

import (
	"testing"

	"github.com/gascoigne-ai/gascoigne-cli/internal/lazyre"
)

func TestDirectiveRulesVerify(t *testing.T) {
	if err := directive.Verify(); err != nil {
		t.Fatal(err)
	}
}

// The regexp directive replaced, kept to pin the old behavior.
var oldDirective = lazyre.New(`^(#!|//go:|//\s*\+build|//\s*(nolint|NOLINT|eslint|prettier|biome|istanbul|c8|@ts-|#region|#endregion|swiftlint|clang-format)|///\s*<reference|#\s*(type:|noqa|pragma|pylint|mypy|pyright|fmt:|-\*-|frozen_string_literal|encoding|rubocop|shellcheck)|--\s*(luacheck|selene))`)

func TestDirectiveSameAsRegexp(t *testing.T) {
	var examples []string
	for _, r := range directive {
		examples = append(examples, r.Yes...)
		examples = append(examples, r.No...)
	}
	examples = append(examples, "// nolint", "//\tnolint:gosec", "///reference", "/// <reference path=x/>", "#   noqa", "#noqa", "--selene: allow(x)", "-- luacheck: ignore",
		"// +build linux", "//+build linux", "//  #region x", "#!/bin/sh", "# -*- coding: utf-8 -*-", "///nolint", "//// <reference", "#!", "//go:", "# type: ignore",
		"// eslint-disable-line", "// c8 ignore next", "// @ts-ignore", "# fmt: off", "# frozen_string_literal: true", "# shellcheck disable=SC2086")
	sameAsOld(t, oldDirective.MatchString, directive.Match, examples)
}
