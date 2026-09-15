package cli

import (
	"github.com/neko233-com/game-numerical-design-cli/internal/curves"
	"github.com/neko233-com/game-numerical-design-cli/internal/htmlreport"
)

func sampleCurveImpl(kind string, maxX float64, n int, a, b, s1, s2, brk, l, k, x0 float64) ([]struct{ X, Y float64 }, error) {
	p := curves.Params{
		A: a, B: b, S1: s1, S2: s2, BreakX: brk,
		L: l, K: k, X0: x0, Base: 0,
	}
	pts, err := curves.Sample(curves.Kind(kind), p, maxX, n)
	if err != nil {
		return nil, err
	}
	out := make([]struct{ X, Y float64 }, len(pts))
	for i, pt := range pts {
		out[i].X = pt.X
		out[i].Y = pt.Y
	}
	return out, nil
}

var _ = htmlreport.Point{}
