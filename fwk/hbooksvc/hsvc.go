// Copyright ©2017 The go-hep Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package hbooksvc // import "go-hep.org/x/hep/fwk/hbooksvc"

import (
	"fmt"
	"os"
	"reflect"
	"strings"
	"sync"

	"go-hep.org/x/hep/fwk"
	"go-hep.org/x/hep/fwk/fsm"
	"go-hep.org/x/hep/hbook"
	"go-hep.org/x/hep/rio"
)

type h1d struct {
	fwk.H1D
	mu sync.RWMutex
}

type h2d struct {
	fwk.H2D
	mu sync.RWMutex
}

type p1d struct {
	fwk.P1D
	mu sync.RWMutex
}

type s2d struct {
	fwk.S2D
	mu sync.RWMutex
}

type hsvc struct {
	fwk.SvcBase

	h1ds map[fwk.HID]*h1d
	h2ds map[fwk.HID]*h2d
	p1ds map[fwk.HID]*p1d
	s2ds map[fwk.HID]*s2d

	streams map[string]Stream
	w       map[string]ostream
	r       map[string]istream
}

func (svc *hsvc) Configure(ctx fwk.Context) error {
	var err error

	return err
}

func (svc *hsvc) StartSvc(ctx fwk.Context) error {
	var err error

	for name, stream := range svc.streams {
		switch stream.Mode {
		case Read:
			_, dup := svc.r[name]
			if dup {
				return fmt.Errorf("%s: duplicate read-stream %q", svc.Name(), name)
			}
			// FIXME(sbinet): handle remote/local files + protocols
			f, err := os.Open(stream.Name)
			if err != nil {
				return fmt.Errorf("error opening file [%s]: %w", stream.Name, err)
			}
			r, err := rio.NewReader(f)
			if err != nil {
				return fmt.Errorf("error opening rio-stream [%s]: %w", stream.Name, err)
			}

			svc.r[name] = istream{
				name:  name,
				fname: stream.Name,
				f:     f,
				r:     r,
			}

		case Write:
			_, dup := svc.w[name]
			if dup {
				return fmt.Errorf("%s: duplicate write-stream %q", svc.Name(), name)
			}
			// FIXME(sbinet): handle remote/local files + protocols
			f, err := os.Create(stream.Name)
			if err != nil {
				return fmt.Errorf("error creating file [%s]: %w", stream.Name, err)
			}
			w, err := rio.NewWriter(f)
			if err != nil {
				return fmt.Errorf("error creating rio-stream [%s]: %w", stream.Name, err)
			}

			svc.w[name] = ostream{
				name:  name,
				fname: stream.Name,
				f:     f,
				w:     w,
			}

		default:
			return fmt.Errorf("%s: invalid stream mode (%d)", svc.Name(), stream.Mode)
		}
	}
	return err
}

func (svc *hsvc) StopSvc(ctx fwk.Context) error {
	var err error

	errs := make([]error, 0, len(svc.r)+len(svc.w))

	// closing write-streams
	for n, w := range svc.w {

		werr := w.write()
		if werr != nil {
			errs = append(errs, fmt.Errorf("error flushing %q: %w", n, werr))
		}

		werr = w.close()
		if werr != nil {
			errs = append(errs, fmt.Errorf("error closing %q: %w", n, werr))
		}
	}

	// closing read-streams
	for n, r := range svc.r {

		rerr := r.close()
		if rerr != nil {
			errs = append(errs, fmt.Errorf("error closing %q: %w", n, rerr))
		}
	}

	if len(errs) > 0 {
		// FIXME(sbinet): return the complete list instead of the first one.
		//                use an errlist.Error ?
		return errs[0]
	}
	return err
}

func (svc *hsvc) BookH1D(name string, nbins int, low, high float64) (fwk.H1D, error) {
	var err error
	var h fwk.H1D

	if !(fsm.Configured < svc.FSMState() && svc.FSMState() < fsm.Running) {
		return h, fmt.Errorf("fwk: can not book histograms during FSM-state %v", svc.FSMState())
	}

	stream, hid := svc.split(name)
	h = fwk.H1D{
		ID:   fwk.HID(hid),
		Hist: hbook.NewH1D(nbins, low, high),
	}
	h.Hist.Annotation()["name"] = svc.fullname(stream, hid)

	switch stream {
	case "":
		// ok, temporary histo.
	default:
		sname := "/" + stream
		str, ok := svc.streams[sname]
		if !ok {
			return h, fmt.Errorf("fwk: no stream [%s] declared", sname)
		}
		switch str.Mode {
		case Read:
			r, ok := svc.r[sname]
			if !ok {
				return h, fmt.Errorf("fwk: no read-stream [%s] declared", sname)
			}
			err = r.read(hid, h.Hist)
			if err != nil {
				return h, err
			}

			r.objs = append(r.objs, h)
			svc.r[sname] = r

		case Write:
			w, ok := svc.w[sname]
			if !ok {
				return h, fmt.Errorf("fwk: no write-stream [%s] declared: %v", sname, svc.w)
			}
			w.objs = append(w.objs, h)
			svc.w[sname] = w
		default:
			return h, fmt.Errorf("%s: invalid stream mode (%d)", svc.Name(), str.Mode)
		}
	}

	hh := &h1d{H1D: h}
	svc.h1ds[h.ID] = hh
	return hh.H1D, err
}

func (svc *hsvc) BookH2D(name string, nx int, xmin, xmax float64, ny int, ymin, ymax float64) (fwk.H2D, error) {
	var err error
	var h fwk.H2D

	if !(fsm.Configured < svc.FSMState() && svc.FSMState() < fsm.Running) {
		return h, fmt.Errorf("fwk: can not book histograms during FSM-state %v", svc.FSMState())
	}

	stream, hid := svc.split(name)
	h = fwk.H2D{
		ID:   fwk.HID(hid),
		Hist: hbook.NewH2D(nx, xmin, xmax, ny, ymin, ymax),
	}
	h.Hist.Annotation()["name"] = svc.fullname(stream, hid)

	switch stream {
	case "":
		// ok, temporary histo.
	default:
		sname := "/" + stream
		str, ok := svc.streams[sname]
		if !ok {
			return h, fmt.Errorf("fwk: no stream [%s] declared", sname)
		}
		switch str.Mode {
		case Read:
			r, ok := svc.r[sname]
			if !ok {
				return h, fmt.Errorf("fwk: no read-stream [%s] declared", sname)
			}
			err = r.read(hid, h.Hist)
			if err != nil {
				return h, err
			}

			r.objs = append(r.objs, h)
			svc.r[sname] = r

		case Write:
			w, ok := svc.w[sname]
			if !ok {
				return h, fmt.Errorf("fwk: no write-stream [%s] declared: %v", sname, svc.w)
			}
			w.objs = append(w.objs, h)
			svc.w[sname] = w
		default:
			return h, fmt.Errorf("%s: invalid stream mode (%d)", svc.Name(), str.Mode)
		}
	}

	hh := &h2d{H2D: h}
	svc.h2ds[h.ID] = hh
	return hh.H2D, err
}

func (svc *hsvc) BookP1D(name string, nbins int, low, high float64) (fwk.P1D, error) {
	var err error
	var h fwk.P1D

	if !(fsm.Configured < svc.FSMState() && svc.FSMState() < fsm.Running) {
		return h, fmt.Errorf("fwk: can not book histograms during FSM-state %v", svc.FSMState())
	}

	stream, hid := svc.split(name)
	h = fwk.P1D{
		ID:      fwk.HID(hid),
		Profile: hbook.NewP1D(nbins, low, high),
	}
	h.Profile.Annotation()["name"] = svc.fullname(stream, hid)

	switch stream {
	case "":
		// ok, temporary histo.
	default:
		sname := "/" + stream
		str, ok := svc.streams[sname]
		if !ok {
			return h, fmt.Errorf("fwk: no stream [%s] declared", sname)
		}
		switch str.Mode {
		case Read:
			r, ok := svc.r[sname]
			if !ok {
				return h, fmt.Errorf("fwk: no read-stream [%s] declared", sname)
			}
			err = r.read(hid, h.Profile)
			if err != nil {
				return h, err
			}

			r.objs = append(r.objs, h)
			svc.r[sname] = r

		case Write:
			w, ok := svc.w[sname]
			if !ok {
				return h, fmt.Errorf("fwk: no write-stream [%s] declared: %v", sname, svc.w)
			}
			w.objs = append(w.objs, h)
			svc.w[sname] = w
		default:
			return h, fmt.Errorf("%s: invalid stream mode (%d)", svc.Name(), str.Mode)
		}
	}

	hh := &p1d{P1D: h}
	svc.p1ds[h.ID] = hh
	return hh.P1D, err
}

func (svc *hsvc) BookS2D(name string) (fwk.S2D, error) {
	var err error
	var h fwk.S2D

	if !(fsm.Configured < svc.FSMState() && svc.FSMState() < fsm.Running) {
		return h, fmt.Errorf("fwk: can not book histograms during FSM-state %v", svc.FSMState())
	}

	stream, hid := svc.split(name)
	h = fwk.S2D{
		ID:      fwk.HID(hid),
		Scatter: hbook.NewS2D(),
	}
	h.Scatter.Annotation()["name"] = svc.fullname(stream, hid)

	switch stream {
	case "":
		// ok, temporary histo.
	default:
		sname := "/" + stream
		str, ok := svc.streams[sname]
		if !ok {
			return h, fmt.Errorf("fwk: no stream [%s] declared", sname)
		}
		switch str.Mode {
		case Read:
			r, ok := svc.r[sname]
			if !ok {
				return h, fmt.Errorf("fwk: no read-stream [%s] declared", sname)
			}
			err = r.read(hid, h.Scatter)
			if err != nil {
				return h, err
			}

			r.objs = append(r.objs, h)
			svc.r[sname] = r

		case Write:
			w, ok := svc.w[sname]
			if !ok {
				return h, fmt.Errorf("fwk: no write-stream [%s] declared: %v", sname, svc.w)
			}
			w.objs = append(w.objs, h)
			svc.w[sname] = w
		default:
			return h, fmt.Errorf("%s: invalid stream mode (%d)", svc.Name(), str.Mode)
		}
	}

	hh := &s2d{S2D: h}
	svc.s2ds[h.ID] = hh
	return hh.S2D, err
}

func (svc *hsvc) fullname(stream, hid string) string {
	if stream == "" {
		return hid
	}
	return stream + "/" + hid
}

// split splits a booking histo name into (stream-name, histo-name).
//
// eg: "/my-stream/histo" -> ("my-stream", "histo")
//     "my-stream/histo"  -> ("my-stream", "histo")
//     "my-stream/histo/" -> ("my-stream", "histo")
//     "/histo"           -> ("",          "histo")
//     "histo"            -> ("",          "histo")
func (svc *hsvc) split(n string) (string, string) {

	n = strings.TrimPrefix(n, "/")
	n = strings.TrimSuffix(n, "/")

	o := strings.Split(n, "/")
	switch len(o) {
	case 0:
		panic("impossible")
	case 1:
		return "", o[0]
	case 2:
		return o[0], o[1]
	default:
		return o[0], strings.Join(o[1:], "/")
	}
}

func (svc *hsvc) FillH1D(id fwk.HID, x, w float64) {
	h := svc.h1ds[id]
	h.mu.Lock()
	h.Hist.Fill(x, w)
	h.mu.Unlock()
}

func (svc *hsvc) FillH2D(id fwk.HID, x, y, w float64) {
	h := svc.h2ds[id]
	h.mu.Lock()
	h.Hist.Fill(x, y, w)
	h.mu.Unlock()
}

func (svc *hsvc) FillP1D(id fwk.HID, x, y, w float64) {
	h := svc.p1ds[id]
	h.mu.Lock()
	h.Profile.Fill(x, y, w)
	h.mu.Unlock()
}

func (svc *hsvc) FillS2D(id fwk.HID, x, y float64) {
	h := svc.s2ds[id]
	h.mu.Lock()
	// FIXME(sbinet): weight?
	h.Scatter.Fill(hbook.Point2D{X: x, Y: y})
	h.mu.Unlock()
}

func newhsvc(typ, name string, mgr fwk.App) (fwk.Component, error) {
	var err error
	svc := &hsvc{
		SvcBase: fwk.NewSvc(typ, name, mgr),
		streams: map[string]Stream{},
		w:       map[string]ostream{},
		r:       map[string]istream{},
		h1ds:    make(map[fwk.HID]*h1d),
		h2ds:    make(map[fwk.HID]*h2d),
		p1ds:    make(map[fwk.HID]*p1d),
		s2ds:    make(map[fwk.HID]*s2d),
	}

	err = svc.DeclProp("Streams", &svc.streams)
	if err != nil {
		return nil, err
	}
	return svc, err
}

func init() {
	fwk.Register(reflect.TypeOf(hsvc{}), newhsvc)
}

var _ fwk.HistSvc = (*hsvc)(nil)
