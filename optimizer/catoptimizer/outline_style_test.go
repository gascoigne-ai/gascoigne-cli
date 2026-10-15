package catoptimizer

import (
	"strings"
	"testing"

	"github.com/gascoigne-ai/gascoigne-cli/internal/detect"
)

// Outlines of languages that close blocks with end, use indentation under a
// declaration, or balance parentheses.
func TestStyleOutlines(t *testing.T) {
	tests := []struct {
		name, file, src string
		keep            []string
	}{
		{"ruby", "a.rb", `module Shop
  class Cart
    def total
      sum = 0
      items.each do |i|
        sum += i.price
      end
      sum
    end

    private def hidden
      a = if ready
        1
      else
        2
      end
      a
    end
  end
end
`, []string{"module Shop", "class Cart", "def total", "private def hidden", "  end", "end"}},
		{"elixir", "a.ex", `defmodule Shop.Cart do
  def total(items) do
    items
    |> Enum.map(fn i ->
      i.price
    end)
    |> Enum.sum()
  end

  def one(x), do: x

  defp helper(x) do
    y = case x do
      1 -> :a
      _ -> :b
    end
    y
  end
end
`, []string{"defmodule Shop.Cart do", "def total(items) do", "def one(x), do: x", "defp helper(x) do"}},
		{"lua", "a.lua", `local M = {}

function M.total(items)
  local sum = 0
  for _, i in ipairs(items) do
    sum = sum + i.price
  end
  return sum
end

local function helper(x)
  if x then
    return 1
  end
  return 2
end

return M
`, []string{"function M.total(items)", "local function helper(x)", "return M"}},
		{"haskell", "a.hs", `module Shop where

import Data.List (sort)

total :: [Int] -> Int
total xs = go 0 xs
  where
    go acc [] = acc
    go acc (y:ys) = go (acc + y) ys

class Priced a where
  price :: a -> Int
  discounted :: a -> Int
  discounted x =
    let p = price x
        d = p - 1
    in p - d
`, []string{"module Shop where", "total :: [Int] -> Int", "total xs = go 0 xs", "class Priced a where", "discounted x ="}},
		{"clojure", "a.clj", `(ns shop.cart)

(defn total
  "Sums prices."
  [items]
  (let [prices (map :price items)
        sum (reduce + prices)]
    (println "total" sum)
    sum))

(def tax 0.2)
`, []string{"(ns shop.cart)", "(defn total", "    sum))", "(def tax 0.2)"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := detect.Hints{File: tt.file}
			lang := detect.Lang(tt.src, h)
			got := opt(Outline).rewrite(tt.src, lang, h, Outline)
			if !strings.Contains(got, "… L") {
				t.Errorf("no body elided:\n%s", got)
			}
			for _, want := range tt.keep {
				if !strings.Contains(got, want) {
					t.Errorf("missing %q in:\n%s", want, got)
				}
			}
		})
	}
}
