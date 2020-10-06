// Copyright ©2018 The go-hep Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package rsrv

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	stdpath "path"
	"path/filepath"
	"sort"
	"strings"

	uuid "github.com/hashicorp/go-uuid"
	"go-hep.org/x/hep/groot/rhist"
	"go-hep.org/x/hep/groot/riofs"
	"go-hep.org/x/hep/groot/root"
	"go-hep.org/x/hep/groot/rtree"
	"go-hep.org/x/hep/hbook"
	"go-hep.org/x/hep/hbook/rootcnv"
	"go-hep.org/x/hep/hplot"
)

// Ping verifies the connection to the server is alive.
// Ping replies with a StatusOK.
func (srv *Server) Ping(w http.ResponseWriter, r *http.Request) {
	srv.wrap(srv.handlePing)(w, r)
}

func (srv *Server) handlePing(w http.ResponseWriter, r *http.Request) error {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	return json.NewEncoder(w).Encode(nil)
}

// OpenFile opens a ROOT file located at the provided URI.
// OpenFile expects an OpenFileRequest payload as JSON:
//   {"uri": "file:///some/file.root"}
//   {"uri": "root://example.org/some/file.root"}
//
// OpenFile replies with a STATUS/OK or STATUS/NotFound if no such file exist.
func (srv *Server) OpenFile(w http.ResponseWriter, r *http.Request) {
	srv.wrap(srv.handleOpen)(w, r)
}

func (srv *Server) handleOpen(w http.ResponseWriter, r *http.Request) error {
	dec := json.NewDecoder(r.Body)
	defer r.Body.Close()

	var req OpenFileRequest

	err := dec.Decode(&req)
	if err != nil {
		return fmt.Errorf("could not decode open-file request: %w", err)
	}

	db, err := srv.db(r)
	if err != nil {
		return fmt.Errorf("could not open file database: %w", err)
	}

	if f := db.get(req.URI); f != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		return json.NewEncoder(w).Encode(nil)
	}

	f, err := riofs.Open(req.URI)
	if err != nil {
		return fmt.Errorf("could not open ROOT file: %w", err)
	}

	db.set(req.URI, f)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	return json.NewEncoder(w).Encode(nil)
}

// UploadFile uploads a ROOT file, provided as a multipart form data under
// the key "groot-file", to the remote server.
// The destination of that ROOT file is also taken from the multipart form,
// under the key "groot-dst".
//
// UploadFile replies with a StatusConflict if a file with the named file
// already exists in the remote server.
func (srv *Server) UploadFile(w http.ResponseWriter, r *http.Request) {
	srv.wrap(srv.handleUpload)(w, r)
}

func (srv *Server) handleUpload(w http.ResponseWriter, r *http.Request) error {
	err := r.ParseMultipartForm(500 << 20)
	if err != nil {
		return fmt.Errorf("could not parse multipart form: %w", err)
	}

	const (
		destKey = "groot-dst"
		fileKey = "groot-file"
	)

	dst := r.FormValue(destKey)
	if dst == "" {
		return fmt.Errorf("empty destination for uploaded ROOT file")
	}

	db, err := srv.db(r)
	if err != nil {
		return fmt.Errorf("could not open file database: %w", err)
	}

	if f := db.get(dst); f != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		return json.NewEncoder(w).Encode(nil)
	}

	f, handler, err := r.FormFile(fileKey)
	if err != nil {
		return fmt.Errorf("could not retrieve ROOT file from multipart form: %w", err)
	}

	fid, err := uuid.GenerateUUID()
	if err != nil {
		return fmt.Errorf("could not generate UUID for %q: %w", handler.Filename, err)
	}

	fname := filepath.Join(srv.dir, fid+".root")
	o, err := os.Create(fname)
	if err != nil {
		return fmt.Errorf("could not create temporary file: %w", err)
	}
	_, err = io.CopyBuffer(o, f, make([]byte, 16*1024*1024))
	if err != nil {
		return fmt.Errorf("could not copy uploaded file: %w", err)
	}
	o.Close()
	f.Close()

	rfile, err := riofs.Open(o.Name())
	if err != nil {
		return fmt.Errorf("could not open ROOT file %q: %w", dst, err)
	}

	db.set(dst, rfile)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	return json.NewEncoder(w).Encode(nil)
}

// CloseFile closes a file specified by the CloseFileRequest:
//   {"uri": "file:///some/file.root"}
func (srv *Server) CloseFile(w http.ResponseWriter, r *http.Request) {
	srv.wrap(srv.handleCloseFile)(w, r)
}

func (srv *Server) handleCloseFile(w http.ResponseWriter, r *http.Request) error {
	db, err := srv.db(r)
	if err != nil {
		return fmt.Errorf("could not open file database: %w", err)
	}

	dec := json.NewDecoder(r.Body)
	defer r.Body.Close()

	var req CloseFileRequest
	err = dec.Decode(&req)
	if err != nil {
		return fmt.Errorf("could not decode request: %w", err)
	}

	db.del(req.URI)

	w.WriteHeader(http.StatusOK)
	return nil
}

// ListFiles lists all the files currently known to the server.
// ListFiles replies with a StatusOK and a ListResponse:
//   [{"uri": "file:///some/file.root"},
//    {"uri": "root://example.org/file.root"}]
func (srv *Server) ListFiles(w http.ResponseWriter, r *http.Request) {
	srv.wrap(srv.handleListFiles)(w, r)
}

func (srv *Server) handleListFiles(w http.ResponseWriter, r *http.Request) error {
	db, err := srv.db(r)
	if err != nil {
		return fmt.Errorf("could not open file database: %w", err)
	}

	var resp ListResponse
	db.RLock()
	defer db.RUnlock()

	for uri, f := range db.files {
		resp.Files = append(resp.Files, File{URI: uri, Version: f.Version()})
	}
	sort.Slice(resp.Files, func(i, j int) bool {
		return resp.Files[i].URI < resp.Files[j].URI
	})

	w.WriteHeader(http.StatusOK)
	w.Header().Set("Content-Type", "application/json")
	return json.NewEncoder(w).Encode(resp)
}

// Dirent lists the content of a ROOT directory inside a ROOT file.
// Dirent expects a DirentRequest:
//   {"uri": "file:///some/file.root", "dir": "/some/dir", "recursive": true}
//   {"uri": "root://example.org/some/file.root", "dir": "/some/dir"}
// Dirent replies with a DirentResponse:
//   {"uri": "file:///some/file.root", "content": [
//     {"path": "/dir", "type": "TDirectoryFile", "name": "dir", "title": "my title"},
//     {"path": "/dir/obj", "type": "TObjString", "name": "obj", "title": "obj string"},
//     {"path": "/dir/sub", "type": "TDirectoryFile", "name": "sub", "title": "my sub dir"},
//     {"path": "/dir/sub/obj", "type": "TObjString", "name": "obj", "title": "my sub obj string"}
//   ]}
func (srv *Server) Dirent(w http.ResponseWriter, r *http.Request) {
	srv.wrap(srv.handleDirent)(w, r)
}

func (srv *Server) handleDirent(w http.ResponseWriter, r *http.Request) error {
	dec := json.NewDecoder(r.Body)
	defer r.Body.Close()

	var (
		req  DirentRequest
		resp DirentResponse
	)

	err := dec.Decode(&req)
	if err != nil {
		return fmt.Errorf("could not decode dirent request: %w", err)
	}

	resp.URI = req.URI

	db, err := srv.db(r)
	if err != nil {
		return fmt.Errorf("could not open file database: %w", err)
	}

	f := db.get(req.URI)
	if f == nil {
		return fmt.Errorf("rsrv: could not find ROOT file %q", req.URI)
	}

	if !strings.HasPrefix(req.Dir, "/") {
		req.Dir = "/" + req.Dir
	}

	// FIXME(sbinet): also handle relative dir-paths? (eg: ./foo/../dir/obj)

	var dir riofs.Directory
	switch req.Dir {
	default:
		obj, err := riofs.Dir(f).Get(req.Dir)
		if err != nil {
			return fmt.Errorf("rsrv: could not find directory %q in ROOT file %q: %w", req.Dir, req.URI, err)
		}
		var ok bool
		dir, ok = obj.(riofs.Directory)
		if !ok {
			return fmt.Errorf("rsrv: %q not a directory", req.Dir)
		}
	case "/":
		dir = f
	}

	switch req.Recursive {
	default:
		obj := dir.(root.Named)
		resp.Content = append(resp.Content, Dirent{
			Path:  req.Dir,
			Type:  obj.Class(),
			Name:  obj.Name(),
			Title: obj.Title(),
		})
		for _, key := range dir.Keys() {
			resp.Content = append(resp.Content, Dirent{
				Path:  stdpath.Join(req.Dir, key.Name()),
				Type:  key.ClassName(),
				Name:  key.Name(),
				Title: key.Title(),
				Cycle: key.Cycle(),
			})
		}
	case true:
		err = riofs.Walk(dir, func(path string, obj root.Object, err error) error {
			var (
				name  = ""
				title = ""
				cycle = 0
			)
			if o, ok := obj.(root.Named); ok {
				name = o.Name()
				title = o.Title()
			}

			type cycler interface {
				Cycle() int
			}
			if o, ok := obj.(cycler); ok {
				cycle = o.Cycle()
			}

			opath := strings.Replace("/"+path, "/"+f.Name(), "/", 1)
			if strings.HasPrefix(opath, "//") {
				opath = strings.Replace(opath, "//", "/", 1)
			}
			resp.Content = append(resp.Content, Dirent{
				Path:  opath,
				Type:  obj.Class(),
				Name:  name,
				Title: title,
				Cycle: cycle,
			})
			return nil
		})
		if err != nil {
			return fmt.Errorf("could not list directory: %w", err)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	return json.NewEncoder(w).Encode(resp)
}

// Tree returns the structure of a TTree specified by the TreeRequest:
//  {"uri": "file:///some/file.root", "dir": "/some/dir", "obj": "myTree"}
// Tree replies with a TreeResponse:
//  {"uri": "file:///some/file.root", "dir": "/some/dir", "obj": "myTree",
//    "tree": {
//      "type": "TTree", "name": "myTree", "title": "my title", "cycle": 1,
//      "entries": 42,
//      "branches": [{"type": "TBranch", "name": "Int64"}, ...],
//      "leaves": [{"type": "TLeafL", "name": "Int64"}, ...]
//    }
//  }
func (srv *Server) Tree(w http.ResponseWriter, r *http.Request) {
	srv.wrap(srv.handleTree)(w, r)
}

func (srv *Server) handleTree(w http.ResponseWriter, r *http.Request) error {
	dec := json.NewDecoder(r.Body)
	defer r.Body.Close()

	var req TreeRequest

	err := dec.Decode(&req)
	if err != nil {
		return fmt.Errorf("could not decode tree request: %w", err)
	}

	resp := TreeResponse{
		URI: req.URI,
		Dir: req.Dir,
		Obj: req.Obj,
	}

	db, err := srv.db(r)
	if err != nil {
		return fmt.Errorf("could not open ROOT file database: %w", err)
	}

	f := db.get(req.URI)
	if f == nil {
		return fmt.Errorf("rsrv: could not find ROOT file named %q", req.URI)
	}

	obj, err := riofs.Dir(f).Get(req.Dir)
	if err != nil {
		return fmt.Errorf("could not find directory %q in file %q: %w", req.Dir, req.URI, err)
	}
	dir, ok := obj.(riofs.Directory)
	if !ok {
		return fmt.Errorf("rsrv: %q in file %q is not a directory", req.Dir, req.URI)
	}

	obj, err = dir.Get(req.Obj)
	if err != nil {
		return fmt.Errorf("could not find object %q under directory %q in file %q: %w", req.Obj, req.Dir, req.URI, err)
	}

	tree, ok := obj.(rtree.Tree)
	if !ok {
		return fmt.Errorf("rsrv: object %v:%s/%q is not a tree (type=%s)", req.URI, req.Dir, req.Obj, obj.Class())
	}

	resp.Tree.Type = tree.Class()
	resp.Tree.Name = tree.Name()
	resp.Tree.Title = tree.Title()
	resp.Tree.Entries = tree.Entries()

	var cnvBranch func(b rtree.Branch) Branch
	var cnvLeaf func(b rtree.Leaf) Leaf

	cnvBranch = func(b rtree.Branch) Branch {
		o := Branch{
			Type: b.Class(),
			Name: b.Name(),
		}
		for _, sub := range b.Branches() {
			o.Branches = append(o.Branches, cnvBranch(sub))
		}
		for _, sub := range b.Leaves() {
			o.Leaves = append(o.Leaves, cnvLeaf(sub))
		}
		return o
	}

	cnvLeaf = func(leaf rtree.Leaf) Leaf {
		o := Leaf{
			Type: leaf.TypeName(),
			Name: leaf.Name(),
		}
		return o
	}

	for _, b := range tree.Branches() {
		resp.Tree.Branches = append(resp.Tree.Branches, cnvBranch(b))
	}

	for _, leaf := range tree.Leaves() {
		resp.Tree.Leaves = append(resp.Tree.Leaves, cnvLeaf(leaf))
	}

	return json.NewEncoder(w).Encode(resp)
}

// PlotH1 plots the 1-dim histogram specified by the PlotH1Request:
//  {"uri": "file:///some/file.root", "dir": "/some/dir", "obj": "h1", "type": "png"}
//  {"uri": "file:///some/file.root", "dir": "/some/dir", "obj": "h1", "type": "svg",
//     "options": {
//       "title": "my histo title", "x": "my x-axis", "y": "my y-axis",
//       "line": {"color": "#ff0000ff", ...},
//       "fill_color": "#00ff00ff"}
//  }}
// PlotH1 replies with a PlotResponse, where "data" contains the base64 encoded representation of
// the plot.
func (srv *Server) PlotH1(w http.ResponseWriter, r *http.Request) {
	srv.wrap(srv.handlePlotH1)(w, r)
}

func (srv *Server) handlePlotH1(w http.ResponseWriter, r *http.Request) error {
	dec := json.NewDecoder(r.Body)
	defer r.Body.Close()

	var (
		req  PlotH1Request
		resp PlotResponse
	)

	err := dec.Decode(&req)
	if err != nil {
		return fmt.Errorf("could not decode plot-h1 request: %w", err)
	}

	db, err := srv.db(r)
	if err != nil {
		return fmt.Errorf("could not open ROOT file database: %w", err)
	}

	err = db.Tx(req.URI, func(f *riofs.File) error {
		if f == nil {
			return fmt.Errorf("rsrv: could not find ROOT file named %q", req.URI)
		}

		obj, err := riofs.Dir(f).Get(req.Dir)
		if err != nil {
			return fmt.Errorf("could not find directory %q in file %q: %w", req.Dir, req.URI, err)
		}
		dir, ok := obj.(riofs.Directory)
		if !ok {
			return fmt.Errorf("rsrv: %q in file %q is not a directory", req.Dir, req.URI)
		}

		obj, err = dir.Get(req.Obj)
		if err != nil {
			return fmt.Errorf("could not find object %q under directory %q in file %q: %w", req.Obj, req.Dir, req.URI, err)
		}

		robj, ok := obj.(rhist.H1)
		if !ok {
			return fmt.Errorf("rsrv: object %v:%s/%q is not a 1-dim histogram (type=%s)", req.URI, req.Dir, req.Obj, obj.Class())
		}

		h1 := rootcnv.H1D(robj)

		req.Options.init()

		pl := hplot.New()
		pl.Title.Text = robj.Title()
		if req.Options.Title != "" {
			pl.Title.Text = req.Options.Title
		}
		pl.X.Label.Text = req.Options.X
		pl.Y.Label.Text = req.Options.Y

		h := hplot.NewH1D(h1)
		h.Infos.Style = hplot.HInfoSummary
		h.Color = req.Options.Line.Color
		h.FillColor = req.Options.FillColor

		pl.Add(h, hplot.NewGrid())

		out, err := srv.render(pl, req.Options)
		if err != nil {
			return fmt.Errorf("could not render H1 plot: %w", err)
		}

		resp.URI = req.URI
		resp.Dir = req.Dir
		resp.Obj = req.Obj
		resp.Data = base64.StdEncoding.EncodeToString(out)
		return nil
	})
	if err != nil {
		return err
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	return json.NewEncoder(w).Encode(resp)
}

// PlotH2 plots the 2-dim histogram specified by the PlotH2Request:
//  {"uri": "file:///some/file.root", "dir": "/some/dir", "obj": "h2", "type": "png"}
//  {"uri": "file:///some/file.root", "dir": "/some/dir", "obj": "h2", "type": "svg",
//     "options": {
//       "title": "my histo title", "x": "my x-axis", "y": "my y-axis"
//  }}
// PlotH2 replies with a PlotResponse, where "data" contains the base64 encoded representation of
// the plot.
func (srv *Server) PlotH2(w http.ResponseWriter, r *http.Request) {
	srv.wrap(srv.handlePlotH2)(w, r)
}

func (srv *Server) handlePlotH2(w http.ResponseWriter, r *http.Request) error {
	dec := json.NewDecoder(r.Body)
	defer r.Body.Close()

	var (
		req  PlotH2Request
		resp PlotResponse
	)

	err := dec.Decode(&req)
	if err != nil {
		return fmt.Errorf("could not decode plot-h2 request: %w", err)
	}

	db, err := srv.db(r)
	if err != nil {
		return fmt.Errorf("could not open ROOT file database: %w", err)
	}

	err = db.Tx(req.URI, func(f *riofs.File) error {
		if f == nil {
			return fmt.Errorf("rsrv: could not find ROOT file named %q", req.URI)
		}

		obj, err := riofs.Dir(f).Get(req.Dir)
		if err != nil {
			return fmt.Errorf("could not find directory %q in file %q: %w", req.Dir, req.URI, err)
		}
		dir, ok := obj.(riofs.Directory)
		if !ok {
			return fmt.Errorf("rsrv: %q in file %q is not a directory", req.Dir, req.URI)
		}

		obj, err = dir.Get(req.Obj)
		if err != nil {
			return fmt.Errorf("could not find object %q under directory %q in file %q: %w", req.Obj, req.Dir, req.URI, err)
		}

		robj, ok := obj.(rhist.H2)
		if !ok {
			return fmt.Errorf("rsrv: object %v:%s/%q is not a 2-dim histogram (type=%s)", req.URI, req.Dir, req.Obj, obj.Class())
		}

		h2 := rootcnv.H2D(robj)

		req.Options.init()

		pl := hplot.New()
		pl.Title.Text = robj.Title()
		if req.Options.Title != "" {
			pl.Title.Text = req.Options.Title
		}
		pl.X.Label.Text = req.Options.X
		pl.Y.Label.Text = req.Options.Y

		h := hplot.NewH2D(h2, nil)
		h.Infos.Style = hplot.HInfoSummary

		pl.Add(h, hplot.NewGrid())

		out, err := srv.render(pl, req.Options)
		if err != nil {
			return fmt.Errorf("could not render H2 plot: %w", err)
		}

		resp.URI = req.URI
		resp.Dir = req.Dir
		resp.Obj = req.Obj
		resp.Data = base64.StdEncoding.EncodeToString(out)
		return nil
	})
	if err != nil {
		return err
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	return json.NewEncoder(w).Encode(resp)
}

// PlotS2 plots the 2-dim scatter specified by the PlotS2Request:
//  {"uri": "file:///some/file.root", "dir": "/some/dir", "obj": "gr", "type": "png"}
//  {"uri": "file:///some/file.root", "dir": "/some/dir", "obj": "gr", "type": "svg",
//     "options": {
//       "title": "my scatter title", "x": "my x-axis", "y": "my y-axis",
//       "line": {"color": "#ff0000ff", ...}
//  }}
// PlotS2 replies with a PlotResponse, where "data" contains the base64 encoded representation of
// the plot.
func (srv *Server) PlotS2(w http.ResponseWriter, r *http.Request) {
	srv.wrap(srv.handlePlotS2)(w, r)
}

func (srv *Server) handlePlotS2(w http.ResponseWriter, r *http.Request) error {
	dec := json.NewDecoder(r.Body)
	defer r.Body.Close()

	var (
		req  PlotS2Request
		resp PlotResponse
	)

	err := dec.Decode(&req)
	if err != nil {
		return fmt.Errorf("could not decode plot-s2 request: %w", err)
	}

	db, err := srv.db(r)
	if err != nil {
		return fmt.Errorf("could not open ROOT file database: %w", err)
	}

	err = db.Tx(req.URI, func(f *riofs.File) error {
		if f == nil {
			return fmt.Errorf("rsrv: could not find ROOT file named %q", req.URI)
		}

		obj, err := riofs.Dir(f).Get(req.Dir)
		if err != nil {
			return fmt.Errorf("could not find directory %q in file %q: %w", req.Dir, req.URI, err)
		}
		dir, ok := obj.(riofs.Directory)
		if !ok {
			return fmt.Errorf("rsrv: %q in file %q is not a directory", req.Dir, req.URI)
		}

		obj, err = dir.Get(req.Obj)
		if err != nil {
			return fmt.Errorf("could not find object %q under directory %q in file %q: %w", req.Obj, req.Dir, req.URI, err)
		}

		robj, ok := obj.(rhist.Graph)
		if !ok {
			return fmt.Errorf("rsrv: object %v:%s/%q is not a 2-dim scatter (type=%s)", req.URI, req.Dir, req.Obj, obj.Class())
		}

		s2 := rootcnv.S2D(robj)

		req.Options.init()

		pl := hplot.New()
		pl.Title.Text = robj.Title()
		if req.Options.Title != "" {
			pl.Title.Text = req.Options.Title
		}
		pl.X.Label.Text = req.Options.X
		pl.Y.Label.Text = req.Options.Y

		var opts []hplot.Options
		if _, ok := robj.(rhist.GraphErrors); ok {
			opts = append(
				opts,
				hplot.WithXErrBars(true), hplot.WithYErrBars(true),
			)
		}
		h := hplot.NewS2D(s2, opts...)
		h.Color = req.Options.Line.Color

		pl.Add(h, hplot.NewGrid())

		out, err := srv.render(pl, req.Options)
		if err != nil {
			return fmt.Errorf("could not render S2 plot: %w", err)
		}

		resp.URI = req.URI
		resp.Dir = req.Dir
		resp.Obj = req.Obj
		resp.Data = base64.StdEncoding.EncodeToString(out)
		return nil
	})
	if err != nil {
		return err
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	return json.NewEncoder(w).Encode(resp)
}

// PlotTree plots the Tree branch(es) specified by the PlotBranchRequest:
//  {"uri": "file:///some/file.root", "dir": "/some/dir", "obj": "gr", "type": "png", "vars": ["pt"]}
//  {"uri": "file:///some/file.root", "dir": "/some/dir", "obj": "gr", "type": "svg", "vars": ["pt", "eta"],
//     "options": {
//       "title": "my plot title", "x": "my x-axis", "y": "my y-axis",
//       "line": {"color": "#ff0000ff", ...}
//  }}
// PlotBranch replies with a PlotResponse, where "data" contains the base64 encoded representation of
// the plot.
func (srv *Server) PlotTree(w http.ResponseWriter, r *http.Request) {
	srv.wrap(srv.handlePlotTree)(w, r)
}

func (srv *Server) handlePlotTree(w http.ResponseWriter, r *http.Request) error {
	dec := json.NewDecoder(r.Body)
	defer r.Body.Close()

	var (
		req  PlotTreeRequest
		resp PlotResponse
	)

	err := dec.Decode(&req)
	if err != nil {
		return fmt.Errorf("could not decode plot-tree request: %w", err)
	}

	db, err := srv.db(r)
	if err != nil {
		return fmt.Errorf("could not open ROOT file database: %w", err)
	}

	err = db.Tx(req.URI, func(f *riofs.File) error {
		if f == nil {
			return fmt.Errorf("rsrv: could not find ROOT file named %q", req.URI)
		}

		obj, err := riofs.Dir(f).Get(req.Dir)
		if err != nil {
			return fmt.Errorf("could not find directory %q in file %q: %w", req.Dir, req.URI, err)
		}
		dir, ok := obj.(riofs.Directory)
		if !ok {
			return fmt.Errorf("rsrv: %q in file %q is not a directory", req.Dir, req.URI)
		}

		obj, err = dir.Get(req.Obj)
		if err != nil {
			return fmt.Errorf("could not find object %q under directory %q in file %q: %w", req.Obj, req.Dir, req.URI, err)
		}

		tree, ok := obj.(rtree.Tree)
		if !ok {
			return fmt.Errorf("rsrv: object %v:%s/%q is not a tree (type=%s)", req.URI, req.Dir, req.Obj, obj.Class())
		}

		if len(req.Vars) != 1 {
			return fmt.Errorf("rsrv: tree-draw of %d variables not supported", len(req.Vars))
		}

		var (
			bname = req.Vars[0]
			br    = tree.Branch(bname)
		)
		if br == nil {
			return fmt.Errorf("rsrv: tree %v:%s/%s has no branch %q", req.URI, req.Dir, req.Obj, bname)
		}

		var (
			leaves = br.Leaves()
			leaf   = leaves[0] // FIXME(sbinet) handle sub-leaves
		)

		fv, err := newFloats(leaf)
		if err != nil {
			return fmt.Errorf("could not create float-leaf: %w", err)
		}

		min := +math.MaxFloat64
		max := -math.MaxFloat64
		vals := make([]float64, 0, int(tree.Entries()))
		r, err := rtree.NewReader(tree, []rtree.ReadVar{{
			Name:  bname,
			Leaf:  leaf.Name(),
			Value: fv.ptr,
		}})
		if err != nil {
			return fmt.Errorf(
				"could not create reader for branch %q in tree %q of file %q: %w",
				bname, tree.Name(), req.URI, err,
			)
		}
		defer r.Close()

		err = r.Read(func(ctx rtree.RCtx) error {
			for _, v := range fv.vals() {
				if !math.IsNaN(v) && !math.IsInf(v, 0) {
					max = math.Max(max, v)
					min = math.Min(min, v)
				}
				vals = append(vals, v)
			}
			return nil
		})
		if err != nil {
			return fmt.Errorf("could not complete scan: %w", err)
		}

		err = r.Close()
		if err != nil {
			return fmt.Errorf("could not close reader: %w", err)
		}

		min = math.Nextafter(min, min-1)
		max = math.Nextafter(max, max+1)
		h1 := hbook.NewH1D(100, min, max)
		for _, v := range vals {
			h1.Fill(v, 1)
		}

		req.Options.init()

		pl := hplot.New()
		pl.Title.Text = leaf.Name()
		if req.Options.Title != "" {
			pl.Title.Text = req.Options.Title
		}
		pl.X.Label.Text = req.Options.X
		pl.Y.Label.Text = req.Options.Y

		h := hplot.NewH1D(h1)
		h.Infos.Style = hplot.HInfoSummary
		h.Color = req.Options.Line.Color
		h.FillColor = req.Options.FillColor

		pl.Add(h, hplot.NewGrid())

		out, err := srv.render(pl, req.Options)
		if err != nil {
			return fmt.Errorf("could not render tree plot: %w", err)
		}

		resp.URI = req.URI
		resp.Dir = req.Dir
		resp.Obj = req.Obj
		resp.Data = base64.StdEncoding.EncodeToString(out)
		return nil
	})
	if err != nil {
		return err
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	return json.NewEncoder(w).Encode(resp)
}
