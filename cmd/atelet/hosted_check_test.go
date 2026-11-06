// Copyright 2026 Google LLC
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

package main

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/agent-substrate/substrate/internal/proto/ateworkerpb"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type hostedStatsServer struct {
	ateworkerpb.UnimplementedWorkerServer
	code codes.Code
}

func (s *hostedStatsServer) GetWorkloadStats(context.Context, *ateworkerpb.GetWorkloadStatsRequest) (*ateworkerpb.GetWorkloadStatsResponse, error) {
	if s.code == codes.OK {
		return &ateworkerpb.GetWorkloadStatsResponse{}, nil
	}
	return nil, status.Error(s.code, "fixture")
}

func TestWorkerHostsActor(t *testing.T) {
	// Not parallel: workerSocketPath is process-global.
	cases := []struct {
		name    string
		code    codes.Code
		want    bool
		wantErr bool
	}{
		{name: "ok means hosted", code: codes.OK, want: true},
		{name: "failed precondition means hosted", code: codes.FailedPrecondition, want: true},
		{name: "not found means cold", code: codes.NotFound, want: false},
		{name: "unavailable propagates", code: codes.Unavailable, wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir, err := os.MkdirTemp("", "wha-")
			if err != nil {
				t.Fatalf("MkdirTemp: %v", err)
			}
			t.Cleanup(func() { os.RemoveAll(dir) })
			sock := filepath.Join(dir, "w.sock")
			lis, err := net.Listen("unix", sock)
			if err != nil {
				t.Fatalf("listen: %v", err)
			}
			srv := grpc.NewServer()
			ateworkerpb.RegisterWorkerServer(srv, &hostedStatsServer{code: tc.code})
			go func() { _ = srv.Serve(lis) }()
			t.Cleanup(srv.Stop)

			orig := workerSocketPath
			workerSocketPath = func(string) string { return sock }
			t.Cleanup(func() { workerSocketPath = orig })

			s := &Atelet{workerDialer: newWorkerDialer(1)}
			got, err := s.workerHostsActor(context.Background(), "pod-uid", "actor-uid")
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("workerHostsActor: %v", err)
			}
			if got != tc.want {
				t.Fatalf("hosted = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestWorkerHostsActorEmptyIDs(t *testing.T) {
	t.Parallel()
	s := &Atelet{workerDialer: newWorkerDialer(1)}
	got, err := s.workerHostsActor(context.Background(), "", "actor")
	if err != nil || got {
		t.Fatalf("empty worker pod: hosted=%v err=%v, want false nil", got, err)
	}
	got, err = s.workerHostsActor(context.Background(), "pod", "")
	if err != nil || got {
		t.Fatalf("empty actor: hosted=%v err=%v, want false nil", got, err)
	}
}

func TestWorkerHostsActorDialError(t *testing.T) {
	t.Parallel()
	orig := workerSocketPath
	workerSocketPath = func(string) string { return filepath.Join(t.TempDir(), "missing.sock") }
	t.Cleanup(func() { workerSocketPath = orig })

	s := &Atelet{workerDialer: newWorkerDialer(1)}
	_, err := s.workerHostsActor(context.Background(), "pod", "actor")
	if err == nil {
		t.Fatal("expected dial error")
	}
}
