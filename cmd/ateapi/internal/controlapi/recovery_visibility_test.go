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

package controlapi

import (
	"testing"

	"github.com/agent-substrate/substrate/cmd/ateapi/internal/controlapi/recovery"
	"github.com/agent-substrate/substrate/pkg/proto/ateapipb"
)

func TestActorRecoveryVisibility(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in   recovery.ClientVisibility
		want ateapipb.ActorRecoveryVisibility
	}{
		{recovery.VisibilityNone, ateapipb.ActorRecoveryVisibility_ACTOR_RECOVERY_VISIBILITY_NONE},
		{recovery.RecoveryPending, ateapipb.ActorRecoveryVisibility_ACTOR_RECOVERY_VISIBILITY_PENDING},
		{recovery.RecoverableFailure, ateapipb.ActorRecoveryVisibility_ACTOR_RECOVERY_VISIBILITY_RECOVERABLE_FAILURE},
		{recovery.Unrecoverable, ateapipb.ActorRecoveryVisibility_ACTOR_RECOVERY_VISIBILITY_UNRECOVERABLE},
		{recovery.ClientVisibility(99), ateapipb.ActorRecoveryVisibility_ACTOR_RECOVERY_VISIBILITY_NONE},
	}
	for _, tc := range cases {
		if got := actorRecoveryVisibility(tc.in); got != tc.want {
			t.Errorf("actorRecoveryVisibility(%v) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

func TestSetStatusRecoveryVisibility(t *testing.T) {
	t.Parallel()
	status := &ateapipb.ActorStatus{}
	setStatusRecoveryVisibility(status, recovery.RecoveryPending)
	if got := status.GetRecoveryVisibility(); got != ateapipb.ActorRecoveryVisibility_ACTOR_RECOVERY_VISIBILITY_PENDING {
		t.Fatalf("RecoveryVisibility = %v, want PENDING", got)
	}
	setStatusRecoveryVisibility(nil, recovery.Unrecoverable) // must not panic
}
