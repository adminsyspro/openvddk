// Copyright 2026 Andrija Panic and the OpenVDDK contributors
// SPDX-License-Identifier: Apache-2.0

package vsphere

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vim25"
	"github.com/vmware/govmomi/vim25/soap"
	"github.com/vmware/govmomi/vim25/types"
)

// fileService stands for the datastore file service of vCenter and ESXi,
// which decodes a '+' of the request path as a space (NET-0006): it serves
// the files of names, and answers 404 for anything else.
func fileService(t *testing.T, names ...string) *httptest.Server {
	t.Helper()
	files := map[string]bool{}
	for _, n := range names {
		files["/folder/"+n] = true
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, err := url.QueryUnescape(r.URL.EscapedPath())
		if err != nil || !files[p] || r.URL.Query().Get("dsName") != "ds1" {
			http.NotFound(w, r)
			return
		}
		if r.Method == http.MethodGet {
			io.WriteString(w, "# Disk DescriptorFile\n")
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

// datastoreAt is datastore ds1 of datacenter dc1 behind srv, as a Finder
// would return it (inventory path set, so nothing is asked of vCenter).
func datastoreAt(t *testing.T, srv *httptest.Server) *object.Datastore {
	t.Helper()
	u, err := url.Parse(srv.URL + "/sdk")
	if err != nil {
		t.Fatal(err)
	}
	c := &vim25.Client{Client: soap.NewClient(u, true)}
	ds := object.NewDatastore(c, types.ManagedObjectReference{Type: "Datastore", Value: "datastore-1"})
	ds.InventoryPath = "/dc1/datastore/ds1"
	ds.DatacenterPath = "/dc1"
	return ds
}

func TestFileServicePathWithPlus(t *testing.T) {
	names := []string{
		"web+db/web+db.vmdk",
		"a b (old)/a b (old).vmdk",
		"x&y=z;w,v@u!/x&y=z;w,v@u!.vmdk",
	}
	ds := datastoreAt(t, fileService(t, names...))
	ctx := context.Background()
	for _, n := range names {
		exists, err := fileExists(ctx, ds, n)
		if err != nil || !exists {
			t.Errorf("fileExists(%q) = %v, %v; want true", n, exists, err)
		}
		rc, err := download(ctx, ds, n, soap.DefaultDownload)
		if err != nil {
			t.Errorf("download(%q): %v", n, err)
			continue
		}
		b, _ := io.ReadAll(rc)
		rc.Close()
		if string(b) != "# Disk DescriptorFile\n" {
			t.Errorf("download(%q) = %q", n, b)
		}
	}
	// The file service still tells a missing file apart.
	if exists, err := fileExists(ctx, ds, "web db/web db.vmdk"); err != nil || exists {
		t.Errorf("fileExists of a missing file = %v, %v; want false", exists, err)
	}
	if _, err := download(ctx, ds, "web+db/missing.vmdk", soap.DefaultDownload); err == nil {
		t.Error("download of a missing file succeeded")
	}
}

func TestPlusEscaped(t *testing.T) {
	for in, want := range map[string]string{
		"/folder/a+b/a+b.vmdk":             "/folder/a%2Bb/a%2Bb.vmdk",
		"/folder/a%20b/a%20b.vmdk":         "/folder/a%20b/a%20b.vmdk",
		"/folder/a%2Bb/x.vmdk":             "/folder/a%2Bb/x.vmdk",
		"/folder/(x)&y=z;w/(x)&y=z;w.vmdk": "/folder/(x)&y=z;w/(x)&y=z;w.vmdk",
	} {
		if got := plusEscaped(in); got != want {
			t.Errorf("plusEscaped(%q) = %q, want %q", in, got, want)
		}
	}
}
