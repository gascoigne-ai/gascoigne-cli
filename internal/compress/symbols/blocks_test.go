package symbols

import (
	"strconv"
	"strings"
	"testing"

	"github.com/gascoigne-ai/gascoigne-cli/internal/detect"
)

const rbSrc = `# frozen_string_literal: true

module Shop
  # Cart holds items.
  class Cart
    attr_reader :items

    def initialize
      @items = []
    end

    # total sums prices.
    def total
      items.sum do |i|
        i.price
      end
    end

    def self.build(list) = new.tap { |c| list.each { |i| c.add(i) } }

    def empty?; items.empty?; end

    private

    def add(item)
      @items << item if item
    end
  end
end
`

const exSrc = `defmodule Shop.Cart do
  @moduledoc "A cart."

  @doc """
  Adds an item.
  """
  @spec add(list, term) :: list
  def add(items, item) do
    if item do
      [item | items]
    else
      items
    end
  end

  def total(items), do: Enum.sum(items)

  defp price(item) do
    item.price
  end
end
`

const luaSrc = `local M = {}

-- add appends an item.
function M.add(cart, item)
  if item then
    table.insert(cart, item)
  end
end

local function price(item)
  return item.price
end

function M:total()
  return 0
end

return M
`

const mdSrc = "# Shop\n\nIntro.\n\n## Install\n\n```sh\n# not a heading\nmake build\n```\n\n## Usage\n\n### Flags\n\nText.\n\n## License\n"

// Ruby, Elixir, Lua blocks close with end; Markdown has headings only.
func TestDefsBlocks(t *testing.T) {
	tests := []struct {
		name, file, src string
		want            []string // name kind from-to line
	}{
		{"ruby", "cart.rb", rbSrc, []string{
			"Shop type 3-29 3", "Shop.Cart type 4-28 5", "Shop.Cart.initialize method 8-10 8", "Shop.Cart.total method 12-17 13",
			"Shop.Cart.build method 19-19 19", "Shop.Cart.empty? method 21-21 21", "Shop.Cart.add method 25-27 25"}},
		{"elixir", "cart.ex", exSrc, []string{"Shop.Cart type 1-21 1", "Shop.Cart.add method 4-14 8", "Shop.Cart.total method 16-16 16", "Shop.Cart.price method 18-20 18"}},
		{"lua", "cart.lua", luaSrc, []string{"M.add func 3-8 4", "price func 10-12 10", "M:total func 14-16 14"}},
		{"markdown", "README.md", mdSrc, []string{"Shop section 1-18 1", "Install section 5-10 5", "Usage section 12-16 12", "Flags section 14-16 14", "License section 18-18 18"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defs, err := Defs(tt.src, detect.Hints{File: tt.file})
			if err != nil {
				t.Fatal(err)
			}
			var got []string
			for _, d := range defs {
				got = append(got, strings.Join([]string{d.Name, d.Kind, strconv.Itoa(d.From) + "-" + strconv.Itoa(d.To), strconv.Itoa(d.Line)}, " "))
			}
			if strings.Join(got, "\n") != strings.Join(tt.want, "\n") {
				t.Fatalf("got\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(tt.want, "\n"))
			}
		})
	}
}
