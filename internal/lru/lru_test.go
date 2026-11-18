package lru

import (
	"sync"
	"testing"
)

func TestEvictsLeastRecentlyUsed(t *testing.T) {
	c := New[string, int](2)
	c.Put("a", 1)
	c.Put("b", 2)
	if v, ok := c.Get("a"); !ok || v != 1 { // a is now the most recent
		t.Fatalf("Get a = %d %v", v, ok)
	}
	c.Put("c", 3)
	if _, ok := c.Get("b"); ok {
		t.Error("b should be evicted")
	}
	for _, k := range []string{"a", "c"} {
		if _, ok := c.Get(k); !ok {
			t.Errorf("%s missing", k)
		}
	}
	if c.Len() != 2 {
		t.Errorf("Len = %d", c.Len())
	}
}

func TestPutReplacesAndRefreshes(t *testing.T) {
	c := New[string, string](2)
	c.Put("a", "x")
	c.Put("b", "y")
	c.Put("a", "z")
	c.Put("c", "w")
	if v, ok := c.Get("a"); !ok || v != "z" {
		t.Errorf("Get a = %q %v", v, ok)
	}
	if _, ok := c.Get("b"); ok {
		t.Error("b should be evicted")
	}
}

func TestMissReturnsZero(t *testing.T) {
	c := New[string, int](1)
	if v, ok := c.Get("x"); ok || v != 0 {
		t.Errorf("Get = %d %v", v, ok)
	}
}

func TestConcurrent(t *testing.T) {
	c := New[int, int](8)
	var wg sync.WaitGroup
	for g := range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range 200 {
				c.Put(i%16+g, i)
				c.Get(i % 16)
			}
		}()
	}
	wg.Wait()
	if c.Len() > 8 {
		t.Errorf("Len = %d", c.Len())
	}
}
