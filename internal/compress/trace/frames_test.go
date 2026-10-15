package trace

import (
	"slices"
	"strings"
	"testing"
)

func TestFrames(t *testing.T) {
	tests := []struct {
		name, in, want string
	}{
		{"jvm", `java.lang.IllegalStateException: boom
	at shop.Cart.total(Cart.scala:41)
	at scala.collection.immutable.List.foreach(List.scala:333)
	at scala.collection.IterableOnceOps.foreach(IterableOnce.scala:575)
	at cats.effect.IOFiber.runLoop(IOFiber.scala:1000)
	at java.base/java.lang.Thread.run(Thread.java:840)
	at shop.Main$.main(Main.scala:9)`, `java.lang.IllegalStateException: boom
	at shop.Cart.total(Cart.scala:41)
… 4 framework stack frames omitted
	at shop.Main$.main(Main.scala:9)`},
		{"jvm keeps the throw site", `java.lang.NullPointerException
	at java.base/java.util.Objects.requireNonNull(Objects.java:233)
	at java.base/java.util.Optional.of(Optional.java:113)
	at java.base/java.util.stream.ReferencePipeline.forEach(ReferencePipeline.java:100)
	at java.base/java.lang.Thread.run(Thread.java:840)
	at shop.Cart.total(Cart.java:41)`, `java.lang.NullPointerException
	at java.base/java.util.Objects.requireNonNull(Objects.java:233)
… 3 framework stack frames omitted
	at shop.Cart.total(Cart.java:41)`},
		{"short run stays", `Exception in thread "main" java.lang.Error
	at shop.A.f(A.java:1)
	at java.base/java.lang.Thread.run(Thread.java:840)
	at java.base/java.lang.Thread.start(Thread.java:841)
	at shop.B.g(B.java:2)`, `Exception in thread "main" java.lang.Error
	at shop.A.f(A.java:1)
	at java.base/java.lang.Thread.run(Thread.java:840)
	at java.base/java.lang.Thread.start(Thread.java:841)
	at shop.B.g(B.java:2)`},
		{"rust", `thread 'main' panicked at src/lru.rs:41:9:
index out of bounds
stack backtrace:
   0: rust_begin_unwind
             at /rustc/abc/library/std/src/panicking.rs:645:5
   1: core::panicking::panic_fmt
             at /rustc/abc/library/core/src/panicking.rs:72:14
   2: core::panicking::panic_bounds_check
             at /rustc/abc/library/core/src/panicking.rs:208:5
   3: shop::lru::Lru<K,V>::get
             at ./src/lru.rs:41:9
   4: std::rt::lang_start::{{closure}}
             at /rustc/abc/library/std/src/rt.rs:166:18`, `thread 'main' panicked at src/lru.rs:41:9:
index out of bounds
stack backtrace:
… 3 framework stack frames omitted
   3: shop::lru::Lru<K,V>::get
             at ./src/lru.rs:41:9
   4: std::rt::lang_start::{{closure}}
             at /rustc/abc/library/std/src/rt.rs:166:18`},
		{"python", `Traceback (most recent call last):
  File "/home/dev/shop/app.py", line 10, in main
    run()
  File "/usr/lib/python3.12/site-packages/click/core.py", line 1157, in __call__
    return self.main(*args, **kwargs)
  File "/usr/lib/python3.12/site-packages/click/core.py", line 1078, in main
    rv = self.invoke(ctx)
  File "/usr/lib/python3.12/site-packages/click/core.py", line 1434, in invoke
    return ctx.invoke(self.callback, **ctx.params)
ValueError: bad`, `Traceback (most recent call last):
  File "/home/dev/shop/app.py", line 10, in main
    run()
… 3 framework stack frames omitted
ValueError: bad`},
		{"asan", `==1234==ERROR: AddressSanitizer: heap-use-after-free
    #0 0x55 in shop::Cart::total() /src/cart.cc:41
    #1 0x56 in std::__invoke_impl<void> /usr/include/c++/13/bits/invoke.h:61
    #2 0x57 in std::thread::_State_impl::_M_run /usr/include/c++/13/bits/std_thread.h:253
    #3 0x58 in std::this_thread::sleep_for /usr/include/c++/13/bits/this_thread_sleep.h:1
    #4 0x59 in main /src/main.cc:9`, `==1234==ERROR: AddressSanitizer: heap-use-after-free
    #0 0x55 in shop::Cart::total() /src/cart.cc:41
… 3 framework stack frames omitted
    #4 0x59 in main /src/main.cc:9`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Frames(strings.Split(tt.in, "\n"), 3)
			if want := strings.Split(tt.want, "\n"); !slices.Equal(got, want) {
				t.Errorf("got:\n%s\nwant:\n%s", strings.Join(got, "\n"), tt.want)
			}
			// idempotent: a second pass changes nothing
			if again := Frames(got, 3); !slices.Equal(again, got) {
				t.Errorf("not idempotent:\n%s", strings.Join(again, "\n"))
			}
		})
	}
}
