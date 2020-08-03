// Copyright ©2018 The go-hep Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package rhist_test

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"go-hep.org/x/hep/groot/internal/rtests"
	"go-hep.org/x/hep/groot/rhist"
	"go-hep.org/x/hep/groot/riofs"
)

func TestCreate(t *testing.T) {

	dir, err := ioutil.TempDir("", "groot-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	for i, tc := range []struct {
		Name string
		Skip bool
		Want []rtests.ROOTer
	}{
		{
			Name: "TAxis",
			Want: []rtests.ROOTer{rhist.NewAxis("xaxis")},
		},
	} {
		fname := filepath.Join(dir, fmt.Sprintf("out-%d.root", i))
		t.Run(tc.Name, func(t *testing.T) {
			if tc.Skip {
				t.Skip()
			}

			w, err := riofs.Create(fname)
			if err != nil {
				t.Fatal(err)
			}

			for i := range tc.Want {
				var (
					kname = fmt.Sprintf("key-%s-%02d", tc.Name, i)
					want  = tc.Want[i]
				)

				err = w.Put(kname, want)
				if err != nil {
					t.Fatal(err)
				}
			}

			if got, want := len(w.Keys()), len(tc.Want); got != want {
				t.Fatalf("invalid number of keys. got=%d, want=%d", got, want)
			}

			err = w.Close()
			if err != nil {
				t.Fatalf("error closing file: %v", err)
			}

			r, err := riofs.Open(fname)
			if err != nil {
				t.Fatal(err)
			}
			defer r.Close()

			if got, want := len(r.Keys()), len(tc.Want); got != want {
				t.Fatalf("invalid number of keys. got=%d, want=%d", got, want)
			}

			for i := range tc.Want {
				var (
					kname = fmt.Sprintf("key-%s-%02d", tc.Name, i)
					want  = tc.Want[i]
				)

				rgot, err := r.Get(kname)
				if err != nil {
					t.Fatal(err)
				}

				if got := rgot.(rtests.ROOTer); !reflect.DeepEqual(got, want) {
					t.Fatalf("error reading back value[%d].\ngot = %#v\nwant= %#v", i, got, want)
				}
			}

			err = r.Close()
			if err != nil {
				t.Fatalf("error closing file: %v", err)
			}

			if !rtests.HasROOT {
				t.Logf("skip test with ROOT/C++")
				return
			}

			const rootls = `#include <iostream>
#include "TFile.h"
#include "TNamed.h"

void rootls(const char *fname, const char *kname) {
	auto f = TFile::Open(fname);
	auto o = f->Get<TNamed>(kname);
	if (o == NULL) {
		std:cerr << "could not retrieve [" << kname << "]" << std::endl;
		o->ClassName();
	}
	std::cout << "retrieved: [" << o->GetName() << "]" << std::endl;
}
`
			for i := range tc.Want {
				kname := fmt.Sprintf("key-%s-%02d", tc.Name, i)

				out, err := rtests.RunCxxROOT("rootls", []byte(rootls), fname, kname)
				if err != nil {
					t.Fatalf("ROOT/C++ could not open file %q:\n%s", fname, string(out))
				}
			}
		})
	}
}
