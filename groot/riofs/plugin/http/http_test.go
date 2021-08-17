// Copyright ©2019 The go-hep Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package http

import (
	"os"
	"testing"
)

func TestTmpFile(t *testing.T) {
	f, err := os.CreateTemp("", "riofs-remote-")
	if err != nil {
		t.Fatal(err)
	}
	tmp := tmpFile{f}
	defer tmp.Close()

	const want = "foo\n"
	_, err = tmp.WriteString(want)
	if err != nil {
		t.Fatal(err)
	}

	err = tmp.Sync()
	if err != nil {
		t.Fatal(err)
	}

	raw, err := os.ReadFile(tmp.Name())
	if err != nil {
		t.Fatal(err)
	}
	str := string(raw)
	if str != want {
		t.Fatalf("got=%q. want=%q", str, want)
	}

	err = tmp.Close()
	if err != nil {
		t.Fatal(err)
	}

	_, err = os.Stat(tmp.Name())
	if err == nil {
		t.Fatalf("file %q should have been removed", tmp.Name())
	}
}
