// Copyright ©2017 The go-hep Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fmom_test

import (
	"math"
	"testing"

	"go-hep.org/x/hep/fmom"
)

func TestPtEtaPhiM(t *testing.T) {
	{
		var p4 fmom.PtEtaPhiM
		if got, want := p4.Pt(), 0.0; got != want {
			t.Fatalf("p4.Pt=%v, want=%v", got, want)
		}
		if got, want := p4.Eta(), 0.0; got != want {
			t.Fatalf("p4.Eta=%v, want=%v", got, want)
		}
		if got, want := p4.Phi(), 0.0; got != want {
			t.Fatalf("p4.Phi=%v, want=%v", got, want)
		}
		if got, want := p4.M(), 0.0; got != want {
			t.Fatalf("p4.M=%v, want=%v", got, want)
		}
		if got, want := p4.String(), "fmom.P4{Pt:0, Eta:0, Phi:0, M:0}"; got != want {
			t.Fatalf("p4=%v, want=%v", got, want)
		}
	}

	{
		p4 := fmom.NewPtEtaPhiM(10, 11, 12, 20)
		if got, want := p4.Pt(), 10.0; got != want {
			t.Fatalf("p4.Pt=%v, want=%v", got, want)
		}
		if got, want := p4.Eta(), 11.0; got != want {
			t.Fatalf("p4.Eta=%v, want=%v", got, want)
		}
		if got, want := p4.Phi(), 12.0; got != want {
			t.Fatalf("p4.Phi=%v, want=%v", got, want)
		}
		if got, want := p4.M(), 20.0; got != want {
			t.Fatalf("p4.M=%v, want=%v", got, want)
		}
		if got, want := p4.String(), "fmom.P4{Pt:10, Eta:11, Phi:12, M:20}"; got != want {
			t.Fatalf("p4=%v, want=%v", got, want)
		}

		p1 := fmom.NewPtEtaPhiM(10, 11, 12, 20)
		if p1 != p4 {
			t.Fatalf("p4=%v, want=%v", p1, p4)
		}

		var p2 fmom.PtEtaPhiM = p1
		if p1 != p2 {
			t.Fatalf("p4=%v, want=%v", p1, p2)
		}
	}

	{
		p1 := fmom.NewPtEtaPhiM(10, 11, 12, 20)
		var p2 fmom.PtEtaPhiM
		p2.Set(&p1)
		if p1 != p2 {
			t.Fatalf("p4=%v want=%v", p2, p1)
		}
	}

	ref := fmom.NewPxPyPzE(10, 11, 12, 20)
	var p fmom.PtEtaPhiM
	p.Set(&ref)

	for i, v := range []float64{
		math.Abs(p.Pt() - ref.Pt()),
		math.Abs(p.Eta() - ref.Eta()),
		math.Abs(p.Phi() - ref.Phi()),
		math.Abs(p.M() - ref.M()),

		math.Abs(math.Atan2(p.SinPhi(), p.CosPhi()) - p.Phi()),
		math.Abs(p.SinPhi()*p.SinPhi() + p.CosPhi()*p.CosPhi() - 1.0),
		math.Abs(-math.Log(math.Tan(math.Atan(p.TanTh())*0.5)) - p.Eta()),
	} {
		if v > epsilon {
			t.Fatalf("test [%d]: value out of tolerance", i)
		}

	}
}
