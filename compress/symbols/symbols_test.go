package symbols

import (
	"strconv"
	"strings"
	"testing"

	"github.com/lukaszb/gascoigne/detect"
)

const goSrc = `package shop

// Server serves orders.
type Server struct {
	db string
}

type (
	ID   int
	Name string
)

// Run starts serving.
func (s *Server) Run() error {
	return nil
}

func helper() {}
`

const pySrc = `import os

class Cart:
    """A cart."""

    @property
    def total(self):
        return sum(
            i.price for i in self.items
        )

    def add(self, item):
        self.items.append(item)


async def checkout(cart):
    return cart
`

const tsSrc = `import { db } from "./db";

/** Orders service. */
export class Orders {
  private cache = new Map();

  // list returns all orders.
  async list(limit: number): Promise<Order[]> {
    if (limit > 0) {
      return db.query("select } from orders");
    }
    return [];
  }
}

export function total(items: Item[]): number {
  return items.reduce((a, b) => a + b.price, 0);
}

interface Item { price: number }
`

func TestDefs(t *testing.T) {
	tests := []struct {
		name, file, src string
		want            []string // name kind from-to line
	}{
		{"go", "server.go", goSrc, []string{"Server type 3-6 4", "ID type 9-9 9", "Name type 10-10 10", "Server.Run method 13-16 14", "helper func 18-18 18"}},
		{"python", "cart.py", pySrc, []string{"Cart class 3-13 3", "Cart.total method 6-10 7", "Cart.add method 12-13 12", "checkout func 16-17 16"}},
		{"typescript", "orders.ts", tsSrc, []string{"Orders type 3-14 4", "Orders.list method 7-13 8", "total func 16-18 16", "Item type 20-20 20"}},
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

func TestFind(t *testing.T) {
	defs := []Def{{Name: "Server.Run"}, {Name: "Client.Run"}, {Name: "helper"}}
	if d, err := Find(defs, "Server.Run"); err != nil || d.Name != "Server.Run" {
		t.Fatalf("exact: %v %v", d, err)
	}
	if d, err := Find(defs, "helper"); err != nil || d.Name != "helper" {
		t.Fatalf("plain: %v %v", d, err)
	}
	if _, err := Find(defs, "Run"); err == nil || !strings.Contains(err.Error(), "ambiguous: Server.Run, Client.Run") {
		t.Fatalf("ambiguous: %v", err)
	}
	if _, err := Find(defs, "nope"); err == nil {
		t.Fatal("missing: want error")
	}
	if _, err := Defs("SELECT 1;", detect.Hints{File: "q.sql"}); err != ErrLang {
		t.Fatalf("sql: %v", err)
	}
}
