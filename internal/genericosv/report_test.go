// Copyright 2023 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package genericosv

import (
	"flag"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	osvschema "github.com/google/osv-scanner/pkg/models"
	"golang.org/x/vulndb/internal/proxy"
	"golang.org/x/vulndb/internal/report"
)

var (
	realProxy = flag.Bool("proxy", false, "if true, contact the real module proxy and update expected responses")
	update    = flag.Bool("update", false, "if true, update test YAML reports to reflect new expected behavior")
)

var (
	testdataDir = "testdata"
	testOSVDir  = filepath.Join(testdataDir, "osv")
	testYAMLDir = filepath.Join(testdataDir, "yaml")
)

// To update test cases to reflect new expected behavior
// (only use -proxy if the calls to the proxy will change):
// go test ./internal/genericosv/... -update -proxy -run TestToReport
func TestToReport(t *testing.T) {
	if err := filepath.WalkDir(testOSVDir, func(path string, f fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if f.IsDir() || filepath.Ext(path) != ".json" {
			return nil
		}
		ghsaID := strings.TrimSuffix(f.Name(), ".json")
		t.Run(ghsaID, func(t *testing.T) {
			t.Parallel()

			pc, err := proxy.NewTestClient(t, *realProxy)
			if err != nil {
				t.Fatal(err)
			}

			osv := Entry{}
			if err := report.UnmarshalFromFile(path, &osv); err != nil {
				t.Fatal(err)
			}

			got := osv.ToReport("GO-TEST-ID", pc)
			yamlFile := filepath.Join(testYAMLDir, ghsaID+".yaml")
			if *update {
				if err := got.Write(yamlFile); err != nil {
					t.Fatal(err)
				}
			}
			want, err := report.Read(yamlFile)
			if err != nil {
				t.Fatal(err)
			}
			if diff := cmp.Diff(want, got); diff != "" {
				t.Errorf("ToReport() mismatch (-want +got)\n%s", diff)
			}
		})
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

// TODO(https://go.dev/issues/61769): unskip test cases as we add features.
// To update proxy responses:
// go test ./internal/genericosv/... -proxy -run TestAffectedToModules
func TestAffectedToModules(t *testing.T) {
	for _, tc := range []struct {
		name string
		desc string
		in   []osvschema.Affected
		want []*report.Module
		skip bool
	}{
		{
			name: "ok",
			desc: "module is already OK",
			in: []osvschema.Affected{{
				Package: osvschema.Package{
					Ecosystem: osvschema.EcosystemGo,
					Name:      "github.com/influxdata/influxdb",
				},
				Ranges: []osvschema.Range{{
					Type: osvschema.RangeEcosystem,
					Events: []osvschema.Event{
						{
							Introduced: "0.3.2",
						},
						{
							Fixed: "1.7.6",
						},
					},
				}},
			}},
			want: []*report.Module{{
				Module: "github.com/influxdata/influxdb",
				Versions: []report.VersionRange{
					{
						Introduced: "0.3.2",
						Fixed:      "1.7.6",
					}},
				VulnerableAt: "1.7.5",
			}},
		},
		{
			name: "import_path",
			desc: "find module from import path",
			in: []osvschema.Affected{{
				Package: osvschema.Package{
					Ecosystem: osvschema.EcosystemGo,
					Name:      "github.com/influxdata/influxdb/services/httpd",
				},
				Ranges: []osvschema.Range{{
					Type: osvschema.RangeEcosystem,
					Events: []osvschema.Event{
						{
							Introduced: "0.3.2",
						},
						{
							Fixed: "1.7.6",
						},
					},
				}},
			}},
			want: []*report.Module{{
				Module: "github.com/influxdata/influxdb",
				Versions: []report.VersionRange{
					{
						Introduced: "0.3.2",
						Fixed:      "1.7.6",
					},
				},
				Packages: []*report.Package{
					{
						Package: "github.com/influxdata/influxdb/services/httpd",
					},
				},
				VulnerableAt: "1.7.5",
			}},
		},
		{
			name: "major_version",
			desc: "correct major version of module path",
			in: []osvschema.Affected{{
				Package: osvschema.Package{
					Ecosystem: osvschema.EcosystemGo,
					Name:      "github.com/nats-io/nats-server",
				},
				Ranges: []osvschema.Range{{
					Type: osvschema.RangeEcosystem,
					Events: []osvschema.Event{
						{
							Introduced: "2.2.0",
						},
						{
							Fixed: "2.8.3",
						},
					},
				}},
			}},
			want: []*report.Module{{
				Module: "github.com/nats-io/nats-server/v2",
				Versions: []report.VersionRange{
					{
						Introduced: "2.2.0",
						Fixed:      "2.8.3",
					},
				},
			}},
			skip: true,
		},
		{
			name: "canonicalize",
			desc: "canonicalize module path",
			in: []osvschema.Affected{{
				Package: osvschema.Package{
					Ecosystem: osvschema.EcosystemGo,
					Name:      "github.com/golang/vulndb",
				},
				Ranges: []osvschema.Range{{
					Type: osvschema.RangeEcosystem,
					Events: []osvschema.Event{
						{
							Fixed: "0.0.0-20230712151357-4fee11d0f8f9",
						},
					},
				}},
			}},
			want: []*report.Module{{
				Module: "golang.org/x/vulndb",
				Versions: []report.VersionRange{
					{
						Fixed: "0.0.0-20230712151357-4fee11d0f8f9",
					},
				},
			}},
			skip: true,
		},
		{
			name: "add_incompatible",
			desc: "add +incompatible",
			in: []osvschema.Affected{{
				Package: osvschema.Package{
					Ecosystem: osvschema.EcosystemGo,
					Name:      "github.com/docker/docker",
				},
				Ranges: []osvschema.Range{{
					Type: osvschema.RangeEcosystem,
					Events: []osvschema.Event{
						{
							Fixed: "23.0.0",
						},
					},
				}},
			}},
			want: []*report.Module{{
				Module: "github.com/docker/docker",
				Versions: []report.VersionRange{
					{
						Fixed: "23.0.0+incompatible",
					},
				},
			}},
			skip: true,
		},
		{
			name: "remove_duplicates",
			desc: "remove major version duplicates",
			in: []osvschema.Affected{{
				Package: osvschema.Package{
					Ecosystem: osvschema.EcosystemGo,
					Name:      "github.com/hashicorp/go-getter/v2",
				},
				Ranges: []osvschema.Range{{
					Type: osvschema.RangeEcosystem,
					Events: []osvschema.Event{
						{
							Introduced: "0",
						},
						{
							Fixed: "2.1.0",
						},
					},
				}},
			},
				{
					Package: osvschema.Package{
						Ecosystem: osvschema.EcosystemGo,
						Name:      "github.com/hashicorp/go-getter",
					},
					Ranges: []osvschema.Range{{
						Type: osvschema.RangeEcosystem,
						Events: []osvschema.Event{
							{
								Introduced: "2.0.0",
							},
							{
								Fixed: "2.1.0",
							},
						},
					}},
				}},
			want: []*report.Module{{
				Module: "github.com/hashicorp/go-getter/v2",
				Versions: []report.VersionRange{
					{
						Introduced: "2.0.0",
						Fixed:      "2.1.0",
					},
				},
			}},
			skip: true,
		},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if tc.skip {
				t.Skip("skipping (not implemented yet)")
			}

			pc, err := proxy.NewTestClient(t, *realProxy)
			if err != nil {
				t.Fatal(err)
			}

			var gotNotes []string
			addNote := func(note string) {
				gotNotes = append(gotNotes, note)
			}
			got := affectedToModules(tc.in, addNote, pc)
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Errorf("%s: affectedToModules() mismatch (-want +got)\n%s", tc.desc, diff)
			}
			if len(gotNotes) > 0 {
				t.Errorf("%s: affectedToModules() output unexpected notes = %s", tc.desc, gotNotes)
			}
		})

	}
}
