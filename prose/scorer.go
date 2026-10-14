package prose

import (
	"cmp"
	"errors"
	"fmt"
	"runtime"

	"github.com/gascoigne-ai/gascoigne-cli/prose/ortrt"
)

// defaultLib is looked up on the system library path when no path is given.
var defaultLib = map[string]string{"darwin": "libonnxruntime.dylib", "windows": "onnxruntime.dll"}

// ortScorer runs the model's graph: input_ids and attention_mask in, a keep
// score per token out: final_scores [batch, seq] (keep probability times span
// weight), or logits [batch, seq, 2] (drop, keep) that become probabilities.
type ortScorer struct {
	rt      *ortrt.Runtime
	session *ortrt.Session
	device  string
	logits  bool
}

func newORT(model, lib, device string, out ortrt.Output) (*ortScorer, error) {
	rt, err := ortrt.Open(cmp.Or(lib, defaultLib[runtime.GOOS], "libonnxruntime.so"))
	if err != nil {
		return nil, fmt.Errorf("onnx runtime: %w", err)
	}
	var errs []error
	for _, d := range candidates(device) {
		s, err := open(rt, model, d, out)
		if err == nil {
			return &ortScorer{rt: rt, session: s, device: d, logits: out.Width == 2}, nil
		}
		errs = append(errs, fmt.Errorf("%s: %w", d, err))
	}
	rt.Close()
	return nil, errors.Join(errs...)
}

// candidates lists the devices to try in order; the CPU is the last resort
// unless the user asked for one device.
func candidates(device string) []string {
	if device != "auto" {
		return []string{device}
	}
	if runtime.GOOS == "darwin" {
		return []string{"coreml", "cpu"}
	}
	return []string{"cuda", "cpu"}
}

func open(rt *ortrt.Runtime, model, device string, out ortrt.Output) (*ortrt.Session, error) {
	s, err := rt.NewSession(model, device, out)
	if err != nil {
		return nil, err
	}
	// Providers can accept a session and fail on the first run: prove it works.
	if _, err := s.Scores([]int64{0, 0}); err != nil {
		s.Close()
		return nil, fmt.Errorf("first run: %w", err)
	}
	return s, nil
}

func (s *ortScorer) Scores(ids []int64) ([]float32, error) {
	scores, err := s.session.Scores(ids)
	if err != nil || !s.logits {
		return scores, err
	}
	return keepProb(scores), nil
}

func (s *ortScorer) close() {
	s.session.Close()
	s.rt.Close()
}
