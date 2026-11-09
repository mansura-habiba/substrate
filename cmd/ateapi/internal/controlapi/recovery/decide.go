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

// Package recovery encodes the actor recovery contract for unavailable workers
// and ambiguous atelet results. Callers decide when to invoke Decide; the
// package itself has no store or RPC side effects.
//
// The critical rule: a timeout or transport failure does not prove the old
// execution stopped. Reassignment is only safe after confirmed termination or
// effective fencing (worker gone, draining cleanup, or epoch fence).
package recovery

// WorkerHealth is the control-plane view of the assigned worker.
type WorkerHealth int

const (
	// WorkerActive is Ready and eligible for work.
	WorkerActive WorkerHealth = iota
	// WorkerUnavailable is not Ready; bound actors stay assigned.
	WorkerUnavailable
	// WorkerDraining is leaving service; bound actors must crash/release.
	WorkerDraining
	// WorkerGone means the worker record is missing (pod deleted/replaced).
	WorkerGone
)

// AteletClass classifies a failed atelet RPC for recovery purposes.
type AteletClass int

const (
	// AteletNone means no atelet error is under consideration.
	AteletNone AteletClass = iota
	// AteletAmbiguous covers Unavailable, Canceled, DeadlineExceeded, and
	// lost-success responses: the sandbox may still be running.
	AteletAmbiguous
	// AteletDefinite is a hard failure where crashing the actor is correct.
	AteletDefinite
)

// SnapshotDurability says whether state can survive leaving the current node.
type SnapshotDurability int

const (
	// SnapshotIrrelevant means durability does not affect this decision.
	SnapshotIrrelevant SnapshotDurability = iota
	// SnapshotLocalOnly is node-local; migrating would lose state.
	SnapshotLocalOnly
	// SnapshotDurable is in durable storage and may follow a fenced reassignment.
	SnapshotDurable
)

// Action is what the control plane must do next.
type Action int

const (
	// Proceed continues the current operation on the assigned worker.
	Proceed Action = iota
	// WaitSameAssignment keeps the binding and asks the client to retry later
	// (worker temporarily unavailable).
	WaitSameAssignment
	// RetrySameExecution retries the atelet call against the same assignment /
	// execution identity without placing elsewhere.
	RetrySameExecution
	// CrashAndRelease fences the actor (crash + release) after confirmed
	// termination or epoch fencing; only then is reassignment safe.
	CrashAndRelease
	// RequireDurableBeforeMigrate refuses migrate/reassign while only a
	// local snapshot exists.
	RequireDurableBeforeMigrate
	// FailUnrecoverable reports a permanent failure to the client.
	FailUnrecoverable
)

// ClientVisibility is how the control plane should describe recovery to clients.
type ClientVisibility int

const (
	// VisibilityNone means no recovery-specific client signal.
	VisibilityNone ClientVisibility = iota
	// RecoveryPending means the system is waiting on the same assignment.
	RecoveryPending
	// RecoverableFailure means the client may retry; state is still held.
	RecoverableFailure
	// Unrecoverable means the actor was crashed or cannot continue.
	Unrecoverable
)

// Input is the observable facts Decide needs. Keep it free of protos so unit
// tests stay small and call sites map fields explicitly.
type Input struct {
	// AssignmentRequired is true for RESUMING/RUNNING paths that must have a worker.
	AssignmentRequired bool
	// HasAssignment is true when the actor record carries a worker assignment.
	HasAssignment bool
	// WorkerHealth is ignored when HasAssignment is false.
	WorkerHealth WorkerHealth
	// HostsActor is true when the worker still claims this actor UID.
	HostsActor bool
	// EpochFenced is true when the worker epoch advanced past the assignment's epoch.
	EpochFenced bool
	// AteletClass is the class of a just-observed atelet error, or AteletNone.
	AteletClass AteletClass
	// TerminateRPC is true when the failing call was Terminate (must not crash).
	TerminateRPC bool
	// ReassignRequested is true when the caller wants to place on another worker.
	ReassignRequested bool
	// SnapshotDurability is consulted only when ReassignRequested is true.
	SnapshotDurability SnapshotDurability
}

// Outcome is the recovery decision.
type Outcome struct {
	Action           Action
	ClientVisibility ClientVisibility
	// AllowReassign is true only when termination/fencing is confirmed and a
	// durable snapshot (or no local-only constraint) makes migrate safe.
	AllowReassign bool
}

// Decide returns the recovery action for in. Precedence matches today's
// controlapi behavior: fencing and worker-gone beat wait/retry; ambiguous
// atelet results never authorize reassignment.
func Decide(in Input) Outcome {
	if in.AssignmentRequired && !in.HasAssignment {
		return Outcome{Action: CrashAndRelease, ClientVisibility: Unrecoverable}
	}

	if in.HasAssignment {
		if in.EpochFenced || in.WorkerHealth == WorkerGone || in.WorkerHealth == WorkerDraining {
			return Outcome{Action: CrashAndRelease, ClientVisibility: Unrecoverable, AllowReassign: true}
		}
		if !in.HostsActor {
			return Outcome{Action: CrashAndRelease, ClientVisibility: Unrecoverable, AllowReassign: true}
		}
		if in.WorkerHealth == WorkerUnavailable {
			return Outcome{Action: WaitSameAssignment, ClientVisibility: RecoveryPending}
		}
	}

	if in.AteletClass != AteletNone {
		if in.TerminateRPC || in.AteletClass == AteletAmbiguous {
			return Outcome{Action: RetrySameExecution, ClientVisibility: RecoverableFailure}
		}
		return Outcome{Action: CrashAndRelease, ClientVisibility: Unrecoverable, AllowReassign: true}
	}

	if in.ReassignRequested {
		if in.SnapshotDurability == SnapshotLocalOnly {
			return Outcome{Action: RequireDurableBeforeMigrate, ClientVisibility: RecoverableFailure}
		}
		if !in.HasAssignment {
			return Outcome{Action: Proceed, ClientVisibility: VisibilityNone, AllowReassign: true}
		}
		// Still bound: refuse migrate until fencing cleared the assignment above.
		return Outcome{Action: WaitSameAssignment, ClientVisibility: RecoveryPending}
	}

	return Outcome{Action: Proceed, ClientVisibility: VisibilityNone}
}

