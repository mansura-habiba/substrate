// Copyright 2026 The Agent Substrate Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package scheduling

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"go.ate.dev/substrate/api/ateapi/v1"
	"go.ate.dev/substrate/internal/hardware"
	"go.ate.dev/substrate/internal/resources"
	"k8s.io/apimachinery/pkg/labels"
)

// Fleet sizes for the benchmarks. 50k is about where a single cluster of
// dense nodes lands; the smaller sizes show how cost grows with the fleet.
var benchFleetSizes = []int{1_000, 10_000, 50_000}

const (
	benchWorkersPerNode  = 8
	benchActorsPerWorker = 16
	// benchWorkerCPUFeatures is a cpu_features mask every bench worker reports,
	// so placement pays the per-worker parse that a real fleet pays.
	benchWorkerCPUFeatures = "3fffffffffffffff"
	// benchSnapshotCPUFeatures is a subset of benchWorkerCPUFeatures.
	benchSnapshotCPUFeatures = "fffffff"
)

// benchRuntime is a gVisor runtime that carries cpu_features, and bundleHash
// when it is not empty.
func benchRuntime(bundleHash string) *ateapi.ActorSandboxRuntime {
	rt := sandboxRuntime("gvisor", "amd64", bundleHash)
	rt.Version.Attributes = append(rt.Version.Attributes,
		&ateapi.AttributeEntry{Key: hardware.AttrCPUFeatures, Value: benchWorkerCPUFeatures})
	return rt
}

// benchFleet returns n registered, ACTIVE gVisor workers spread over n/8
// nodes, each with room for benchActorsPerWorker actors, 8 cores and 32 GiB.
// used reports how many actors worker i already holds.
func benchFleet(n int, used func(i int) int) fleet {
	lbls := map[string]string{"ate.dev/pool": "agents", "tier": "1"}
	f := make(fleet, n)
	for i := range n {
		w := worker(fmt.Sprintf("w-%d", i), "", fmt.Sprintf("node-%d", i/benchWorkersPerNode), lbls,
			withRuntimes(benchRuntime("bundle-a")),
			withMaxActors(benchActorsPerWorker),
			withCapacity(8000, 32<<30))
		for j := range used(i) {
			assignedFor("bench", fmt.Sprintf("a-%d-%d", i, j), resources.CPUMemory(500, 2<<30))(w)
		}
		f[i] = w
	}
	return f
}

// benchConstraints is a memory-fidelity resume of a 500m / 2 GiB actor whose
// snapshot needs bundleHash and a subset of the fleet's cpu features.
func benchConstraints(bundleHash string) Constraints {
	snap := sandboxRuntime("gvisor", "amd64", bundleHash)
	snap.Version.Attributes = append(snap.Version.Attributes,
		&ateapi.AttributeEntry{Key: hardware.AttrCPUFeatures, Value: benchSnapshotCPUFeatures})
	req := hardware.NewRequirement(snap)
	return Constraints{
		SandboxClass:     "gvisor",
		RequiredRuntime:  req,
		SnapshotRuntime:  req,
		TemplateSelector: labels.SelectorFromSet(labels.Set{"tier": "1"}),
		ActorSelector:    labels.SelectorFromSet(labels.Set{"ate.dev/pool": "agents"}),
		Limits:           resources.CPUMemory(500, 2<<30),
	}
}

// BenchmarkSchedule measures one placement against fleets of growing size, in
// the shapes the resume path asks for. Every shape scans the whole fleet
// today; the benchmark makes that cost visible (see #3783).
func BenchmarkSchedule(b *testing.B) {
	half := func(int) int { return benchActorsPerWorker / 2 }
	full := func(int) int { return benchActorsPerWorker }

	for _, n := range benchFleetSizes {
		// The common case: a fleet with room everywhere.
		b.Run(fmt.Sprintf("room/workers=%d", n), func(b *testing.B) {
			benchSchedule(b, New(benchFleet(n, half)), benchConstraints("bundle-a"), nil)
		})

		// A saturated fleet: every resume scans everything to learn there is
		// no room.
		b.Run(fmt.Sprintf("saturated/workers=%d", n), func(b *testing.B) {
			benchSchedule(b, New(benchFleet(n, full)), benchConstraints("bundle-a"), ErrNoCapacity)
		})

		// A resume pinned to the node holding its local snapshot. Only that
		// node's workers can take it, but the whole fleet is still scanned.
		b.Run(fmt.Sprintf("pinned/workers=%d", n), func(b *testing.B) {
			c := benchConstraints("bundle-a")
			c.RequiredNodes = []string{fmt.Sprintf("node-%d", (n-1)/benchWorkersPerNode)}
			benchSchedule(b, New(benchFleet(n, half)), c, nil)
		})

		// A memory snapshot no worker can restore: the resume path scans once
		// with the runtime requirement, fails, and scans again without it
		// (see #3779). Measures both scans.
		b.Run(fmt.Sprintf("runtime-fallback/workers=%d", n), func(b *testing.B) {
			s := New(benchFleet(n, half))
			strict := benchConstraints("bundle-old")
			relaxed := strict
			relaxed.RequiredRuntime = hardware.Requirement{}
			ctx := context.Background()
			b.ReportAllocs()
			for b.Loop() {
				if _, err := s.Schedule(ctx, strict); !errors.Is(err, ErrNoCapacity) {
					b.Fatalf("strict Schedule() error = %v, want ErrNoCapacity", err)
				}
				if _, err := s.Schedule(ctx, relaxed); err != nil {
					b.Fatalf("relaxed Schedule() error = %v", err)
				}
			}
		})
	}
}

// benchSchedule runs Schedule in a loop and fails unless each call returns
// wantErr (nil for a successful placement).
func benchSchedule(b *testing.B, s Scheduler, c Constraints, wantErr error) {
	b.Helper()
	ctx := context.Background()
	b.ReportAllocs()
	for b.Loop() {
		_, err := s.Schedule(ctx, c)
		if wantErr == nil && err != nil {
			b.Fatalf("Schedule() error = %v", err)
		}
		if wantErr != nil && !errors.Is(err, wantErr) {
			b.Fatalf("Schedule() error = %v, want %v", err, wantErr)
		}
	}
}
