package dataoptimizer

import (
	"strings"
	"testing"
)

func TestNotebook(t *testing.T) {
	nb := `{"cells":[
 {"cell_type":"markdown","metadata":{},"source":"# Title"},
 {"cell_type":"code","execution_count":2,"metadata":{"collapsed":true},"source":["x = 1\n","x"],
  "outputs":[{"output_type":"execute_result","data":{"text/plain":["1"],"image/png":"iVBORw0KGgo="}},
   {"output_type":"stream","name":"stderr","text":"l1\nl2\nl3\nl4\nl5\nl6\nl7\n"},
   {"output_type":"error","ename":"ValueError","evalue":"bad","traceback":["\u001b[0;31mValueError\u001b[0m: bad"]}]},
 {"cell_type":"code","execution_count":null,"metadata":{},"source":"","outputs":[]}
],"metadata":{"kernelspec":{"name":"python3"}},"nbformat":4,"nbformat_minor":5}`
	want := `# notebook, kernel python3
# %% [1] markdown
# Title
# %% [2] code In[2]
x = 1
x
# out:
1
[image/png 14 B]
# out: stderr
l1
l2
l3
… 2 lines omitted
l6
l7
# error: ValueError: bad
ValueError: bad
# %% [3] code
`
	o := &Optimizer{cfg: &Config{MaxOutputLines: 5}}
	got, err := o.notebook(nb)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("got\n%s\nwant\n%s", got, want)
	}
	if _, err := o.notebook("{not json"); err == nil || !strings.Contains(err.Error(), "notebook") {
		t.Fatalf("bad notebook: err %v", err)
	}
}
