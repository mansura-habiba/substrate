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

package recovery

import "testing"

func TestDecide(t *testing.T) {
	assignedActive := Input{
		AssignmentRequired: true,
		HasAssignment:      true,
		WorkerHealth:       WorkerActive,
		HostsActor:         true,
	}

	tests := []struct {
		name string
		in   Input
		want Outcome
	}{
		{
			name: "missing assignment when required crashes",
			in: Input{
				AssignmentRequired: true,
				HasAssignment:      false,
			},
			want: Outcome{Action: CrashAndRelease, ClientVisibility: Unrecoverable},
		},
		{
			name: "unavailable worker waits on same assignment",
			in: Input{
				AssignmentRequired: true,
				HasAssignment:      true,
				WorkerHealth:       WorkerUnavailable,
				HostsActor:         true,
			},
			want: Outcome{Action: WaitSameAssignment, ClientVisibility: RecoveryPending},
		},
		{
			name: "unavailable never allows reassign",
			in: Input{
				AssignmentRequired: true,
				HasAssignment:      true,
				WorkerHealth:       WorkerUnavailable,
				HostsActor:         true,
				ReassignRequested:  true,
				SnapshotDurability: SnapshotDurable,
			},
			want: Outcome{Action: WaitSameAssignment, ClientVisibility: RecoveryPending},
		},
		{
			name: "ambiguous atelet retries same execution",
			in: Input{
				AssignmentRequired: true,
				HasAssignment:      true,
				WorkerHealth:       WorkerActive,
				HostsActor:         true,
				AteletClass:        AteletAmbiguous,
			},
			want: Outcome{Action: RetrySameExecution, ClientVisibility: RecoverableFailure},
		},
		{
			name: "ambiguous atelet never allows reassign",
			in: Input{
				AssignmentRequired: true,
				HasAssignment:      true,
				WorkerHealth:       WorkerActive,
				HostsActor:         true,
				AteletClass:        AteletAmbiguous,
				ReassignRequested:  true,
				SnapshotDurability: SnapshotDurable,
			},
			want: Outcome{Action: RetrySameExecution, ClientVisibility: RecoverableFailure},
		},
		{
			name: "definite atelet failure crashes",
			in: Input{
				AssignmentRequired: true,
				HasAssignment:      true,
				WorkerHealth:       WorkerActive,
				HostsActor:         true,
				AteletClass:        AteletDefinite,
			},
			want: Outcome{Action: CrashAndRelease, ClientVisibility: Unrecoverable, AllowReassign: true},
		},
		{
			name: "terminate RPC keeps assignment on definite error",
			in: Input{
				AssignmentRequired: true,
				HasAssignment:      true,
				WorkerHealth:       WorkerActive,
				HostsActor:         true,
				AteletClass:        AteletDefinite,
				TerminateRPC:       true,
			},
			want: Outcome{Action: RetrySameExecution, ClientVisibility: RecoverableFailure},
		},
		{
			name: "worker gone fences and allows reassign",
			in: Input{
				AssignmentRequired: true,
				HasAssignment:      true,
				WorkerHealth:       WorkerGone,
				HostsActor:         true,
			},
			want: Outcome{Action: CrashAndRelease, ClientVisibility: Unrecoverable, AllowReassign: true},
		},
		{
			name: "draining worker fences and allows reassign",
			in: Input{
				AssignmentRequired: true,
				HasAssignment:      true,
				WorkerHealth:       WorkerDraining,
				HostsActor:         true,
			},
			want: Outcome{Action: CrashAndRelease, ClientVisibility: Unrecoverable, AllowReassign: true},
		},
		{
			name: "epoch fence crashes and allows reassign",
			in: Input{
				AssignmentRequired: true,
				HasAssignment:      true,
				WorkerHealth:       WorkerActive,
				HostsActor:         true,
				EpochFenced:        true,
			},
			want: Outcome{Action: CrashAndRelease, ClientVisibility: Unrecoverable, AllowReassign: true},
		},
		{
			name: "worker no longer hosts actor fences",
			in: Input{
				AssignmentRequired: true,
				HasAssignment:      true,
				WorkerHealth:       WorkerActive,
				HostsActor:         false,
			},
			want: Outcome{Action: CrashAndRelease, ClientVisibility: Unrecoverable, AllowReassign: true},
		},
		{
			name: "local-only snapshot blocks migrate",
			in: Input{
				HasAssignment:      false,
				ReassignRequested:  true,
				SnapshotDurability: SnapshotLocalOnly,
			},
			want: Outcome{Action: RequireDurableBeforeMigrate, ClientVisibility: RecoverableFailure},
		},
		{
			name: "durable snapshot allows place when unbound",
			in: Input{
				HasAssignment:      false,
				ReassignRequested:  true,
				SnapshotDurability: SnapshotDurable,
			},
			want: Outcome{Action: Proceed, ClientVisibility: VisibilityNone, AllowReassign: true},
		},
		{
			name: "still-bound actor refuses migrate until fenced",
			in: Input{
				AssignmentRequired: true,
				HasAssignment:      true,
				WorkerHealth:       WorkerActive,
				HostsActor:         true,
				ReassignRequested:  true,
				SnapshotDurability: SnapshotDurable,
			},
			want: Outcome{Action: WaitSameAssignment, ClientVisibility: RecoveryPending},
		},
		{
			name: "healthy assigned worker proceeds",
			in:   assignedActive,
			want: Outcome{Action: Proceed, ClientVisibility: VisibilityNone},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Decide(tt.in)
			if got != tt.want {
				t.Fatalf("Decide() = %+v, want %+v", got, tt.want)
			}
			if tt.in.WorkerHealth == WorkerUnavailable && got.AllowReassign {
				t.Fatalf("unavailable worker must not AllowReassign")
			}
			if tt.in.AteletClass == AteletAmbiguous && got.AllowReassign {
				t.Fatalf("ambiguous atelet result must not AllowReassign")
			}
		})
	}
}
